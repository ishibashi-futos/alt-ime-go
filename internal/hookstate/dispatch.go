package hookstate

// Wire format of the two-stage dispatches (hook thread -> UI window
// message). Requests carry only plain integers through WPARAM/LPARAM (never
// Go pointers): WPARAM packs the request bits, LPARAM carries the target
// HWND.

// ---- switch request packing ----

// WPARAM packs the direction and the trigger VK.

const switchOpenBit = 0x100

func PackSwitchWParam(open bool, vk uint32) uintptr {
	wp := uintptr(vk & 0xFF)
	if open {
		wp |= switchOpenBit
	}
	return wp
}

func UnpackSwitchWParam(wp uintptr) (open bool, vk uint32) {
	return wp&switchOpenBit != 0, uint32(wp & 0xFF)
}

// ---- Enter-guard request packing ----

// The guarded Enter is consumed inside the callback; WPARAM carries what the
// UI needs to pick the replacement: whether Ctrl was held (send intent) and
// whether a composition is believed to be in progress.

const (
	guardSendBit      = 0x1
	guardComposingBit = 0x2
)

func PackGuardWParam(send, composing bool) uintptr {
	var wp uintptr
	if send {
		wp |= guardSendBit
	}
	if composing {
		wp |= guardComposingBit
	}
	return wp
}

func UnpackGuardWParam(wp uintptr) (send, composing bool) {
	return wp&guardSendBit != 0, wp&guardComposingBit != 0
}
