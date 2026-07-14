package main

import (
	"testing"
	"unsafe"
)

// The Win32 structs in win32types.go must match the windows/amd64 SDK layout
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
	if vkMenuSuppressLegacy != 0x07 {
		t.Fatalf("vkMenuSuppressLegacy = %#x, want 0x07", vkMenuSuppressLegacy)
	}
	// This second key must be assigned so Electron/Chromium can observe it.
	if vkMenuSuppressDOM != 0x87 {
		t.Fatalf("vkMenuSuppressDOM = %#x, want VK_F24 (0x87)", vkMenuSuppressDOM)
	}
}

func TestKbdllHookStructLayout(t *testing.T) {
	require64(t)
	var v kbdllHookStruct
	assertSize(t, unsafe.Sizeof(v), 24)
	assertOffset(t, "vkCode", unsafe.Offsetof(v.vkCode), 0)
	assertOffset(t, "scanCode", unsafe.Offsetof(v.scanCode), 4)
	assertOffset(t, "flags", unsafe.Offsetof(v.flags), 8)
	assertOffset(t, "time", unsafe.Offsetof(v.time), 12)
	assertOffset(t, "dwExtraInfo", unsafe.Offsetof(v.dwExtraInfo), 16)
}

func TestKeybdInputLayout(t *testing.T) {
	require64(t)
	var v keybdInput
	assertSize(t, unsafe.Sizeof(v), 24)
	assertOffset(t, "wVk", unsafe.Offsetof(v.wVk), 0)
	assertOffset(t, "wScan", unsafe.Offsetof(v.wScan), 2)
	assertOffset(t, "dwFlags", unsafe.Offsetof(v.dwFlags), 4)
	assertOffset(t, "time", unsafe.Offsetof(v.time), 8)
	assertOffset(t, "dwExtraInfo", unsafe.Offsetof(v.dwExtraInfo), 16)
}

func TestInputStructLayout(t *testing.T) {
	require64(t)
	var v inputStruct
	assertSize(t, unsafe.Sizeof(v), 40) // DWORD + pad + 32-byte union
	assertOffset(t, "inputType", unsafe.Offsetof(v.inputType), 0)
	assertOffset(t, "ki", unsafe.Offsetof(v.ki), 8)
}

func TestMsgStructLayout(t *testing.T) {
	require64(t)
	var v msgStruct
	assertSize(t, unsafe.Sizeof(v), 48)
	assertOffset(t, "hwnd", unsafe.Offsetof(v.hwnd), 0)
	assertOffset(t, "message", unsafe.Offsetof(v.message), 8)
	assertOffset(t, "wParam", unsafe.Offsetof(v.wParam), 16)
	assertOffset(t, "lParam", unsafe.Offsetof(v.lParam), 24)
	assertOffset(t, "time", unsafe.Offsetof(v.time), 32)
	assertOffset(t, "pt", unsafe.Offsetof(v.pt), 36)
}

func TestWndClassExWLayout(t *testing.T) {
	require64(t)
	var v wndClassExW
	assertSize(t, unsafe.Sizeof(v), 80)
	assertOffset(t, "lpfnWndProc", unsafe.Offsetof(v.lpfnWndProc), 8)
	assertOffset(t, "hInstance", unsafe.Offsetof(v.hInstance), 24)
	assertOffset(t, "hCursor", unsafe.Offsetof(v.hCursor), 40)
	assertOffset(t, "lpszClassName", unsafe.Offsetof(v.lpszClassName), 64)
	assertOffset(t, "hIconSm", unsafe.Offsetof(v.hIconSm), 72)
}

func TestMonitorInfoLayout(t *testing.T) {
	require64(t)
	var v monitorInfo
	assertSize(t, unsafe.Sizeof(v), 40)
	assertOffset(t, "rcMonitor", unsafe.Offsetof(v.rcMonitor), 4)
	assertOffset(t, "rcWork", unsafe.Offsetof(v.rcWork), 20)
	assertOffset(t, "dwFlags", unsafe.Offsetof(v.dwFlags), 36)
}

func TestPaintStructLayout(t *testing.T) {
	require64(t)
	var v paintStruct
	assertSize(t, unsafe.Sizeof(v), 72)
	assertOffset(t, "hdc", unsafe.Offsetof(v.hdc), 0)
	assertOffset(t, "rcPaint", unsafe.Offsetof(v.rcPaint), 12)
	assertOffset(t, "rgbReserved", unsafe.Offsetof(v.rgbReserved), 36)
}

func TestNotifyIconDataWLayout(t *testing.T) {
	require64(t)
	var v notifyIconDataW
	assertSize(t, unsafe.Sizeof(v), 976)
	assertOffset(t, "hWnd", unsafe.Offsetof(v.hWnd), 8)
	assertOffset(t, "uID", unsafe.Offsetof(v.uID), 16)
	assertOffset(t, "uCallbackMessage", unsafe.Offsetof(v.uCallbackMessage), 24)
	assertOffset(t, "hIcon", unsafe.Offsetof(v.hIcon), 32)
	assertOffset(t, "szTip", unsafe.Offsetof(v.szTip), 40)
	assertOffset(t, "dwState", unsafe.Offsetof(v.dwState), 296)
	assertOffset(t, "szInfo", unsafe.Offsetof(v.szInfo), 304)
	assertOffset(t, "uVersion", unsafe.Offsetof(v.uVersion), 816)
	assertOffset(t, "szInfoTitle", unsafe.Offsetof(v.szInfoTitle), 820)
	assertOffset(t, "dwInfoFlags", unsafe.Offsetof(v.dwInfoFlags), 948)
	assertOffset(t, "guidItem", unsafe.Offsetof(v.guidItem), 952)
	assertOffset(t, "hBalloonIcon", unsafe.Offsetof(v.hBalloonIcon), 968)
}

func TestSmallStructLayouts(t *testing.T) {
	require64(t)
	assertSize(t, unsafe.Sizeof(point{}), 8)
	assertSize(t, unsafe.Sizeof(rect{}), 16)
	assertSize(t, unsafe.Sizeof(guid{}), 16)
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
