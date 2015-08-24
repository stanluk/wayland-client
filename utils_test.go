package wayland

import (
	"testing"
)

func TestFixedToFloat64(t *testing.T) {
	var f int32
	var d float64

	f = 0x012030
	d = fixedToFloat64(f)
	if d != 288.1875 {
		t.Fail()
	}

	f = -0x012030
	d = fixedToFloat64(f)
	if d != -288.1875 {
		t.Fail()
	}
}
