package wayland

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

var (
	Logger = log.New(os.Stderr, "wayland: ", log.Lshortfile)
)

type connection struct {
	mu        sync.Mutex
	conn      *net.UnixConn
	currentId ProxyId
	objects   map[ProxyId]Proxy
	done      chan bool
}

func (d *Display) Register(proxy Proxy) {
	d.context.mu.Lock()
	d.context.currentId += 1
	proxy.SetId(d.context.currentId)
	proxy.SetDisplay(d)
	d.context.objects[d.context.currentId] = proxy
	d.context.mu.Unlock()
}

func (d *Display) Unregister(proxy Proxy) {
	d.context.mu.Lock()
	delete(d.context.objects, d.GetId())
	d.context.mu.Unlock()
}

func (d *Display) Close() error {
	if d.context.conn == nil {
		return errors.New("Wayland connection not established.")
	}
	d.context.done <- true
	return nil
}

func (d *Display) Listen() error {
	for {
		err := d.context.dispatch()
		if err != nil {
			return err
		}
	}
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
	//Logger.Println("Connecting to: ", addr)
	ctx := &connection{}
	ctx.objects = make(map[ProxyId]Proxy)
	ctx.done = make(chan bool)
	ctx.currentId = 0
	ctx.conn, err = net.DialUnix("unix", nil, &net.UnixAddr{addr, "unix"})
	if err != nil {
		return nil, err
	}
	ret = &Display{}
	ret.context = ctx
	ret.Register(ret)
	return ret, nil
}

func (p *Display) SendRequest(proxy Proxy, opcode uint32, args ...interface{}) (err error) {
	if p.context.conn == nil {
		return errors.New("No wayland connection established for Proxy object.")
	}
	msg := NewWaylandRequest(proxy, opcode)

	for arg, _ := range args {
		msg.Write(arg)
	}

	return SendWaylandMessage(p.context.conn, msg)
}

func (context *connection) dispatch() error {
	context.conn.SetReadDeadline(time.Time{})

	msg, err := ReadWaylandMessage(context.conn)
	if err != nil {
		return err
	}
	proxy, ok := context.objects[msg.Id]
	if !ok {
		return errors.New(fmt.Sprintf("Unknown object id: %d", msg.Id))
	}
	//Logger.Printf("Event recieved: obj: %d, opcode: %d, len: %d", id, opcode, size)
	err = proxy.DispatchEvent(msg)
	if err != nil {
		return errors.New(fmt.Sprintf("Unable to dispatch event %d.", msg.Opcode))
	}
	return nil
}
