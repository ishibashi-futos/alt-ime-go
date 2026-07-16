package hookstate

import (
	"testing"

	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

func TestGuardWParamPacking(t *testing.T) {
	for _, send := range []bool{false, true} {
		for _, composing := range []bool{false, true} {
			gotSend, gotComposing := UnpackGuardWParam(PackGuardWParam(send, composing))
			if gotSend != send || gotComposing != composing {
				t.Errorf("roundtrip(%v, %v) = (%v, %v)", send, composing, gotSend, gotComposing)
			}
		}
	}
}

func TestSwitchWParamPacking(t *testing.T) {
	cases := []struct {
		open bool
		vk   uint32
	}{
		{false, win32.VkLMenu},
		{true, win32.VkRMenu},
		{true, 0xFF},
		{false, 1},
	}
	for _, c := range cases {
		open, vk := UnpackSwitchWParam(PackSwitchWParam(c.open, c.vk))
		if open != c.open || vk != c.vk {
			t.Errorf("roundtrip(%v, %#x) = (%v, %#x)", c.open, c.vk, open, vk)
		}
	}
}
