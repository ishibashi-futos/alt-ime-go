package win32

// Win32 constants and struct layouts used by the bindings in win32.go.
// This file has no build constraint on purpose: it contains no syscalls, so
// the layout tests in types_test.go can run on any 64-bit host, not
// only under GOOS=windows. All layouts target windows/amd64.

// OwnInputTag marks every KEYBDINPUT.dwExtraInfo this process injects
// (IME keys and the menu-focus suppressor). The hook treats an event as
// self-injected only when LLKHF_INJECTED is set AND dwExtraInfo matches.
// It is an ownership marker, not a security boundary. ASCII "ALTIME1".
const OwnInputTag uintptr = 0x414C54494D4531

// Virtual-key codes.
const (
	VkLButton            = 0x01
	VkRButton            = 0x02
	VkCancel             = 0x03
	VkMButton            = 0x04
	VkXButton1           = 0x05
	VkXButton2           = 0x06
	VkMenuSuppressLegacy = 0x07 // unassigned VK for Win32-style menu masking
	VkMenuSuppressDOM    = 0x87 // VK_F24: visible to Electron/Chromium and DOM
	VkReturn             = 0x0D
	VkShift              = 0x10
	VkControl            = 0x11
	VkMenu               = 0x12
	VkKana               = 0x15 // VK_HANGUL/VK_KANA
	VkImeOn              = 0x16
	VkKanji              = 0x19 // VK_HANJA/VK_KANJI
	VkImeOff             = 0x1A
	VkEscape             = 0x1B
	VkLWin               = 0x5B
	VkRWin               = 0x5C
	VkOemAuto            = 0xF3 // 半角/全角 (auto)
	VkOemEnlw            = 0xF4 // 半角/全角 (enlw)
	VkLShift             = 0xA0
	VkRShift             = 0xA1
	VkLControl           = 0xA2
	VkRControl           = 0xA3
	VkLMenu              = 0xA4
	VkRMenu              = 0xA5
)

// Low-level keyboard hook.
const (
	WhKeyboardLL = 13
	HcAction     = 0

	WmKeyDown    = 0x0100
	WmKeyUp      = 0x0101
	WmSysKeyDown = 0x0104
	WmSysKeyUp   = 0x0105

	LlkhfExtended = 0x01
	LlkhfInjected = 0x10
	LlkhfUp       = 0x80
)

// SendInput.
const (
	InputKeyboard        = 1
	KeyEventFExtendedKey = 0x0001
	KeyEventFKeyUp       = 0x0002

	MapvkVKToVSC = 0 // MapVirtualKeyW: virtual key -> scan code
)

// WinEvent hook (foreground tracking for the Enter guard).
const (
	EventSystemForeground = 0x0003
	WineventOutOfContext  = 0x0000
)

// OpenProcess access right for QueryFullProcessImageNameW.
const ProcessQueryLimitedInformation = 0x1000

// Window messages.
const (
	WmNull             = 0x0000
	WmDestroy          = 0x0002
	WmPaint            = 0x000F
	WmClose            = 0x0010
	WmContextMenu      = 0x007B
	WmTimer            = 0x0113
	WmPowerBroadcast   = 0x0218
	WmImeControl       = 0x0283
	WmWtsSessionChange = 0x02B1
	WmDpiChanged       = 0x02E0
	WmUser             = 0x0400
	WmApp              = 0x8000
)

// IMM32 / WM_IME_CONTROL.
const (
	ImcGetOpenStatus = 0x0005
	ImcSetOpenStatus = 0x0006

	SmtoBlock       = 0x0001
	SmtoAbortIfHung = 0x0002
)

// Session / power notifications.
const (
	WtsSessionLock        = 0x7
	WtsSessionUnlock      = 0x8
	NotifyForThisSession  = 0
	PbtApmResumeSuspend   = 0x0007
	PbtApmResumeAutomatic = 0x0012
)

// Shell_NotifyIconW.
const (
	NimAdd        = 0x0000
	NimModify     = 0x0001
	NimDelete     = 0x0002
	NimSetFocus   = 0x0003
	NimSetVersion = 0x0004

	NifMessage = 0x0001
	NifIcon    = 0x0002
	NifTip     = 0x0004
	NifShowTip = 0x0080

	NotifyIconVersion4 = 4

	NinSelect    = WmUser + 0
	NinKeySelect = WmUser + 1
)

// Window styles and show/positioning flags.
const (
	WsOverlapped = 0x00000000
	WsPopup      = 0x80000000

	WsExTopmost     = 0x00000008
	WsExTransparent = 0x00000020
	WsExToolWindow  = 0x00000080
	WsExLayered     = 0x00080000
	WsExNoActivate  = 0x08000000

	SwHide           = 0
	SwShowNoActivate = 4

	SwpNoActivate = 0x0010

	LwaAlpha = 0x0002

	CsVRedraw = 0x0001
	CsHRedraw = 0x0002
)

// HwndTopmost is the HWND_TOPMOST pseudo-handle (-1).
const HwndTopmost = ^uintptr(0)

// Menus.
const (
	MfString    = 0x0000
	MfUnchecked = 0x0000
	MfChecked   = 0x0008
	MfByCommand = 0x0000

	TpmRightButton = 0x0002
	TpmNoNotify    = 0x0080
	TpmReturnCmd   = 0x0100
)

// MessageBox.
const (
	MbOK              = 0x0000
	MbIconError       = 0x0010
	MbIconInformation = 0x0040
)

// Monitor / DPI.
const (
	MonitorDefaultToNearest = 2
	MdtEffectiveDpi         = 0
)

// DpiAwarenessContextPerMonitorAwareV2 is DPI_AWARENESS_CONTEXT -4.
const DpiAwarenessContextPerMonitorAwareV2 = ^uintptr(3)

// GDI text/font.
const (
	BkModeTransparent = 1

	DtCenter     = 0x0001
	DtVCenter    = 0x0004
	DtSingleLine = 0x0020

	FwBold            = 700
	DefaultCharset    = 1
	OutDefaultPrecis  = 0
	ClipDefaultPrecis = 0
	CleartypeQuality  = 5
	DefaultPitch      = 0
)

// Misc.
const (
	IdcArrow = 32512

	PmNoRemove = 0

	ErrorAlreadyExists = 183

	LoadLibrarySearchSystem32 = 0x00000800
)

// KBDLLHOOKSTRUCT.
type KbdllHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// KEYBDINPUT. The explicit padding keeps dwExtraInfo 8-byte aligned exactly
// as the Windows SDK lays it out on amd64.
type KeybdInput struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	_           uint32
	DwExtraInfo uintptr
}

// INPUT restricted to the keyboard member of the union. The union itself is
// 32 bytes on amd64 (MOUSEINPUT is the largest member), hence the trailing
// padding after the 24-byte KEYBDINPUT.
type InputStruct struct {
	InputType uint32
	_         uint32
	Ki        KeybdInput
	_         [8]byte
}

type Point struct {
	X int32
	Y int32
}

type Rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

func (r Rect) Width() int32  { return r.Right - r.Left }
func (r Rect) Height() int32 { return r.Bottom - r.Top }

// MSG.
type MsgStruct struct {
	Hwnd    uintptr
	Message uint32
	_       uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      Point
}

// WNDCLASSEXW.
type WndClassExW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

// MONITORINFO.
type MonitorInfo struct {
	CbSize    uint32
	RcMonitor Rect
	RcWork    Rect
	DwFlags   uint32
}

// PAINTSTRUCT.
type PaintStruct struct {
	Hdc         uintptr
	FErase      int32
	RcPaint     Rect
	FRestore    int32
	FIncUpdate  int32
	RgbReserved [32]byte
}

// GUITHREADINFO.
type GuiThreadInfo struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    uintptr
	HwndFocus     uintptr
	HwndCapture   uintptr
	HwndMenuOwner uintptr
	HwndMoveSize  uintptr
	HwndCaret     uintptr
	RcCaret       Rect
}

// GUID.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

// NOTIFYICONDATAW (Windows Vista+ layout; 976 bytes on amd64).
type NotifyIconDataW struct {
	CbSize           uint32
	_                uint32
	HWnd             uintptr
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	_                uint32
	HIcon            uintptr
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32 // union with uTimeout
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         GUID
	HBalloonIcon     uintptr
}
