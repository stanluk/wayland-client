package wayland

import (
	"fmt"
	"log"
	"syscall"
	"testing"
	"time"
)

func TestDisconnection(t *testing.T) {
	d, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
	}
	go d.Listen()
	select {
	case wlerror := <-d.ErrorChan:
		t.Logf("Error: obj: %d, err_code: %d, msg: %s", id, code, m)
		t.Fail()
	case <-time.After(5 * time.Second):
		t.Logf("Timeout passed...")
	}
	err = d.Close()
	if err != nil {
		t.Errorf("Disconnecing wayland server failed")
	}
}

func TestSync(t *testing.T) {
	display, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
	}
	callback, err := display.Sync()
	if err != nil {
		t.Errorf("Sync request failed.")
	}
	go display.Listen()
	select {
	case wlerror := <-display.ErrorChan:
		t.Logf("Error: obj: %d, err_code: %d, msg: %s", id, code, m)
		t.Fail()
	case done := <-callback.DoneChan:
		t.Logf("Done callback")
	case <-time.After(100 * time.Millisecond):
		t.Logf("Timeout passed...")
		t.Fail()
	}
	display.Close()
}

func TestGetRegistry(t *testing.T) {
	display, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
	}
	registry, err := display.GetRegistry()
	if err != nil {
		t.Errorf("Disconnecing wayland server failed")
	}
	go d.Listen()
	select {
	case wlerror := <-display.ErrorChan:
		t.Logf("Error: obj: %d, err_code: %d, msg: %s", id, code, m)
		t.Fail()
	case done := <-registry.GlobalChan:
		t.Logf("Global chan")
	case <-time.After(100 * time.Millisecond):
		t.Logf("Timeout passed...")
		t.Fail()
	}
	display.Close()
}

func BenchmarkRegistrySync(t *testing.B) {
	display, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
		t.FailNow()
	}
	go display.Listen()
	for i := 0; i < t.N; i++ {
		callback, err := display.Sync()
		if err != nil {
			t.Errorf("Unable to send Sync() request.")
		}
		<-callback.DoneChan
	}
	display.Close()
}

func TestGetPointer(t *testing.T) {
	display, err := ConnectDisplay("")
	var pointer Pointer
	if err != nil {
		t.FailNow()
	}
	registry, err := display.GetRegistry()
	if err != nil {
		t.FailNow()
	}
	for {
		select {
		case <-display.ErrorChan:
			t.FailNow()
		case ev := <-registry.GlobalChan:
			if ev.ifc == "wl_seat" {
				seat := NewSeat(reg)
				err = registry.Bind(name, ifc, version, seat)
				if err != nil {
					t.FailNow()
				}
				pointer, err = seat.GetPointer()
				if err != nil {
					t.FailNow()
				}
				break
			}
		}
	}

	if pointer == nil {
		t.FailNow()
	}

	for {
		select {
		case <-pointer.MotionChan:
			t.Logf("Mouse position ")
		case <-time.After(2000 * time.Millisecond):
			break
		}
	}
	display.Close()
}

func ExamplePaint() {
	var (
		shm        Shm
		compositor Compositor
		surface    Surface
		data       []byte
		buf        Buffer
		shell      Shell
		pointer    Pointer
		seat       Seat
		width      int  = 640
		height     int  = 480
		pressed    bool = false
	)
	stride := width * 4
	size := stride * height
	display, err := ConnectDisplay("")
	if err != nil {
		panic("Failed to connect to wayland server")
	}

	// run one gorutine to panic in case of protocol error
	go func() {
		<-display.ErrorChan
		panic("Protocol Error")
	}()

	registry, err := display.GetRegistry()
	if err != nil {
		panic("Disconnecing wayland server failed")
	}
	callback, err := registry.Sync()
	if err != nil {
		panic("Sync request failed")
	}

	for {
		select {
		case ev <- registry.GlobalChan:
			if ev.ifc == "wl_shm" {
				shm = NewShm(&reg.Proxy)
				err = reg.Bind(name, ifc, version, shm.Id)
				if err != nil {
					panic("unable to bind Shm object")
				}
			}
			if ev.ifc == "wl_compositor" {
				compositor = NewCompositor(&reg.Proxy)
				err = reg.Bind(name, ifc, version, compositor.Id)
				if err != nil {
					panic("unable to bind Compositor object")
				}
			}
			if ev.ifc == "wl_shell" {
				shell = NewShell(&reg.Proxy)
				err = reg.Bind(name, ifc, version, shell.Id)
				if err != nil {
					panic("unable to bind shell object")
				}
			}
			if ev.ifc == "wl_seat" {
				seat = NewSeat(&reg.Proxy)
				err = reg.Bind(name, ifc, version, seat.Id)
				if err != nil {
					panic("unable to bind seat object")
				}
			}
		case <-callback.DoneChan:
			break
		}
	}

	// query seat capabilities
	callback, err := registry.Sync()
	if err != nil {
		panic("Sync request failed")
	}
	for {
		select {
		case ev <- seat.CapabilitiesChan:
			if (ev.caps & WlSeatCapabilityPointer) != 0 {
				pointer, err = seat.GetPointer()
				if err != nil {
					panic("unable to get pointer object")
				}
			}
		case <-callback.DoneChan:
			break
		}
	}
	// if we don't have a pointer - just exit program
	if pointer == nil {
		return
	}
	surface, err = compositor.CreateSurface()
	if err != nil {
		panic("Surface creation failed")
	}

	// create shm buffer
	file, err := CreateAnonymousFile(size)
	if err != nil {
		panic("Unable to create file")
	}
	data, err = syscall.Mmap(int(file.Fd()), 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		panic("Unable to mmap file")
	}
	for i, _ := range data {
		data[i] = 255
	}
	pool, err := shm.CreatePool(file, size)
	if err != nil {
		panic(fmt.Sprintf("Unable to create pool: %s", err))
	}
	buf, err = pool.CreateBuffer(0, width, height, stride, ShmFormatArgb8888)
	if err != nil {
		panic("Unable to create buffer")
	}
	pool.Destroy()
	shsurf, err := shell.GetShellSurface(surface)
	if err != nil {
		panic("Unable to shell surface")
	}
	shsurf.SetToplevel()
	err = surface.Attach(buf, width, height)
	if err != nil {
		panic("Unable to attach buffer")
	}
	err = surface.Damage(0, 0, width, height)
	if err != nil {
		panic("Unable to damage surface")
	}
	surface.Commit()
	if err != nil {
		panic("Unable to commit surface")
	}

	// start main loop
	go d.Listen()

	// main application controller
	for {
		select {
		case ev := <-shsurg.PingChan:
			shsurf.Pong(ev.serial)
		case ev := <-pointer.MotionChan:
			if pressed {
				x1 := int(x)
				y1 := int(y)
				offsets := []int{0, 1, -1, 2, -2}
				for _, x := range offsets {
					for _, y := range offsets {
						dx := y1 + x
						dy := x1 + y
						if dx < 0 || dx > height-1 || dy < 0 || dy > width-1 {
							continue
						}
						data[(dx)*stride+(dy)*4+1] = 0
						data[(dx)*stride+(dy)*4+2] = 0
					}
				}
				if surface != nil {
					surface.Attach(buf, 0, 0)
					surface.Damage(x1-1, y1-1, 4, 4)
					surface.Commit()
				}
			}
		case ev := <-pointer.ButtonChan:
			if state == WlPointerButtonStatePressed {
				pressed = true
			}
			if state == WlPointerButtonStateReleased {
				pressed = false
			}
			log.Println("PointerButton: ", time, butt, state)
		case <-time.After(10000 * time.Millisecond):
			break
		}
	}
	// Output:
	// OK
	display.Close()
}
