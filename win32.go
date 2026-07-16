//go:build windows

package main

// Direct Win32 bindings over the standard syscall package (no third-party
// dependencies, NFR-3). System DLLs are loaded with LoadLibraryExW and
// LOAD_LIBRARY_SEARCH_SYSTEM32 so the application directory is never
// searched. Every proc is resolved once at startup by loadWin32; a missing
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

// loadWin32 resolves every binding. kernel32 is loaded first with the plain
// loader, which is safe because it is mapped into every process before any
// Go code runs; all other DLLs go through LoadLibraryExW with
// LOAD_LIBRARY_SEARCH_SYSTEM32.
func loadWin32() error {
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
			r, errno := procLoadLibraryExW.call(uintptr(unsafe.Pointer(name)), 0, loadLibrarySearchSystem32)
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

// mustUTF16 converts a compile-time literal; interior NULs are programmer
// errors, hence the panic.
func mustUTF16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}

func copyUTF16(dst []uint16, s string) {
	src, err := syscall.UTF16FromString(s) // NUL-terminated
	if err != nil {
		dst[0] = 0
		return
	}
	n := copy(dst, src)
	dst[min(n, len(dst)-1)] = 0
}

// debugf writes a diagnostic line to OutputDebugStringW. GUI-subsystem
// builds have no console, so this is the only always-available channel.
// Never call it from the hook callback (NFR-2).
func debugf(format string, args ...any) {
	msg := "[alt-ime] " + fmt.Sprintf(format, args...)
	p, err := syscall.UTF16PtrFromString(msg)
	if err != nil {
		return
	}
	procOutputDebugStringW.call(uintptr(unsafe.Pointer(p)))
}

// ---- kernel32 ----

func getModuleHandle() uintptr {
	r, _ := procGetModuleHandleW.call(0)
	return r
}

func getCurrentThreadId() uint32 {
	r, _ := procGetCurrentThreadId.call()
	return uint32(r)
}

func getTickCount64() uint64 {
	r, _ := procGetTickCount64.call()
	return uint64(r)
}

func createMutex(name string) (syscall.Handle, syscall.Errno) {
	r, errno := procCreateMutexW.call(0, 0, uintptr(unsafe.Pointer(mustUTF16(name))))
	return syscall.Handle(r), errno
}

// closeMutex releases the single-instance mutex. The handle was created
// with bInitialOwner=FALSE and never acquired, so ReleaseMutex would be an
// error; closing the handle is the correct teardown.
func closeMutex(h syscall.Handle) {
	if h != 0 {
		syscall.CloseHandle(h)
	}
}

func queryPerformanceCounter() uint64 {
	var v uint64
	procQueryPerformanceCounter.call(uintptr(unsafe.Pointer(&v)))
	return v
}

func queryPerformanceFrequency() uint64 {
	var v uint64
	procQueryPerformanceFrequency.call(uintptr(unsafe.Pointer(&v)))
	return v
}

// ---- windows, classes, message loop ----

func registerClass(class string, wndProc uintptr, hInstance uintptr) error {
	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		style:         csHRedraw | csVRedraw,
		lpfnWndProc:   wndProc,
		hInstance:     hInstance,
		hCursor:       loadCursor(0, idcArrow),
		lpszClassName: mustUTF16(class),
	}
	r, errno := procRegisterClassExW.call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return fmt.Errorf("RegisterClassExW(%s): %w", class, errno)
	}
	return nil
}

func createWindow(exStyle uintptr, class, title string, style uintptr, x, y, w, h int32, parent, hInstance uintptr) (uintptr, syscall.Errno) {
	return procCreateWindowExW.call(
		exStyle,
		uintptr(unsafe.Pointer(mustUTF16(class))),
		uintptr(unsafe.Pointer(mustUTF16(title))),
		style,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h),
		parent, 0, hInstance, 0,
	)
}

func destroyWindow(hwnd uintptr) {
	procDestroyWindow.call(hwnd)
}

func defWindowProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	r, _ := procDefWindowProcW.call(hwnd, msg, wParam, lParam)
	return r
}

func showWindow(hwnd uintptr, cmd int32) {
	procShowWindow.call(hwnd, uintptr(cmd))
}

func setWindowPos(hwnd, insertAfter uintptr, x, y, w, h int32, flags uint32) bool {
	r, _ := procSetWindowPos.call(hwnd, insertAfter, uintptr(x), uintptr(y), uintptr(w), uintptr(h), uintptr(flags))
	return r != 0
}

func setLayeredWindowAttributes(hwnd uintptr, colorKey uint32, alpha byte, flags uint32) bool {
	r, _ := procSetLayeredWindowAttributes.call(hwnd, uintptr(colorKey), uintptr(alpha), uintptr(flags))
	return r != 0
}

// setWindowRgn transfers region ownership to the system when it succeeds;
// the caller must delete the region only on failure.
func setWindowRgn(hwnd, rgn uintptr, redraw bool) bool {
	r, _ := procSetWindowRgn.call(hwnd, rgn, boolToUintptr(redraw))
	return r != 0
}

func invalidateRect(hwnd uintptr) {
	procInvalidateRect.call(hwnd, 0, 1)
}

func validateRect(hwnd uintptr) {
	procValidateRect.call(hwnd, 0)
}

func beginPaint(hwnd uintptr, ps *paintStruct) uintptr {
	r, _ := procBeginPaint.call(hwnd, uintptr(unsafe.Pointer(ps)))
	return r
}

func endPaint(hwnd uintptr, ps *paintStruct) {
	procEndPaint.call(hwnd, uintptr(unsafe.Pointer(ps)))
}

func fillRect(hdc uintptr, rc *rect, brush uintptr) {
	procFillRect.call(hdc, uintptr(unsafe.Pointer(rc)), brush)
}

func drawText(hdc uintptr, text *uint16, rc *rect, format uint32) {
	procDrawTextW.call(hdc, uintptr(unsafe.Pointer(text)), ^uintptr(0) /* -1: NUL-terminated */, uintptr(unsafe.Pointer(rc)), uintptr(format))
}

func getMessage(m *msgStruct) int32 {
	r, _ := procGetMessageW.call(uintptr(unsafe.Pointer(m)), 0, 0, 0)
	return int32(uint32(r))
}

func peekMessage(m *msgStruct, flags uint32) bool {
	r, _ := procPeekMessageW.call(uintptr(unsafe.Pointer(m)), 0, 0, 0, uintptr(flags))
	return r != 0
}

func translateMessage(m *msgStruct) {
	procTranslateMessage.call(uintptr(unsafe.Pointer(m)))
}

func dispatchMessage(m *msgStruct) {
	procDispatchMessageW.call(uintptr(unsafe.Pointer(m)))
}

func postMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	r, _ := procPostMessageW.call(hwnd, uintptr(msg), wParam, lParam)
	return r != 0
}

func postThreadMessage(tid uint32, msg uint32, wParam, lParam uintptr) bool {
	r, _ := procPostThreadMessageW.call(uintptr(tid), uintptr(msg), wParam, lParam)
	return r != 0
}

func postQuitMessage(code int32) {
	procPostQuitMessage.call(uintptr(code))
}

func registerWindowMessage(name string) uint32 {
	r, _ := procRegisterWindowMessageW.call(uintptr(unsafe.Pointer(mustUTF16(name))))
	return uint32(r)
}

// ---- hooks and input ----

func setWindowsHookEx(id int32, fn uintptr, hMod uintptr, threadID uint32) (uintptr, error) {
	r, errno := procSetWindowsHookExW.call(uintptr(id), fn, hMod, uintptr(threadID))
	if r == 0 {
		return 0, fmt.Errorf("SetWindowsHookExW: %w", errno)
	}
	return r, nil
}

func unhookWindowsHookEx(hhook uintptr) bool {
	r, _ := procUnhookWindowsHookEx.call(hhook)
	return r != 0
}

func callNextHookEx(nCode, wParam, lParam uintptr) uintptr {
	r, _ := procCallNextHookEx.call(0, nCode, wParam, lParam)
	return r
}

// setWinEventHook registers an out-of-context WinEvent hook for one event.
// The callback is delivered through the registering thread's message pump.
func setWinEventHook(event uint32, fn uintptr) uintptr {
	r, _ := procSetWinEventHook.call(uintptr(event), uintptr(event), 0, fn, 0, 0, wineventOutOfContext)
	return r
}

func unhookWinEvent(hhook uintptr) bool {
	r, _ := procUnhookWinEvent.call(hhook)
	return r != 0
}

func windowProcessId(hwnd uintptr) uint32 {
	var pid uint32
	procGetWindowThreadProcessId.call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// processImagePath resolves the full Win32 image path of a process. It uses
// PROCESS_QUERY_LIMITED_INFORMATION so it also works across integrity levels
// where broader access would be denied; UIPI-protected processes may still
// fail, which callers treat as "not a guard target".
func processImagePath(pid uint32) (string, bool) {
	if pid == 0 {
		return "", false
	}
	h, _ := procOpenProcess.call(processQueryLimitedInformation, 0, uintptr(pid))
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

func getForegroundWindow() uintptr {
	r, _ := procGetForegroundWindow.call()
	return r
}

func setForegroundWindow(hwnd uintptr) {
	procSetForegroundWindow.call(hwnd)
}

func isWindow(hwnd uintptr) bool {
	r, _ := procIsWindow.call(hwnd)
	return r != 0
}

func getAsyncKeyStateDown(vk uint32) bool {
	r, _ := procGetAsyncKeyState.call(uintptr(vk))
	return uint16(r)&0x8000 != 0
}

// sendKeyPair injects one tagged down/up pair for vk in a single SendInput
// call and returns the number of events actually inserted (2 on success).
func sendKeyPair(vk uint16) (uint32, syscall.Errno) {
	inputs := [2]inputStruct{
		{inputType: inputKeyboard, ki: keybdInput{wVk: vk, dwExtraInfo: ownInputTag}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vk, dwFlags: keyEventFKeyUp, dwExtraInfo: ownInputTag}},
	}
	n, errno := procSendInput.call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), errno
}

// sendKeyUp inserts a tagged release used as best-effort cleanup after a
// short key-pair insertion. An unmatched key-up is harmless, while omitting
// it could leave an assigned suppressor key logically down in the target.
func sendKeyUp(vk uint16) (uint32, syscall.Errno) {
	input := inputStruct{
		inputType: inputKeyboard,
		ki:        keybdInput{wVk: vk, dwFlags: keyEventFKeyUp, dwExtraInfo: ownInputTag},
	}
	n, errno := procSendInput.call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	return uint32(n), errno
}

// sendKeyDown inserts a tagged press used as best-effort recovery when a
// short guard insertion may have left a physically held modifier logically
// released. A duplicate key-down over an already-down key is harmless.
func sendKeyDown(vk uint16) (uint32, syscall.Errno) {
	input := inputStruct{
		inputType: inputKeyboard,
		ki:        keybdInput{wVk: vk, dwFlags: extendedFlagFor(vk), dwExtraInfo: ownInputTag},
	}
	n, errno := procSendInput.call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(input))
	return uint32(n), errno
}

// extendedFlagFor marks the right-side modifiers that live in the extended
// scan-code range, so an injected VK is not folded back onto its left twin.
func extendedFlagFor(vk uint16) uint32 {
	switch vk {
	case vkRControl, vkRMenu:
		return keyEventFExtendedKey
	}
	return 0
}

// sendShiftEnter injects the newline replacement for a guarded plain Enter:
// a tagged Shift+Enter chord in one SendInput call. Returns the number of
// events actually inserted (4 on success).
func sendShiftEnter() (uint32, syscall.Errno) {
	inputs := [4]inputStruct{
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkLShift, dwExtraInfo: ownInputTag}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkReturn, dwExtraInfo: ownInputTag}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkReturn, dwFlags: keyEventFKeyUp, dwExtraInfo: ownInputTag}},
		{inputType: inputKeyboard, ki: keybdInput{wVk: vkLShift, dwFlags: keyEventFKeyUp, dwExtraInfo: ownInputTag}},
	}
	n, errno := procSendInput.call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), errno
}

// sendEnterBypassingCtrl injects the send replacement for a guarded
// Ctrl+Enter: release the physically held Ctrl side(s), tap Enter, then
// press them again, all tagged and in one SendInput call, so the target
// observes a plain Enter while the physical Ctrl state is preserved. With
// neither side reported down it degrades to a bare Enter tap. Returns the
// expected and actually inserted event counts.
func sendEnterBypassingCtrl(lctrl, rctrl bool) (want, got uint32, errno syscall.Errno) {
	var inputs [6]inputStruct
	n := 0
	add := func(vk uint16, up bool) {
		flags := extendedFlagFor(vk)
		if up {
			flags |= keyEventFKeyUp
		}
		inputs[n] = inputStruct{
			inputType: inputKeyboard,
			ki:        keybdInput{wVk: vk, dwFlags: flags, dwExtraInfo: ownInputTag},
		}
		n++
	}
	if lctrl {
		add(vkLControl, true)
	}
	if rctrl {
		add(vkRControl, true)
	}
	add(vkReturn, false)
	add(vkReturn, true)
	if rctrl {
		add(vkRControl, false)
	}
	if lctrl {
		add(vkLControl, false)
	}
	r, errno := procSendInput.call(uintptr(n), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(n), uint32(r), errno
}

func setTimer(hwnd uintptr, id uintptr, ms uint32) bool {
	r, _ := procSetTimer.call(hwnd, id, uintptr(ms), 0)
	return r != 0
}

func killTimer(hwnd uintptr, id uintptr) {
	procKillTimer.call(hwnd, id)
}

// ---- icons, cursors, menus ----

func loadIcon(hInstance uintptr, id uintptr) (uintptr, syscall.Errno) {
	return procLoadIconW.call(hInstance, id)
}

func loadCursor(hInstance uintptr, id uintptr) uintptr {
	r, _ := procLoadCursorW.call(hInstance, id)
	return r
}

func createPopupMenu() uintptr {
	r, _ := procCreatePopupMenu.call()
	return r
}

func destroyMenu(menu uintptr) {
	procDestroyMenu.call(menu)
}

func appendMenu(menu uintptr, flags uint32, id uintptr, text string) bool {
	r, _ := procAppendMenuW.call(menu, uintptr(flags), id, uintptr(unsafe.Pointer(mustUTF16(text))))
	return r != 0
}

func checkMenuItem(menu uintptr, id uintptr, flags uint32) {
	procCheckMenuItem.call(menu, id, uintptr(flags))
}

func trackPopupMenuEx(menu uintptr, flags uint32, x, y int32, hwnd uintptr) uintptr {
	r, _ := procTrackPopupMenuEx.call(menu, uintptr(flags), uintptr(x), uintptr(y), hwnd, 0)
	return r
}

// ---- monitors and DPI ----

func monitorFromWindow(hwnd uintptr, flags uint32) uintptr {
	r, _ := procMonitorFromWindow.call(hwnd, uintptr(flags))
	return r
}

func getMonitorInfo(mon uintptr, mi *monitorInfo) bool {
	mi.cbSize = uint32(unsafe.Sizeof(monitorInfo{}))
	r, _ := procGetMonitorInfoW.call(mon, uintptr(unsafe.Pointer(mi)))
	return r != 0
}

func getDpiForMonitor(mon uintptr) (uint32, bool) {
	var x, y uint32
	r, _ := procGetDpiForMonitor.call(mon, mdtEffectiveDpi, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)))
	return x, r == 0 // S_OK
}

func getDpiForWindow(hwnd uintptr) uint32 {
	r, _ := procGetDpiForWindow.call(hwnd)
	return uint32(r)
}

func setPerMonitorV2() {
	r, errno := procSetProcessDpiAwarenessContext.call(dpiAwarenessContextPerMonitorAwareV2)
	if r == 0 {
		// Expected when the embedded manifest already applied PerMonitorV2
		// (ERROR_ACCESS_DENIED); anything else is worth a diagnostic too.
		debugf("SetProcessDpiAwarenessContext: errno=%d (manifest usually already applied)", errno)
	}
}

// ---- messaging with deadlines ----

func sendMessageTimeout(hwnd uintptr, msg uint32, wParam, lParam uintptr, flags, timeoutMs uint32) (uintptr, bool, syscall.Errno) {
	var result uintptr
	r, errno := procSendMessageTimeoutW.call(hwnd, uintptr(msg), wParam, lParam, uintptr(flags), uintptr(timeoutMs), uintptr(unsafe.Pointer(&result)))
	return result, r != 0, errno
}

func messageBox(hwnd uintptr, text, caption string, flags uint32) {
	procMessageBoxW.call(hwnd, uintptr(unsafe.Pointer(mustUTF16(text))), uintptr(unsafe.Pointer(mustUTF16(caption))), uintptr(flags))
}

// ---- gdi32 ----

func createOsdFont(height int32) uintptr {
	r, _ := procCreateFontW.call(
		uintptr(-height), // negative: character height
		0, 0, 0,
		fwBold,
		0, 0, 0,
		defaultCharset,
		outDefaultPrecis,
		clipDefaultPrecis,
		cleartypeQuality,
		defaultPitch,
		uintptr(unsafe.Pointer(mustUTF16(osdFontFace))),
	)
	return r
}

func createSolidBrush(color uint32) uintptr {
	r, _ := procCreateSolidBrush.call(uintptr(color))
	return r
}

func createRoundRectRgn(left, top, right, bottom, ellipseW, ellipseH int32) uintptr {
	r, _ := procCreateRoundRectRgn.call(uintptr(left), uintptr(top), uintptr(right), uintptr(bottom), uintptr(ellipseW), uintptr(ellipseH))
	return r
}

func deleteObject(obj uintptr) {
	procDeleteObject.call(obj)
}

func selectObject(hdc, obj uintptr) uintptr {
	r, _ := procSelectObject.call(hdc, obj)
	return r
}

func setTextColor(hdc uintptr, color uint32) {
	procSetTextColor.call(hdc, uintptr(color))
}

func setBkMode(hdc uintptr, mode int32) {
	procSetBkMode.call(hdc, uintptr(mode))
}

// ---- shell32 ----

func shellNotifyIcon(action uint32, nid *notifyIconDataW) bool {
	r, _ := procShellNotifyIconW.call(uintptr(action), uintptr(unsafe.Pointer(nid)))
	return r != 0
}

// ---- imm32 ----

func immGetDefaultIMEWnd(hwnd uintptr) uintptr {
	r, _ := procImmGetDefaultIMEWnd.call(hwnd)
	return r
}

// ---- wtsapi32 ----

func wtsRegisterSessionNotification(hwnd uintptr) bool {
	r, _ := procWTSRegisterSessionNotification.call(hwnd, notifyForThisSession)
	return r != 0
}

func wtsUnRegisterSessionNotification(hwnd uintptr) {
	procWTSUnRegisterSessionNotification.call(hwnd)
}

// winError wraps an API failure; errno 0 means the API reports failure only
// through its return value.
func winError(api string, errno syscall.Errno) error {
	if errno == 0 {
		return fmt.Errorf("%s failed", api)
	}
	return fmt.Errorf("%s: %w", api, errno)
}

func boolToUintptr(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}
