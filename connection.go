package wayland

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"sync"
	"time"
)

type Connection struct {
	mu              sync.Mutex
	conn            *net.UnixConn
	currentId       ProxyId
	objects         map[ProxyId]Proxy
	dispatchRequest chan bool
	exit            chan bool
}

func (context *Connection) Register(proxy Proxy) {
	context.mu.Lock()
	context.currentId += 1
	proxy.SetId(context.currentId)
	proxy.SetConnection(context)
	context.objects[context.currentId] = proxy
	context.mu.Unlock()
}

func (context *Connection) Unregister(proxy Proxy) {
	context.mu.Lock()
	delete(context.objects, proxy.Id())
	context.mu.Unlock()
}

func (context *Connection) Close() error {
	if context.conn == nil {
		return errors.New("Wayland connection not established.")
	}
	context.conn.Close()
	context.exit <- true
	return nil
}

func (context *Connection) Dispatch() chan<- bool {
	return context.dispatchRequest
}

func ConnectDisplay(addr string) (ret *Display, err error) {
	runtime_dir := os.Getenv("XDG_RUNTIME_DIR")
	if runtime_dir == "" {
		return nil, errors.New("XDG_RUNTIME_DIR not set in the environment.")
	}
	if addr == "" {
		addr = os.Getenv("WAYLAND_DISPLAY")
	}
	if addr == "" {
		addr = "wayland-0"
	}
	addr = runtime_dir + "/" + addr
	ctx := &Connection{}
	ctx.objects = make(map[ProxyId]Proxy)
	ctx.currentId = 0
	ctx.dispatchRequest = make(chan bool)
	ctx.exit = make(chan bool)
	ctx.conn, err = net.DialUnix("unix", nil, &net.UnixAddr{addr, "unix"})
	if err != nil {
		return nil, err
	}
	ret = NewDisplay(ctx)
	// dispatch events in separate gorutine
	go ctx.run()
	return ret, nil
}

func (context *Connection) SendRequest(proxy Proxy, opcode uint32, args ...interface{}) (err error) {
	if context.conn == nil {
		return errors.New("No wayland connection established for Proxy object.")
	}
	msg := NewRequest(proxy, opcode)

	for _, arg := range args {
		if err = msg.Write(arg); err != nil {
			return err
		}
	}

	return SendWaylandMessage(context.conn, msg)
}

func dispatchEvent(proxy Proxy, m *Message) {
	v := reflect.ValueOf(proxy)
	f := v.Elem().Field(int(m.Opcode) + 1) // +1 because of BaseProxy
	t := f.Type().Elem()
	ev := reflect.New(t)
	el := ev.Elem()
	for i := 0; i < el.NumField(); i++ {
		ef := el.Field(i)
		var fv reflect.Value
		switch ef.Kind() {
		case reflect.Int32:
			fv = reflect.ValueOf(m.GetInt32())
		case reflect.Uint32:
			fv = reflect.ValueOf(m.GetUint32())
		case reflect.Float32:
			fv = reflect.ValueOf(m.GetFloat32())
		case reflect.String:
			fv = reflect.ValueOf(m.GetString())
		case reflect.Slice:
			fv = reflect.ValueOf(m.GetArray())
		case reflect.Uintptr:
			fv = reflect.ValueOf(m.GetFD())
		case reflect.Ptr:
			fv = reflect.ValueOf(m.GetProxy(proxy.Connection())).Elem().Addr()
		default:
			panic(fmt.Sprint("Not handled field type: ", ef.Kind().String()))
		}
		ef.Set(fv)
	}
	f.Send(el)
}

func (context *Connection) run() error {
	context.conn.SetReadDeadline(time.Time{})
	for {
		select {
		case <-context.dispatchRequest:
			msg, err := ReadWaylandMessage(context.conn)
			if err != nil {
				continue
			}
			proxy, ok := context.objects[msg.Id]
			if !ok {
				return errors.New(fmt.Sprintf("Unknown object id: %d", msg.Id))
			}
			dispatchEvent(proxy, msg)
		case <-context.exit:
			return nil
		}
	}
	return nil
}
