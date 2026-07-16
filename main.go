//go:build windows

package main

// Process lifecycle and the UI thread: controller window, switch-request
// validation, tray interaction, session/power notifications, and ordered
// shutdown. The UI thread is locked to its OS thread for its whole life;
// the hook lives on its own locked thread (architecture §3).

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

const (
	appTitle      = "alt-ime"
	ctrlClassName = "altIMECtrl"
	osdClassName  = "altIMEOsd"

	// Session-local named mutex (no Global\ prefix) for single instance.
	mutexName = "alt-ime-single-instance"

	timerIDRetrySwitch      = 1
	timerIDShutdownFallback = 2
)

// pendingSwitch holds a validated-but-for-Alt-release switch request while
// the UI re-checks the trigger key for at most switchRetryDeadlineMs.
type pendingSwitch struct {
	active   bool
	open     bool
	vk       uint32
	target   uintptr
	deadline uint64 // GetTickCount64 ms
}

type app struct {
	mutex          syscall.Handle
	ctrl           uintptr
	osd            *osdWindow
	tray           *trayIcon
	taskbarCreated uint32
	sessionNotify  bool

	// enabled / guardEnabled mirror the hook thread's state for menu display
	// and request gating; the hook side is updated via msgHookSetEnabled and
	// msgHookSetEnterGuard.
	enabled      bool
	guardEnabled bool
	hookRunning  bool
	shuttingDown bool
	shutdownDone bool
	pending      pendingSwitch
}

// ui is the UI-thread singleton reached from the raw window procedures.
var ui *app

var ctrlWndProcCB = syscall.NewCallback(ctrlWndProc)

func main() {
	runtime.LockOSThread()
	if err := loadWin32(); err != nil {
		// Even MessageBoxW may be unavailable here; there is no better
		// channel in a GUI-subsystem process.
		if procMessageBoxW.addr != 0 {
			messageBox(0, "起動に失敗しました:\n"+err.Error(), appTitle, mbOK|mbIconError)
		}
		os.Exit(1)
	}
	if err := runApp(); err != nil {
		debugf("fatal: %v", err)
		messageBox(0, "alt-ime を継続できません:\n"+err.Error(), appTitle, mbOK|mbIconError)
		os.Exit(1)
	}
}

func runApp() error {
	// DPI awareness before the first HWND. The embedded manifest is the
	// primary mechanism; this call only matters for manifest-less builds.
	setPerMonitorV2()

	// Everything acquired so far is torn down in reverse order when a later
	// initialization step fails (architecture §3.3). Once the message loop
	// owns shutdown, the undo stack is disarmed.
	var undo []func()
	fail := func(err error) error {
		for i := len(undo) - 1; i >= 0; i-- {
			undo[i]()
		}
		return err
	}

	mutex, errno := createMutex(mutexName)
	if mutex == 0 {
		return winError("CreateMutexW", errno)
	}
	if errno == errorAlreadyExists {
		syscall.CloseHandle(mutex)
		messageBox(0, "alt-ime は既に起動しています。", appTitle, mbOK|mbIconInformation)
		return nil
	}
	undo = append(undo, func() { closeMutex(mutex) })

	a := &app{mutex: mutex, enabled: true, guardEnabled: enterGuardDefaultEnabled}
	ui = a
	undo = append(undo, func() { ui = nil })

	hinst := getModuleHandle()
	a.taskbarCreated = registerWindowMessage("TaskbarCreated")
	if a.taskbarCreated == 0 {
		debugf("RegisterWindowMessageW(TaskbarCreated) failed; tray will not survive Explorer restarts")
	}

	if err := registerClass(ctrlClassName, ctrlWndProcCB, hinst); err != nil {
		return fail(err)
	}
	ctrl, cerr := createWindow(0, ctrlClassName, appTitle, wsOverlapped, 0, 0, 0, 0, 0, hinst)
	if ctrl == 0 {
		return fail(winError("CreateWindowExW(controller)", cerr))
	}
	a.ctrl = ctrl
	undo = append(undo, func() { destroyWindow(ctrl) })

	osd, err := newOsdWindow(hinst)
	if err != nil {
		return fail(err)
	}
	a.osd = osd
	undo = append(undo, osd.destroy)

	tray, err := newTrayIcon(ctrl, hinst)
	if err != nil {
		return fail(err) // fatal per design §7: MessageBox happens in main
	}
	a.tray = tray
	undo = append(undo, tray.destroy)

	if a.sessionNotify = wtsRegisterSessionNotification(ctrl); !a.sessionNotify {
		debugf("WTSRegisterSessionNotification failed; no session-resume resync")
	} else {
		undo = append(undo, func() { wtsUnRegisterSessionNotification(ctrl) })
	}

	hook = newHookThread(ctrl)
	ready := make(chan error, 1)
	go hook.run(ready)
	if err := <-ready; err != nil {
		return fail(err)
	}
	a.hookRunning = true

	// From here on, shutdown is owned by beginShutdown/finishShutdown.
	undo = nil

	var msg msgStruct
	for {
		switch r := getMessage(&msg); r {
		case 0:
			return nil
		case -1:
			// Broken UI loop (NFR-10). The process exit removes the hook;
			// resources cannot be freed reliably at this point.
			return errors.New("GetMessage が -1 を返しました (UI スレッド異常)")
		default:
			translateMessage(&msg)
			dispatchMessage(&msg)
		}
	}
}

func ctrlWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := ui
	if a == nil || a.ctrl != hwnd {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
	switch uint32(msg) {
	case msgSwitch:
		a.onSwitchRequest(wParam, lParam)
		return 0
	case msgGuardEnter:
		a.onGuardEnter(wParam, lParam)
		return 0
	case msgTray:
		a.onTrayEvent(wParam, lParam)
		return 0
	case msgHookStopped:
		a.onHookStopped()
		return 0
	case wmTimer:
		a.onTimer(wParam)
		return 0
	case wmWtsSessionChange:
		if wParam == wtsSessionLock || wParam == wtsSessionUnlock {
			a.postHookReset()
		}
		return 0
	case wmPowerBroadcast:
		if wParam == pbtApmResumeAutomatic || wParam == pbtApmResumeSuspend {
			a.postHookReset()
		}
		return 1 // TRUE
	case wmClose:
		a.beginShutdown()
		return 0
	default:
		if a.taskbarCreated != 0 && uint32(msg) == a.taskbarCreated {
			if !a.shuttingDown {
				a.tray.reRegister()
			}
			return 0
		}
	}
	return defWindowProc(hwnd, msg, wParam, lParam)
}

// onSwitchRequest is the UI half of the two-stage dispatch. Every condition
// from architecture §4.4 is re-validated here because the request may have
// sat in two queues: enabled, live target, target still foreground, and the
// trigger Alt actually released.
func (a *app) onSwitchRequest(wParam, lParam uintptr) {
	if a.shuttingDown || !a.enabled {
		return
	}
	open, vk := unpackSwitchWParam(wParam)
	target := lParam
	if target == 0 || !isWindow(target) || getForegroundWindow() != target {
		return
	}
	if getAsyncKeyStateDown(vk) {
		// The hook saw the Alt-up before the async state updated. Re-check
		// briefly on a timer instead of sending Alt+IME-key by accident.
		a.pending = pendingSwitch{
			active:   true,
			open:     open,
			vk:       vk,
			target:   target,
			deadline: getTickCount64() + switchRetryDeadlineMs,
		}
		if !setTimer(a.ctrl, timerIDRetrySwitch, switchRetryIntervalMs) {
			debugf("SetTimer(retry) failed; discarding switch request")
			a.pending = pendingSwitch{}
		}
		return
	}
	a.doSwitch(open, target)
}

func (a *app) onRetryTimer() {
	p := a.pending
	if !p.active || a.shuttingDown || !a.enabled {
		a.dropPending()
		return
	}
	if !isWindow(p.target) || getForegroundWindow() != p.target {
		a.dropPending() // target changed while waiting: discard, no OSD
		return
	}
	if !getAsyncKeyStateDown(p.vk) {
		a.dropPending()
		a.doSwitch(p.open, p.target)
		return
	}
	if getTickCount64() >= p.deadline {
		a.dropPending() // Alt still held past the deadline: not a tap we honor
	}
}

func (a *app) dropPending() {
	if a.pending.active {
		a.pending = pendingSwitch{}
	}
	killTimer(a.ctrl, timerIDRetrySwitch)
}

func (a *app) doSwitch(open bool, target uintptr) {
	if setIME(open, target) {
		kind := osdOff
		if open {
			kind = osdOn
		}
		a.osd.show(kind, target)
	} else {
		a.osd.show(osdFail, target)
	}
}

// onGuardEnter is the UI half of the Enter-guard dispatch. The physical
// Enter was already consumed by the hook; this decides what to deliver
// instead. A plain Enter goes back when Ctrl was held (send intent) or when
// a composition is believed open and the target's IME reports open — the
// CON-9 mitigation that lets Enter keep committing Japanese input.
// Shift+Enter (newline) is delivered otherwise. If the foreground changed
// while the request sat in two queues, it is dropped: injecting into a
// different application would be worse than losing one keystroke.
func (a *app) onGuardEnter(wParam, lParam uintptr) {
	if a.shuttingDown || !a.enabled || !a.guardEnabled {
		return
	}
	send, composing := unpackGuardWParam(wParam)
	target := lParam
	if target == 0 || !isWindow(target) || getForegroundWindow() != target {
		debugf("guard: replacement dropped (foreground changed)")
		return
	}
	plain := send
	imeOpen, imeOK := false, false
	if !plain && composing {
		imeOpen, imeOK = queryImeOpen(target)
		// A definitive "closed" means direct (non-IME) input and the
		// newline replacement is safe. Both "open" and "no answer" deliver
		// the plain Enter: breaking a possible composition commit is worse
		// than falling back to the app's default Enter behavior (CON-9
		// fails open).
		plain = imeOpen || !imeOK
	}
	if guardTrace {
		debugf("guard: enter send=%t composing=%t imeOpen=%t imeOK=%t -> plain=%t", send, composing, imeOpen, imeOK, plain)
	}
	if plain {
		a.deliverPlainEnter()
		return
	}
	if n, errno := sendShiftEnter(); n != 4 {
		debugf("guard: Shift+Enter SendInput inserted %d/4, errno=%d", n, errno)
		// A partial insertion can leave the injected Shift logically down;
		// an unmatched key-up is harmless.
		sendKeyUp(vkLShift)
	}
}

// deliverPlainEnter injects a tagged plain Enter, temporarily releasing
// whichever physical Ctrl side is still down so the target does not observe
// Ctrl+Enter. When the user already released Ctrl by the time this runs,
// sendEnterBypassingCtrl degrades to a bare Enter tap.
func (a *app) deliverPlainEnter() {
	lctrl := getAsyncKeyStateDown(vkLControl)
	rctrl := getAsyncKeyStateDown(vkRControl)
	if want, got, errno := sendEnterBypassingCtrl(lctrl, rctrl); got != want {
		debugf("guard: Ctrl-bypass Enter SendInput inserted %d/%d, errno=%d", got, want, errno)
		// The physically held side(s) must end logically down again; a
		// duplicate key-down over an already-down key is harmless.
		if lctrl {
			sendKeyDown(vkLControl)
		}
		if rctrl {
			sendKeyDown(vkRControl)
		}
	}
}

func (a *app) onTrayEvent(wParam, lParam uintptr) {
	if a.shuttingDown {
		return
	}
	switch uint32(lParam & 0xFFFF) { // NOTIFYICON_VERSION_4: LOWORD(lParam)
	case wmContextMenu, ninSelect, ninKeySelect:
		x := int32(int16(wParam & 0xFFFF))
		y := int32(int16((wParam >> 16) & 0xFFFF))
		switch a.tray.showMenu(x, y, a.enabled, a.guardEnabled) {
		case cmdToggleEnabled:
			a.toggleEnabled()
		case cmdToggleEnterGuard:
			a.toggleEnterGuard()
		case cmdExit:
			a.beginShutdown()
		}
	}
}

func (a *app) toggleEnabled() {
	a.enabled = !a.enabled
	a.dropPending() // queued/pending requests are re-gated by a.enabled
	if a.hookRunning && !postThreadMessage(hook.tid, msgHookSetEnabled, boolToUintptr(a.enabled), 0) {
		debugf("PostThreadMessage(msgHookSetEnabled) failed")
	}
}

func (a *app) toggleEnterGuard() {
	a.guardEnabled = !a.guardEnabled
	if a.hookRunning && !postThreadMessage(hook.tid, msgHookSetEnterGuard, boolToUintptr(a.guardEnabled), 0) {
		debugf("PostThreadMessage(msgHookSetEnterGuard) failed")
	}
}

func (a *app) postHookReset() {
	if a.hookRunning && !postThreadMessage(hook.tid, msgHookReset, 0, 0) {
		debugf("PostThreadMessage(msgHookReset) failed")
	}
}

// beginShutdown starts the ordered teardown: stop the hook first, release
// UI resources only after the hook confirms (architecture §3.3). A fallback
// timer prevents waiting forever on a wedged hook thread.
func (a *app) beginShutdown() {
	if a.shuttingDown {
		return
	}
	a.shuttingDown = true
	a.dropPending()
	if a.hookRunning && postThreadMessage(hook.tid, msgHookStop, 0, 0) {
		if setTimer(a.ctrl, timerIDShutdownFallback, shutdownFallbackMs) {
			return
		}
		debugf("SetTimer(shutdown fallback) failed; finishing immediately")
	}
	a.finishShutdown()
}

func (a *app) onHookStopped() {
	a.hookRunning = false
	if !a.shuttingDown {
		// The hook loop died without being asked (broken GetMessage): the
		// core feature is gone, so surface it and exit cleanly.
		a.shuttingDown = true
		debugf("hook thread stopped unexpectedly; exiting")
		messageBox(a.ctrl, "キーボードフックが停止したため終了します。", appTitle, mbOK|mbIconError)
	}
	a.finishShutdown()
}

func (a *app) onTimer(id uintptr) {
	switch id {
	case timerIDRetrySwitch:
		a.onRetryTimer()
	case timerIDShutdownFallback:
		killTimer(a.ctrl, timerIDShutdownFallback)
		if !a.shutdownDone {
			debugf("hook thread did not confirm stop within %dms", shutdownFallbackMs)
			a.finishShutdown()
		}
	}
}

// finishShutdown releases resources in reverse acquisition order: tray,
// timers, OSD (window + GDI), session notification, controller window,
// mutex, and finally the quit message (architecture §3.3).
func (a *app) finishShutdown() {
	if a.shutdownDone {
		return
	}
	a.shutdownDone = true
	a.shuttingDown = true
	if a.tray != nil {
		a.tray.destroy()
	}
	a.dropPending()
	killTimer(a.ctrl, timerIDShutdownFallback)
	if a.osd != nil {
		a.osd.destroy()
	}
	if a.sessionNotify {
		wtsUnRegisterSessionNotification(a.ctrl)
		a.sessionNotify = false
	}
	if a.ctrl != 0 {
		destroyWindow(a.ctrl)
		a.ctrl = 0
	}
	closeMutex(a.mutex)
	a.mutex = 0
	postQuitMessage(0)
}
