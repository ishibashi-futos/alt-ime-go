package win32

import (
	"testing"
	"unsafe"
)

// The Win32 structs in types.go must match the windows/amd64 SDK layout
// byte for byte. Go's gc compiler lays out these field types identically on
// every 64-bit target, so the assertions run on any 64-bit host and hold for
// GOOS=windows GOARCH=amd64.

func require64(t *testing.T) {
	t.Helper()
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout assertions target 64-bit platforms")
	}
}

func TestMenuSuppressKeys(t *testing.T) {
	if VkMenuSuppressLegacy != 0x07 {
		t.Fatalf("VkMenuSuppressLegacy = %#x, want 0x07", VkMenuSuppressLegacy)
	}
	// This second key must be assigned so Electron/Chromium can observe it.
	if VkMenuSuppressDOM != 0x87 {
		t.Fatalf("VkMenuSuppressDOM = %#x, want VK_F24 (0x87)", VkMenuSuppressDOM)
	}
}

func TestKbdllHookStructLayout(t *testing.T) {
	require64(t)
	var v KbdllHookStruct
	assertSize(t, unsafe.Sizeof(v), 24)
	assertOffset(t, "VkCode", unsafe.Offsetof(v.VkCode), 0)
	assertOffset(t, "ScanCode", unsafe.Offsetof(v.ScanCode), 4)
	assertOffset(t, "Flags", unsafe.Offsetof(v.Flags), 8)
	assertOffset(t, "Time", unsafe.Offsetof(v.Time), 12)
	assertOffset(t, "DwExtraInfo", unsafe.Offsetof(v.DwExtraInfo), 16)
}

func TestKeybdInputLayout(t *testing.T) {
	require64(t)
	var v KeybdInput
	assertSize(t, unsafe.Sizeof(v), 24)
	assertOffset(t, "WVk", unsafe.Offsetof(v.WVk), 0)
	assertOffset(t, "WScan", unsafe.Offsetof(v.WScan), 2)
	assertOffset(t, "DwFlags", unsafe.Offsetof(v.DwFlags), 4)
	assertOffset(t, "Time", unsafe.Offsetof(v.Time), 8)
	assertOffset(t, "DwExtraInfo", unsafe.Offsetof(v.DwExtraInfo), 16)
}

func TestInputStructLayout(t *testing.T) {
	require64(t)
	var v InputStruct
	assertSize(t, unsafe.Sizeof(v), 40) // DWORD + pad + 32-byte union
	assertOffset(t, "InputType", unsafe.Offsetof(v.InputType), 0)
	assertOffset(t, "Ki", unsafe.Offsetof(v.Ki), 8)
}

func TestMsgStructLayout(t *testing.T) {
	require64(t)
	var v MsgStruct
	assertSize(t, unsafe.Sizeof(v), 48)
	assertOffset(t, "Hwnd", unsafe.Offsetof(v.Hwnd), 0)
	assertOffset(t, "Message", unsafe.Offsetof(v.Message), 8)
	assertOffset(t, "WParam", unsafe.Offsetof(v.WParam), 16)
	assertOffset(t, "LParam", unsafe.Offsetof(v.LParam), 24)
	assertOffset(t, "Time", unsafe.Offsetof(v.Time), 32)
	assertOffset(t, "Pt", unsafe.Offsetof(v.Pt), 36)
}

func TestWndClassExWLayout(t *testing.T) {
	require64(t)
	var v WndClassExW
	assertSize(t, unsafe.Sizeof(v), 80)
	assertOffset(t, "LpfnWndProc", unsafe.Offsetof(v.LpfnWndProc), 8)
	assertOffset(t, "HInstance", unsafe.Offsetof(v.HInstance), 24)
	assertOffset(t, "HCursor", unsafe.Offsetof(v.HCursor), 40)
	assertOffset(t, "LpszClassName", unsafe.Offsetof(v.LpszClassName), 64)
	assertOffset(t, "HIconSm", unsafe.Offsetof(v.HIconSm), 72)
}

func TestMonitorInfoLayout(t *testing.T) {
	require64(t)
	var v MonitorInfo
	assertSize(t, unsafe.Sizeof(v), 40)
	assertOffset(t, "RcMonitor", unsafe.Offsetof(v.RcMonitor), 4)
	assertOffset(t, "RcWork", unsafe.Offsetof(v.RcWork), 20)
	assertOffset(t, "DwFlags", unsafe.Offsetof(v.DwFlags), 36)
}

func TestPaintStructLayout(t *testing.T) {
	require64(t)
	var v PaintStruct
	assertSize(t, unsafe.Sizeof(v), 72)
	assertOffset(t, "Hdc", unsafe.Offsetof(v.Hdc), 0)
	assertOffset(t, "RcPaint", unsafe.Offsetof(v.RcPaint), 12)
	assertOffset(t, "RgbReserved", unsafe.Offsetof(v.RgbReserved), 36)
}

func TestNotifyIconDataWLayout(t *testing.T) {
	require64(t)
	var v NotifyIconDataW
	assertSize(t, unsafe.Sizeof(v), 976)
	assertOffset(t, "HWnd", unsafe.Offsetof(v.HWnd), 8)
	assertOffset(t, "UID", unsafe.Offsetof(v.UID), 16)
	assertOffset(t, "UCallbackMessage", unsafe.Offsetof(v.UCallbackMessage), 24)
	assertOffset(t, "HIcon", unsafe.Offsetof(v.HIcon), 32)
	assertOffset(t, "SzTip", unsafe.Offsetof(v.SzTip), 40)
	assertOffset(t, "DwState", unsafe.Offsetof(v.DwState), 296)
	assertOffset(t, "SzInfo", unsafe.Offsetof(v.SzInfo), 304)
	assertOffset(t, "UVersion", unsafe.Offsetof(v.UVersion), 816)
	assertOffset(t, "SzInfoTitle", unsafe.Offsetof(v.SzInfoTitle), 820)
	assertOffset(t, "DwInfoFlags", unsafe.Offsetof(v.DwInfoFlags), 948)
	assertOffset(t, "GuidItem", unsafe.Offsetof(v.GuidItem), 952)
	assertOffset(t, "HBalloonIcon", unsafe.Offsetof(v.HBalloonIcon), 968)
}

func TestGuiThreadInfoLayout(t *testing.T) {
	require64(t)
	var v GuiThreadInfo
	assertSize(t, unsafe.Sizeof(v), 72)
	assertOffset(t, "Flags", unsafe.Offsetof(v.Flags), 4)
	assertOffset(t, "HwndActive", unsafe.Offsetof(v.HwndActive), 8)
	assertOffset(t, "HwndFocus", unsafe.Offsetof(v.HwndFocus), 16)
	assertOffset(t, "HwndCaret", unsafe.Offsetof(v.HwndCaret), 48)
	assertOffset(t, "RcCaret", unsafe.Offsetof(v.RcCaret), 56)
}

func TestSmallStructLayouts(t *testing.T) {
	require64(t)
	assertSize(t, unsafe.Sizeof(Point{}), 8)
	assertSize(t, unsafe.Sizeof(Rect{}), 16)
	assertSize(t, unsafe.Sizeof(GUID{}), 16)
}

func assertSize(t *testing.T, got uintptr, want uintptr) {
	t.Helper()
	if got != want {
		t.Errorf("sizeof = %d, want %d", got, want)
	}
}

func assertOffset(t *testing.T, field string, got, want uintptr) {
	t.Helper()
	if got != want {
		t.Errorf("offsetof(%s) = %d, want %d", field, got, want)
	}
}
