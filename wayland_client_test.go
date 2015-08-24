package wayland

import (
	"fmt"
	"log"
	"syscall"
	"testing"
	"time"
)

func TestConnectionDisconnection(t *testing.T) {
	d, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
	}
	d.Connection().Close()
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
loop:
	for {
		select {
		case dee := <-display.ErrorChan:
			t.Log("Error: obj: %d, err_code: %d, msg: %s", dee.ObjectId.Id(), dee.Code, dee.Message)
			t.Fail()
			break loop
		case <-callback.DoneChan:
			break loop
		case <-time.After(1000 * time.Millisecond):
			t.Fail()
			break loop
		case display.Connection().Dispatch() <- true:
		}
	}
	display.Connection().Close()
}

func TestGetRegistry(t *testing.T) {
	display, err := ConnectDisplay("")
	if err != nil {
		t.Errorf("Failed to connect to wayland server")
	}
	var has_global bool = false
	registry, err := display.GetRegistry()
	if err != nil {
		t.Errorf("Disconnecing wayland server failed")
	}
loop:
	for {
		select {
		case dee := <-display.ErrorChan:
			t.Logf("Error: obj: %d, err_code: %d, msg: %s", dee.ObjectId.Id(), dee.Code, dee.Message)
			t.Fail()
		case ge := <-registry.GlobalChan:
			t.Logf("Global obj %s %d %d", ge.Ifc, ge.Name, ge.Version)
			has_global = true
		case <-time.After(100 * time.Millisecond):
			break loop
		case display.Connection().Dispatch() <- true:
		}
	}
	display.Connection().Close()
	if !has_global {
		t.Fail()
	}
}

func ExamplePaint() {
	var (
		shm        *Shm
		compositor *Compositor
		surface    *Surface
		data       []byte
		buf        *Buffer
		shell      *Shell
		pointer    *Pointer
		seat       *Seat
		keyboard   *Keyboard
		width      int32 = 640
		height     int32 = 480
		pressed    bool  = false
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
	callback, err := display.Sync()
	if err != nil {
		panic("Sync request failed")
	}

loop:
	for {
		select {
		case ev := <-registry.GlobalChan:
			if ev.Ifc == "wl_shm" {
				shm = NewShm(display.Connection())
				err = registry.Bind(ev.Name, ev.Ifc, ev.Version, shm)
				if err != nil {
					panic("unable to bind Shm object")
				}
			}
			if ev.Ifc == "wl_compositor" {
				compositor = NewCompositor(display.Connection())
				err = registry.Bind(ev.Name, ev.Ifc, ev.Version, compositor)
				if err != nil {
					panic("unable to bind Compositor object")
				}
			}
			if ev.Ifc == "wl_shell" {
				shell = NewShell(display.Connection())
				err = registry.Bind(ev.Name, ev.Ifc, ev.Version, shell)
				if err != nil {
					panic("unable to bind shell object")
				}
			}
			if ev.Ifc == "wl_seat" {
				seat = NewSeat(display.Connection())
				err = registry.Bind(ev.Name, ev.Ifc, ev.Version, seat)
				if err != nil {
					panic("unable to bind seat object")
				}
			}
		case <-callback.DoneChan:
			break loop
		case display.Connection().Dispatch() <- true:
		}
	}

	// query seat capabilities
	callback, err = display.Sync()
	if err != nil {
		panic("Sync request failed")
	}
loop2:
	for {
		select {
		case <-shm.FormatChan:
		case <-display.DeleteIdChan:
		case ev := <-seat.CapabilitiesChan:
			if (ev.Capabilities & SeatCapabilityPointer) != 0 {
				pointer, err = seat.GetPointer()
				if err != nil {
					panic("unable to get pointer object")
				}
				keyboard, err = seat.GetKeyboard()
				if err != nil {
					panic("unable to get keyboard object")
				}
			}
		case <-seat.NameChan:
		case <-callback.DoneChan:
			break loop2
		case display.Connection().Dispatch() <- true:
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
	file, err := CreateAnonymousFile(int(size))
	if err != nil {
		panic("Unable to create file")
	}
	data, err = syscall.Mmap(int(file.Fd()), 0, int(size), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		panic("Unable to mmap file")
	}
	for i, _ := range data {
		data[i] = 255
	}
	pool, err := shm.CreatePool(file.Fd(), size)
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

	// main application loop
main_loop:
	for {
		select {
		case <-display.DeleteIdChan:
		case <-seat.NameChan:
		case <-seat.CapabilitiesChan:
		case <-buf.ReleaseChan:
		case <-pointer.EnterChan:
		case <-pointer.LeaveChan:
		case ev := <-shsurf.PingChan:
			shsurf.Pong(ev.Serial)
		case ev := <-pointer.MotionChan:
			if pressed {
				x1 := int32(ev.SurfaceX)
				y1 := int32(ev.SurfaceY)
				offsets := []int32{0, 1, -1, 2, -2}
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
			log.Println("PointerButton: ", ev.Time, ev.Button, ev.State)
			if ev.State == PointerButtonStatePressed {
				pressed = true
			}
			if ev.State == PointerButtonStateReleased {
				pressed = false
			}
		case <-keyboard.EnterChan:
		case <-keyboard.LeaveChan:
		case <-keyboard.ModifiersChan:
		case ev := <-keyboard.KeymapChan:
			log.Println("KeyMapEvent", ev.Format, ev.Fd)
		case <-keyboard.RepeatInfoChan:
		case ev := <-keyboard.KeyChan:
			log.Println("KeyEvent: ", ev.Key)
			if ev.Key == 16 {
				fmt.Println("OK")
				break main_loop
			}
		case display.Connection().Dispatch() <- true:
		}
	}
	// Output:
	// OK
	display.Connection().Close()
}
