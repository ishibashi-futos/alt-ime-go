package main

// Win32 constants and struct layouts used by the bindings in win32.go.
// This file has no build constraint on purpose: it contains no syscalls, so
// the layout tests in win32types_test.go can run on any 64-bit host, not
// only under GOOS=windows. All layouts target windows/amd64.

// ownInputTag marks every KEYBDINPUT.dwExtraInfo this process injects
// (IME keys and the menu-focus suppressor). The hook treats an event as
// self-injected only when LLKHF_INJECTED is set AND dwExtraInfo matches.
// It is an ownership marker, not a security boundary. ASCII "ALTIME1".
const ownInputTag uintptr = 0x414C54494D4531

// Virtual-key codes.
const (
	vkLButton      = 0x01
	vkRButton      = 0x02
	vkCancel       = 0x03
	vkMButton      = 0x04
	vkXButton1     = 0x05
	vkXButton2     = 0x06
	vkMenuSuppress = 0x07 // unassigned VK injected to suppress Alt menu focus
	vkShift        = 0x10
	vkControl      = 0x11
	vkMenu         = 0x12
	vkImeOn        = 0x16
	vkImeOff       = 0x1A
	vkLMenu        = 0xA4
	vkRMenu        = 0xA5
)

// Low-level keyboard hook.
const (
	whKeyboardLL = 13
	hcAction     = 0

	wmKeyDown    = 0x0100
	wmKeyUp      = 0x0101
	wmSysKeyDown = 0x0104
	wmSysKeyUp   = 0x0105

	llkhfExtended = 0x01
	llkhfInjected = 0x10
	llkhfUp       = 0x80
)

// SendInput.
const (
	inputKeyboard  = 1
	keyEventFKeyUp = 0x0002
)

// Window messages.
const (
	wmNull             = 0x0000
	wmDestroy          = 0x0002
	wmPaint            = 0x000F
	wmClose            = 0x0010
	wmContextMenu      = 0x007B
	wmTimer            = 0x0113
	wmPowerBroadcast   = 0x0218
	wmImeControl       = 0x0283
	wmWtsSessionChange = 0x02B1
	wmDpiChanged       = 0x02E0
	wmUser             = 0x0400
	wmApp              = 0x8000
)

// Application messages. Controller-window messages first, then messages
// posted to the hook thread's message queue.
const (
	msgSwitch      = wmApp + 1 // wParam: packed open+VK, lParam: target HWND
	msgTray        = wmApp + 2 // tray callback (NOTIFYICON_VERSION_4 encoding)
	msgHookStopped = wmApp + 3 // wParam: 1 when the hook loop died unexpectedly

	msgHookDispatchSwitch = wmApp + 16 // same payload as msgSwitch
	msgHookSetEnabled     = wmApp + 17 // wParam: 0/1
	msgHookReset          = wmApp + 18
	msgHookStop           = wmApp + 19
)

// IMM32 / WM_IME_CONTROL.
const (
	imcSetOpenStatus = 0x0006

	smtoBlock       = 0x0001
	smtoAbortIfHung = 0x0002
)

// Session / power notifications.
const (
	wtsSessionLock        = 0x7
	wtsSessionUnlock      = 0x8
	notifyForThisSession  = 0
	pbtApmResumeSuspend   = 0x0007
	pbtApmResumeAutomatic = 0x0012
)

// Shell_NotifyIconW.
const (
	nimAdd        = 0x0000
	nimModify     = 0x0001
	nimDelete     = 0x0002
	nimSetFocus   = 0x0003
	nimSetVersion = 0x0004

	nifMessage = 0x0001
	nifIcon    = 0x0002
	nifTip     = 0x0004
	nifShowTip = 0x0080

	notifyIconVersion4 = 4

	ninSelect    = wmUser + 0
	ninKeySelect = wmUser + 1
)

// Window styles and show/positioning flags.
const (
	wsOverlapped = 0x00000000
	wsPopup      = 0x80000000

	wsExTopmost     = 0x00000008
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000

	swHide           = 0
	swShowNoActivate = 4

	swpNoActivate = 0x0010

	lwaAlpha = 0x0002

	csVRedraw = 0x0001
	csHRedraw = 0x0002
)

// hwndTopmost is the HWND_TOPMOST pseudo-handle (-1).
const hwndTopmost = ^uintptr(0)

// Menus.
const (
	mfString    = 0x0000
	mfUnchecked = 0x0000
	mfChecked   = 0x0008
	mfByCommand = 0x0000

	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100
)

// MessageBox.
const (
	mbOK              = 0x0000
	mbIconError       = 0x0010
	mbIconInformation = 0x0040
)

// Monitor / DPI.
const (
	monitorDefaultToNearest = 2
	mdtEffectiveDpi         = 0
)

// dpiAwarenessContextPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT -4.
const dpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

// GDI text/font.
const (
	bkModeTransparent = 1

	dtCenter     = 0x0001
	dtVCenter    = 0x0004
	dtSingleLine = 0x0020

	fwBold            = 700
	defaultCharset    = 1
	outDefaultPrecis  = 0
	clipDefaultPrecis = 0
	cleartypeQuality  = 5
	defaultPitch      = 0
)

// Misc.
const (
	idiApplication = 32512
	idcArrow       = 32512

	pmNoRemove = 0

	errorAlreadyExists = 183

	loadLibrarySearchSystem32 = 0x00000800
)

// KBDLLHOOKSTRUCT.
type kbdllHookStruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

// KEYBDINPUT. The explicit padding keeps dwExtraInfo 8-byte aligned exactly
// as the Windows SDK lays it out on amd64.
type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	_           uint32
	dwExtraInfo uintptr
}

// INPUT restricted to the keyboard member of the union. The union itself is
// 32 bytes on amd64 (MOUSEINPUT is the largest member), hence the trailing
// padding after the 24-byte KEYBDINPUT.
type inputStruct struct {
	inputType uint32
	_         uint32
	ki        keybdInput
	_         [8]byte
}

type point struct {
	x int32
	y int32
}

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

func (r rect) width() int32  { return r.right - r.left }
func (r rect) height() int32 { return r.bottom - r.top }

// MSG.
type msgStruct struct {
	hwnd    uintptr
	message uint32
	_       uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

// WNDCLASSEXW.
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

// MONITORINFO.
type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

// PAINTSTRUCT.
type paintStruct struct {
	hdc         uintptr
	fErase      int32
	rcPaint     rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

// GUID.
type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

// NOTIFYICONDATAW (Windows Vista+ layout; 976 bytes on amd64).
type notifyIconDataW struct {
	cbSize           uint32
	_                uint32
	hWnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	_                uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32 // union with uTimeout
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         guid
	hBalloonIcon     uintptr
}
