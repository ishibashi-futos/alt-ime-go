//go:build windows

package win32

// Direct Win32 bindings over the standard syscall package (no third-party
// dependencies, NFR-3). System DLLs are loaded with LoadLibraryExW and
// LOAD_LIBRARY_SEARCH_SYSTEM32 so the application directory is never
// searched. Every proc is resolved once at startup by Load; a missing
// export is a fatal initialization error (NFR-10), never a silent nil call.

import (
	"fmt"
	"syscall"
	"unsafe"
)

type winProc struct {
	name string
	addr uintptr
}

func (p *winProc) call(args ...uintptr) (uintptr, syscall.Errno) {
	r1, _, errno := syscall.SyscallN(p.addr, args...)
	return r1, errno
}

var (
	// kernel32
	procLoadLibraryExW            winProc
	procGetModuleHandleW          winProc
	procGetCurrentThreadId        winProc
	procOutputDebugStringW        winProc
	procGetTickCount64            winProc
	procCreateMutexW              winProc
	procQueryPerformanceCounter   winProc
	procQueryPerformanceFrequency winProc
	procOpenProcess               winProc
	procQueryFullProcessImageName winProc

	// user32
	procRegisterClassExW              winProc
	procCreateWindowExW               winProc
	procDestroyWindow                 winProc
	procDefWindowProcW                winProc
	procShowWindow                    winProc
	procSetWindowPos                  winProc
	procSetLayeredWindowAttributes    winProc
	procSetWindowRgn                  winProc
	procInvalidateRect                winProc
	procValidateRect                  winProc
	procBeginPaint                    winProc
	procEndPaint                      winProc
	procFillRect                      winProc
	procDrawTextW                     winProc
	procGetMessageW                   winProc
	procPeekMessageW                  winProc
	procTranslateMessage              winProc
	procDispatchMessageW              winProc
	procPostMessageW                  winProc
	procPostThreadMessageW            winProc
	procPostQuitMessage               winProc
	procRegisterWindowMessageW        winProc
	procSetWindowsHookExW             winProc
	procUnhookWindowsHookEx           winProc
	procCallNextHookEx                winProc
	procSetWinEventHook               winProc
	procUnhookWinEvent                winProc
	procGetWindowThreadProcessId      winProc
	procGetGUIThreadInfo              winProc
	procMapVirtualKeyW                winProc
	procGetForegroundWindow           winProc
	procSetForegroundWindow           winProc
	procIsWindow                      winProc
	procGetAsyncKeyState              winProc
	procSendInput                     winProc
	procSetTimer                      winProc
	procKillTimer                     winProc
	procLoadIconW                     winProc
	procLoadCursorW                   winProc
	procCreatePopupMenu               winProc
	procDestroyMenu                   winProc
	procAppendMenuW                   winProc
	procCheckMenuItem                 winProc
	procTrackPopupMenuEx              winProc
	procMonitorFromWindow             winProc
	procGetMonitorInfoW               winProc
	procGetDpiForWindow               winProc
	procSetProcessDpiAwarenessContext winProc
	procSendMessageTimeoutW           winProc
	procMessageBoxW                   winProc

	// gdi32
	procCreateFontW        winProc
	procCreateSolidBrush   winProc
	procCreateRoundRectRgn winProc
	procDeleteObject       winProc
	procSelectObject       winProc
	procSetTextColor       winProc
	procSetBkMode          winProc

	// shell32
	procShellNotifyIconW winProc

	// imm32
	procImmGetDefaultIMEWnd winProc

	// wtsapi32
	procWTSRegisterSessionNotification   winProc
	procWTSUnRegisterSessionNotification winProc

	// shcore
	procGetDpiForMonitor winProc
)

type procBinding struct {
	dll  string
	name string
	out  *winProc
}

var procBindings = []procBinding{
	{"kernel32.dll", "GetModuleHandleW", &procGetModuleHandleW},
	{"kernel32.dll", "GetCurrentThreadId", &procGetCurrentThreadId},
	{"kernel32.dll", "OutputDebugStringW", &procOutputDebugStringW},
	{"kernel32.dll", "GetTickCount64", &procGetTickCount64},
	{"kernel32.dll", "CreateMutexW", &procCreateMutexW},
	{"kernel32.dll", "QueryPerformanceCounter", &procQueryPerformanceCounter},
	{"kernel32.dll", "QueryPerformanceFrequency", &procQueryPerformanceFrequency},
	{"kernel32.dll", "OpenProcess", &procOpenProcess},
	{"kernel32.dll", "QueryFullProcessImageNameW", &procQueryFullProcessImageName},

	{"user32.dll", "RegisterClassExW", &procRegisterClassExW},
	{"user32.dll", "CreateWindowExW", &procCreateWindowExW},
	{"user32.dll", "DestroyWindow", &procDestroyWindow},
	{"user32.dll", "DefWindowProcW", &procDefWindowProcW},
	{"user32.dll", "ShowWindow", &procShowWindow},
	{"user32.dll", "SetWindowPos", &procSetWindowPos},
	{"user32.dll", "SetLayeredWindowAttributes", &procSetLayeredWindowAttributes},
	{"user32.dll", "SetWindowRgn", &procSetWindowRgn},
	{"user32.dll", "InvalidateRect", &procInvalidateRect},
	{"user32.dll", "ValidateRect", &procValidateRect},
	{"user32.dll", "BeginPaint", &procBeginPaint},
	{"user32.dll", "EndPaint", &procEndPaint},
	{"user32.dll", "FillRect", &procFillRect},
	{"user32.dll", "DrawTextW", &procDrawTextW},
	{"user32.dll", "GetMessageW", &procGetMessageW},
	{"user32.dll", "PeekMessageW", &procPeekMessageW},
	{"user32.dll", "TranslateMessage", &procTranslateMessage},
	{"user32.dll", "DispatchMessageW", &procDispatchMessageW},
	{"user32.dll", "PostMessageW", &procPostMessageW},
	{"user32.dll", "PostThreadMessageW", &procPostThreadMessageW},
	{"user32.dll", "PostQuitMessage", &procPostQuitMessage},
	{"user32.dll", "RegisterWindowMessageW", &procRegisterWindowMessageW},
	{"user32.dll", "SetWindowsHookExW", &procSetWindowsHookExW},
	{"user32.dll", "UnhookWindowsHookEx", &procUnhookWindowsHookEx},
	{"user32.dll", "CallNextHookEx", &procCallNextHookEx},
	{"user32.dll", "SetWinEventHook", &procSetWinEventHook},
	{"user32.dll", "UnhookWinEvent", &procUnhookWinEvent},
	{"user32.dll", "GetWindowThreadProcessId", &procGetWindowThreadProcessId},
	{"user32.dll", "GetGUIThreadInfo", &procGetGUIThreadInfo},
	{"user32.dll", "MapVirtualKeyW", &procMapVirtualKeyW},
	{"user32.dll", "GetForegroundWindow", &procGetForegroundWindow},
	{"user32.dll", "SetForegroundWindow", &procSetForegroundWindow},
	{"user32.dll", "IsWindow", &procIsWindow},
	{"user32.dll", "GetAsyncKeyState", &procGetAsyncKeyState},
	{"user32.dll", "SendInput", &procSendInput},
	{"user32.dll", "SetTimer", &procSetTimer},
	{"user32.dll", "KillTimer", &procKillTimer},
	{"user32.dll", "LoadIconW", &procLoadIconW},
	{"user32.dll", "LoadCursorW", &procLoadCursorW},
	{"user32.dll", "CreatePopupMenu", &procCreatePopupMenu},
	{"user32.dll", "DestroyMenu", &procDestroyMenu},
	{"user32.dll", "AppendMenuW", &procAppendMenuW},
	{"user32.dll", "CheckMenuItem", &procCheckMenuItem},
	{"user32.dll", "TrackPopupMenuEx", &procTrackPopupMenuEx},
	{"user32.dll", "MonitorFromWindow", &procMonitorFromWindow},
	{"user32.dll", "GetMonitorInfoW", &procGetMonitorInfoW},
	{"user32.dll", "GetDpiForWindow", &procGetDpiForWindow},
	{"user32.dll", "SetProcessDpiAwarenessContext", &procSetProcessDpiAwarenessContext},
	{"user32.dll", "SendMessageTimeoutW", &procSendMessageTimeoutW},
	{"user32.dll", "MessageBoxW", &procMessageBoxW},

	{"gdi32.dll", "CreateFontW", &procCreateFontW},
	{"gdi32.dll", "CreateSolidBrush", &procCreateSolidBrush},
	{"gdi32.dll", "CreateRoundRectRgn", &procCreateRoundRectRgn},
	{"gdi32.dll", "DeleteObject", &procDeleteObject},
	{"gdi32.dll", "SelectObject", &procSelectObject},
	{"gdi32.dll", "SetTextColor", &procSetTextColor},
	{"gdi32.dll", "SetBkMode", &procSetBkMode},

	{"shell32.dll", "Shell_NotifyIconW", &procShellNotifyIconW},

	{"imm32.dll", "ImmGetDefaultIMEWnd", &procImmGetDefaultIMEWnd},

	{"wtsapi32.dll", "WTSRegisterSessionNotification", &procWTSRegisterSessionNotification},
	{"wtsapi32.dll", "WTSUnRegisterSessionNotification", &procWTSUnRegisterSessionNotification},

	{"shcore.dll", "GetDpiForMonitor", &procGetDpiForMonitor},
}

// Load resolves every binding. kernel32 is loaded first with the plain
// loader, which is safe because it is mapped into every process before any
// Go code runs; all other DLLs go through LoadLibraryExW with
// LOAD_LIBRARY_SEARCH_SYSTEM32.
func Load() error {
	k32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return fmt.Errorf("load kernel32.dll: %w", err)
	}
	addr, err := syscall.GetProcAddress(k32, "LoadLibraryExW")
	if err != nil {
		return fmt.Errorf("kernel32!LoadLibraryExW: %w", err)
	}
	procLoadLibraryExW = winProc{name: "LoadLibraryExW", addr: addr}

	handles := map[string]syscall.Handle{"kernel32.dll": k32}
	for _, b := range procBindings {
		h, ok := handles[b.dll]
		if !ok {
			name, err := syscall.UTF16PtrFromString(b.dll)
			if err != nil {
				return err
			}
			r, errno := procLoadLibraryExW.call(uintptr(unsafe.Pointer(name)), 0, LoadLibrarySearchSystem32)
			if r == 0 {
				return fmt.Errorf("LoadLibraryExW(%s): %w", b.dll, errno)
			}
			h = syscall.Handle(r)
			handles[b.dll] = h
		}
		addr, err := syscall.GetProcAddress(h, b.name)
		if err != nil {
			return fmt.Errorf("%s!%s: %w", b.dll, b.name, err)
		}
		*b.out = winProc{name: b.name, addr: addr}
	}
	return nil
}

// MustUTF16 converts a compile-time literal; interior NULs are programmer
// errors, hence the panic.
func MustUTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func CopyUTF16(dst []uint16, s string) {
	src, err := syscall.UTF16FromString(s) // NUL-terminated
	if err != nil {
		dst[0] = 0
		return
	}
	n := copy(dst, src)
	dst[min(n, len(dst)-1)] = 0
}

// Debugf writes a diagnostic line to OutputDebugStringW. GUI-subsystem
// builds have no console, so this is the only always-available channel.
// Never call it from the hook callback (NFR-2).
func Debugf(format string, args ...any) {
	msg := "[alt-ime] " + fmt.Sprintf(format, args...)
	p, err := syscall.UTF16PtrFromString(msg)
	if err != nil {
		return
	}
	procOutputDebugStringW.call(uintptr(unsafe.Pointer(p)))
}

// ---- kernel32 ----

func GetModuleHandle() uintptr {
	r, _ := procGetModuleHandleW.call(0)
	return r
}

func GetCurrentThreadId() uint32 {
	r, _ := procGetCurrentThreadId.call()
	return uint32(r)
}

func GetTickCount64() uint64 {
	r, _ := procGetTickCount64.call()
	return uint64(r)
}

func CreateMutex(name string) (syscall.Handle, syscall.Errno) {
	r, errno := procCreateMutexW.call(0, 0, uintptr(unsafe.Pointer(MustUTF16(name))))
	return syscall.Handle(r), errno
}

// CloseMutex releases the single-instance mutex. The handle was created
// with bInitialOwner=FALSE and never acquired, so ReleaseMutex would be an
// error; closing the handle is the correct teardown.
func CloseMutex(h syscall.Handle) {
	if h != 0 {
		syscall.CloseHandle(h)
	}
}

func QueryPerformanceCounter() uint64 {
	var v uint64
	procQueryPerformanceCounter.call(uintptr(unsafe.Pointer(&v)))
	return v
}

func QueryPerformanceFrequency() uint64 {
	var v uint64
	procQueryPerformanceFrequency.call(uintptr(unsafe.Pointer(&v)))
	return v
}

// ---- windows, classes, message loop ----

func RegisterClass(class string, wndProc uintptr, hInstance uintptr) error {
	wc := WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(WndClassExW{})),
		Style:         CsHRedraw | CsVRedraw,
		LpfnWndProc:   wndProc,
		HInstance:     hInstance,
		HCursor:       loadCursor(0, IdcArrow),
		LpszClassName: MustUTF16(class),
	}
	r, errno := procRegisterClassExW.call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return fmt.Errorf("RegisterClassExW(%s): %w", class, errno)
	}
	return nil
}

func CreateWindow(exStyle uintptr, class, title string, style uintptr, x, y, w, h int32, parent, hInstance uintptr) (uintptr, syscall.Errno) {
	return procCreateWindowExW.call(
		exStyle,
		uintptr(unsafe.Pointer(MustUTF16(class))),
		uintptr(unsafe.Pointer(MustUTF16(title))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, 0, hInstance, 0,
	)
}

func DestroyWindow(hwnd uintptr) {
	procDestroyWindow.call(hwnd)
}

func DefWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _ := procDefWindowProcW.call(hwnd, msg, wParam, lParam)
	return r
}

func ShowWindow(hwnd uintptr, cmd int32) {
	procShowWindow.call(hwnd, uintptr(cmd))
}

func SetWindowPos(hwnd, insertAfter uintptr, x, y, w, h int32, flags uint32) bool {
	r, _ := procSetWindowPos.call(hwnd, insertAfter, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
	return r != 0
}

func SetLayeredWindowAttributes(hwnd uintptr, colorKey uint32, alpha byte, flags uint32) bool {
	r, _ := procSetLayeredWindowAttributes.call(hwnd, uintptr(colorKey), uintptr(alpha), uintptr(flags))
	return r != 0
}

// SetWindowRgn transfers region ownership to the system when it succeeds;
// the caller must delete the region only on failure.
func SetWindowRgn(hwnd, rgn uintptr, redraw bool) bool {
	r, _ := procSetWindowRgn.call(hwnd, rgn, BoolToUintptr(redraw))
	return r != 0
}

func InvalidateRect(hwnd uintptr) {
	procInvalidateRect.call(hwnd, 0, 1)
}

func ValidateRect(hwnd uintptr) {
	procValidateRect.call(hwnd, 0)
}

func BeginPaint(hwnd uintptr, ps *PaintStruct) uintptr {
	r, _ := procBeginPaint.call(hwnd, uintptr(unsafe.Pointer(ps)))
	return r
}

func EndPaint(hwnd uintptr, ps *PaintStruct) {
	procEndPaint.call(hwnd, uintptr(unsafe.Pointer(ps)))
}

func FillRect(hdc uintptr, rc *Rect, brush uintptr) {
	procFillRect.call(hdc, uintptr(unsafe.Pointer(rc)), brush)
}

func DrawText(hdc uintptr, text *uint16, rc *Rect, format uint32) {
	procDrawTextW.call(hdc, uintptr(unsafe.Pointer(text)), ^uintptr(0) /* -1: NUL-terminated */, uintptr(unsafe.Pointer(rc)), uintptr(format))
}

func GetMessage(m *MsgStruct) int32 {
	r, _ := procGetMessageW.call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(uint32(r))
}

func PeekMessage(m *MsgStruct, flags uint32) bool {
	r, _ := procPeekMessageW.call(uintptr(unsafe.Pointer(m)), 0, 0, 0, uintptr(flags))
	return r != 0
}

func TranslateMessage(m *MsgStruct) {
	procTranslateMessage.call(uintptr(unsafe.Pointer(m)))
}

func DispatchMessage(m *MsgStruct) {
	procDispatchMessageW.call(uintptr(unsafe.Pointer(m)))
}

func PostMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	r, _ := procPostMessageW.call(hwnd, uintptr(msg), wParam, lParam)
	return r != 0
}

func PostThreadMessage(tid uint32, msg uint32, wParam, lParam uintptr) bool {
	r, _ := procPostThreadMessageW.call(uintptr(tid), uintptr(msg), wParam, lParam)
	return r != 0
}

func PostQuitMessage(code int32) {
	procPostQuitMessage.call(uintptr(code))
}

func RegisterWindowMessage(name string) uint32 {
	r, _ := procRegisterWindowMessageW.call(uintptr(unsafe.Pointer(MustUTF16(name))))
	return uint32(r)
}

// ---- hooks and input ----

func SetWindowsHookEx(id int32, fn uintptr, hMod uintptr, threadID uint32) (uintptr, error) {
	r, errno := procSetWindowsHookExW.call(uintptr(id), fn, hMod, uintptr(threadID))
	if r == 0 {
		return 0, fmt.Errorf("SetWindowsHookExW: %w", errno)
	}
	return r, nil
}

func UnhookWindowsHookEx(hhook uintptr) bool {
	r, _ := procUnhookWindowsHookEx.call(hhook)
	return r != 0
}

func CallNextHookEx(nCode, wParam, lParam uintptr) uintptr {
	r, _ := procCallNextHookEx.call(0, nCode, wParam, lParam)
	return r
}

// SetWinEventHook registers an out-of-context WinEvent hook for one event.
// The callback is delivered through the registering thread's message pump.
func SetWinEventHook(event uint32, fn uintptr) uintptr {
	r, _ := procSetWinEventHook.call(uintptr(event), uintptr(event), 0, fn, 0, 0, WineventOutOfContext)
	return r
}

func UnhookWinEvent(hhook uintptr) bool {
	r, _ := procUnhookWinEvent.call(hhook)
	return r != 0
}

func WindowProcessId(hwnd uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func WindowThreadId(hwnd uintptr) uint32 {
	r, _ := procGetWindowThreadProcessId.call(hwnd, 0)
	return uint32(r)
}

// GuiFocusWindow returns the window with keyboard focus on the given thread
// (0 = the foreground thread), or 0 when it cannot be determined. This is
// the documented cross-process way to find the real focus owner; for
// WebView2/Chromium hosts it is a child window on a different thread (often
// a different process) than the top-level window.
func GuiFocusWindow(tid uint32) uintptr {
	var gti GuiThreadInfo
	gti.CbSize = uint32(unsafe.Sizeof(gti))
	if r, _ := procGetGUIThreadInfo.call(uintptr(tid), uintptr(unsafe.Pointer(&gti))); r == 0 {
		return 0
	}
	return gti.HwndFocus
}

// ProcessImagePath resolves the full Win32 image path of a process. It uses
// PROCESS_QUERY_LIMITED_INFORMATION so it also works across integrity levels
// where broader access would be denied; UIPI-protected processes may still
// fail, which callers treat as "not a guard target".
func ProcessImagePath(pid uint32) (string, bool) {
	if pid == 0 {
		return "", false
	}
	h, _ := procOpenProcess.call(ProcessQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return "", false
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	var buf [512]uint16
	size := uint32(len(buf))
	r, _ := procQueryFullProcessImageName.call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 || size == 0 || size >= uint32(len(buf)) {
		return "", false
	}
	return syscall.UTF16ToString(buf[:size]), true
}

func GetForegroundWindow() uintptr {
	r, _ := procGetForegroundWindow.call()
	return r
}

func SetForegroundWindow(hwnd uintptr) {
	procSetForegroundWindow.call(hwnd)
}

func IsWindow(hwnd uintptr) bool {
	r, _ := procIsWindow.call(hwnd)
	return r != 0
}

func GetAsyncKeyStateDown(vk uint32) bool {
	r, _ := procGetAsyncKeyState.call(uintptr(vk))
	return uint16(r)&0x8000 != 0
}

// SendKeyPair injects one tagged down/up pair for vk in a single SendInput
// call and returns the number of events actually inserted (2 on success).
func SendKeyPair(vk uint16) (uint32, syscall.Errno) {
	inputs := [2]InputStruct{
		{InputType: InputKeyboard, Ki: KeybdInput{WVk: vk, DwExtraInfo: OwnInputTag}},
		{InputType: InputKeyboard, Ki: KeybdInput{WVk: vk, DwFlags: KeyEventFKeyUp, DwExtraInfo: OwnInputTag}},
	}
	n, errno := procSendInput.call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), errno
}

// SendKeyUp inserts a tagged release used as best-effort cleanup after a
// short key-pair insertion. An unmatched key-up is harmless, while omitting
// it could leave an assigned suppressor key logically down in the target.
func SendKeyUp(vk uint16) (uint32, syscall.Errno) {
	input := InputStruct{
		InputType: InputKeyboard,
		Ki:        KeybdInput{WVk: vk, DwFlags: KeyEventFKeyUp, DwExtraInfo: OwnInputTag},
	}
	n, errno := procSendInput.call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	return uint32(n), errno
}

// extendedFlagFor marks the right-side modifiers that live in the extended
// scan-code range, so an injected VK is not folded back onto its left twin.
func extendedFlagFor(vk uint16) uint32 {
	switch vk {
	case VkRControl, VkRMenu:
		return KeyEventFExtendedKey
	}
	return 0
}

// guardKeyInput builds one tagged keyboard input for the guard replacements.
// Unlike the suppressor/IME injections it also carries the real scan code:
// Chromium/WebView2 derives the DOM `code` value from the scan code, and a
// zero-scan-code Enter can be ignored by web keyboard handlers in exactly
// the applications the guard targets.
func guardKeyInput(vk uint16, up bool) InputStruct {
	flags := extendedFlagFor(vk)
	if up {
		flags |= KeyEventFKeyUp
	}
	scan, _ := procMapVirtualKeyW.call(uintptr(vk), MapvkVKToVSC)
	return InputStruct{
		InputType: InputKeyboard,
		Ki:        KeybdInput{WVk: vk, WScan: uint16(scan), DwFlags: flags, DwExtraInfo: OwnInputTag},
	}
}

// SendKeyDown inserts a tagged press used as best-effort recovery when a
// short guard insertion may have left a physically held modifier logically
// released. A duplicate key-down over an already-down key is harmless.
func SendKeyDown(vk uint16) (uint32, syscall.Errno) {
	input := guardKeyInput(vk, false)
	n, errno := procSendInput.call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	return uint32(n), errno
}

// SendShiftEnter injects the newline replacement for a guarded plain Enter:
// a tagged Shift+Enter chord in one SendInput call. Returns the number of
// events actually inserted (4 on success).
func SendShiftEnter() (uint32, syscall.Errno) {
	inputs := [4]InputStruct{
		guardKeyInput(VkLShift, false),
		guardKeyInput(VkReturn, false),
		guardKeyInput(VkReturn, true),
		guardKeyInput(VkLShift, true),
	}
	n, errno := procSendInput.call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), errno
}

// SendEnterBypassingCtrl injects a plain Enter: release the physically held
// Ctrl side(s), tap Enter, then press them again, all tagged and in one
// SendInput call, so the target observes a plain Enter while the physical
// Ctrl state is preserved. With neither side reported down it degrades to a
// bare Enter tap. Returns the expected and actually inserted event counts.
func SendEnterBypassingCtrl(lctrl, rctrl bool) (want, got uint32, errno syscall.Errno) {
	var inputs [6]InputStruct
	n := 0
	add := func(vk uint16, up bool) {
		inputs[n] = guardKeyInput(vk, up)
		n++
	}
	if lctrl {
		add(VkLControl, true)
	}
	if rctrl {
		add(VkRControl, true)
	}
	add(VkReturn, false)
	add(VkReturn, true)
	if rctrl {
		add(VkRControl, false)
	}
	if lctrl {
		add(VkLControl, false)
	}
	r, errno := procSendInput.call(uintptr(n), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), uint32(r), errno
}

func SetTimer(hwnd uintptr, id uintptr, ms uint32) bool {
	r, _ := procSetTimer.call(hwnd, id, uintptr(ms), 0)
	return r != 0
}

func KillTimer(hwnd uintptr, id uintptr) {
	procKillTimer.call(hwnd, id)
}

// ---- icons, cursors, menus ----

func LoadIcon(hInstance uintptr, id uintptr) (uintptr, syscall.Errno) {
	return procLoadIconW.call(hInstance, id)
}

func loadCursor(hInstance uintptr, id uintptr) uintptr {
	r, _ := procLoadCursorW.call(hInstance, id)
	return r
}

func CreatePopupMenu() uintptr {
	r, _ := procCreatePopupMenu.call()
	return r
}

func DestroyMenu(menu uintptr) {
	procDestroyMenu.call(menu)
}

func AppendMenu(menu uintptr, flags uint32, id uintptr, text string) bool {
	r, _ := procAppendMenuW.call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(MustUTF16(text))))
	return r != 0
}

func CheckMenuItem(menu uintptr, id uintptr, flags uint32) {
	procCheckMenuItem.call(menu, id, uintptr(flags))
}

func TrackPopupMenuEx(menu uintptr, flags uint32, x, y int32, hwnd uintptr) uintptr {
	r, _ := procTrackPopupMenuEx.call(menu, uintptr(flags), uintptr(x), uintptr(y), hwnd, 0)
	return r
}

// ---- monitors and DPI ----

func MonitorFromWindow(hwnd uintptr, flags uint32) uintptr {
	r, _ := procMonitorFromWindow.call(hwnd, uintptr(flags))
	return r
}

func GetMonitorInfo(mon uintptr, mi *MonitorInfo) bool {
	mi.CbSize = uint32(unsafe.Sizeof(MonitorInfo{}))
	r, _ := procGetMonitorInfoW.call(mon, uintptr(unsafe.Pointer(mi)))
	return r != 0
}

func GetDpiForMonitor(mon uintptr) (uint32, bool) {
	var x, y uint32
	r, _ := procGetDpiForMonitor.call(mon, MdtEffectiveDpi, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	return x, r == 0 // S_OK
}

func GetDpiForWindow(hwnd uintptr) uint32 {
	r, _ := procGetDpiForWindow.call(hwnd)
	return uint32(r)
}

func SetPerMonitorV2() {
	r, errno := procSetProcessDpiAwarenessContext.call(DpiAwarenessContextPerMonitorAwareV2)
	if r == 0 {
		// Expected when the embedded manifest already applied PerMonitorV2
		// (ERROR_ACCESS_DENIED); anything else is worth a diagnostic too.
		Debugf("SetProcessDpiAwarenessContext: errno=%d (manifest usually already applied)", errno)
	}
}

// ---- messaging with deadlines ----

func SendMessageTimeout(hwnd uintptr, msg uint32, wParam, lParam uintptr, flags, timeoutMs uint32) (uintptr, bool, syscall.Errno) {
	var result uintptr
	r, errno := procSendMessageTimeoutW.call(hwnd, uintptr(msg), wParam, lParam, uintptr(flags), uintptr(timeoutMs), uintptr(unsafe.Pointer(&result)))
	return result, r != 0, errno
}

// MessageBox is a no-op when Load failed before user32 was resolved, so
// callers can report fatal startup errors unconditionally.
func MessageBox(hwnd uintptr, text, caption string, flags uint32) {
	if procMessageBoxW.addr == 0 {
		return
	}
	procMessageBoxW.call(hwnd, uintptr(unsafe.Pointer(MustUTF16(text))), uintptr(unsafe.Pointer(MustUTF16(caption))), uintptr(flags))
}

// ---- gdi32 ----

// CreateFont creates a bold ClearType HFONT of the given face and character
// height (the height is negated per the CreateFontW contract).
func CreateFont(face string, height int32) uintptr {
	r, _ := procCreateFontW.call(
		uintptr(-height), // negative: character height
		0, 0, 0,
		FwBold,
		0, 0, 0,
		DefaultCharset,
		OutDefaultPrecis,
		ClipDefaultPrecis,
		CleartypeQuality,
		DefaultPitch,
		uintptr(unsafe.Pointer(MustUTF16(face))),
	)
	return r
}

func CreateSolidBrush(color uint32) uintptr {
	r, _ := procCreateSolidBrush.call(uintptr(color))
	return r
}

func CreateRoundRectRgn(left, top, right, bottom, ellipseW, ellipseH int32) uintptr {
	r, _ := procCreateRoundRectRgn.call(uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(ellipseW), uintptr(ellipseH))
	return r
}

func DeleteObject(obj uintptr) {
	procDeleteObject.call(obj)
}

func SelectObject(hdc, obj uintptr) uintptr {
	r, _ := procSelectObject.call(hdc, obj)
	return r
}

func SetTextColor(hdc uintptr, color uint32) {
	procSetTextColor.call(hdc, uintptr(color))
}

func SetBkMode(hdc uintptr, mode int32) {
	procSetBkMode.call(hdc, uintptr(mode))
}

// ---- shell32 ----

func ShellNotifyIcon(action uint32, nid *NotifyIconDataW) bool {
	r, _ := procShellNotifyIconW.call(uintptr(action), uintptr(unsafe.Pointer(nid)))
	return r != 0
}

// ---- imm32 ----

func ImmGetDefaultIMEWnd(hwnd uintptr) uintptr {
	r, _ := procImmGetDefaultIMEWnd.call(hwnd)
	return r
}

// ---- wtsapi32 ----

func WtsRegisterSessionNotification(hwnd uintptr) bool {
	r, _ := procWTSRegisterSessionNotification.call(hwnd, NotifyForThisSession)
	return r != 0
}

func WtsUnRegisterSessionNotification(hwnd uintptr) {
	procWTSUnRegisterSessionNotification.call(hwnd)
}

// WinError wraps an API failure; errno 0 means the API reports failure only
// through its return value.
func WinError(api string, errno syscall.Errno) error {
	if errno == 0 {
		return fmt.Errorf("%s failed", api)
	}
	return fmt.Errorf("%s: %w", api, errno)
}

func BoolToUintptr(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
