//go:build windows

package app

// Process lifecycle and the UI thread: controller window, switch-request
// validation, tray interaction, session/power notifications, and ordered
// shutdown. The UI thread is locked to its OS thread for its whole life;
// the hook lives on its own locked thread (architecture §3).

import (
	"errors"
	"os"
	"runtime"
	"syscall"

	"github.com/ishibashi-futos/alt-ime-go/internal/config"
	"github.com/ishibashi-futos/alt-ime-go/internal/hookstate"
	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
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
// the UI re-checks the trigger key for at most config.SwitchRetryDeadlineMs.
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

// Main owns the UI thread for the whole process life. It must run on the
// main goroutine so LockOSThread pins it to the initial OS thread.
func Main() {
	runtime.LockOSThread()
	if err := win32.Load(); err != nil {
		// MessageBox degrades to a no-op when user32 could not be resolved;
		// there is no better channel in a GUI-subsystem process.
		win32.MessageBox(0, "起動に失敗しました:\n"+err.Error(), appTitle, win32.MbOK|win32.MbIconError)
		os.Exit(1)
	}
	if err := runApp(); err != nil {
		win32.Debugf("fatal: %v", err)
		win32.MessageBox(0, "alt-ime を継続できません:\n"+err.Error(), appTitle, win32.MbOK|win32.MbIconError)
		os.Exit(1)
	}
}

func runApp() error {
	// DPI awareness before the first HWND. The embedded manifest is the
	// primary mechanism; this call only matters for manifest-less builds.
	win32.SetPerMonitorV2()

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

	mutex, errno := win32.CreateMutex(mutexName)
	if mutex == 0 {
		return win32.WinError("CreateMutexW", errno)
	}
	if errno == win32.ErrorAlreadyExists {
		syscall.CloseHandle(mutex)
		win32.MessageBox(0, "alt-ime は既に起動しています。", appTitle, win32.MbOK|win32.MbIconInformation)
		return nil
	}
	undo = append(undo, func() { win32.CloseMutex(mutex) })

	a := &app{mutex: mutex, enabled: true, guardEnabled: config.EnterGuardDefaultEnabled}
	ui = a
	undo = append(undo, func() { ui = nil })

	hinst := win32.GetModuleHandle()
	a.taskbarCreated = win32.RegisterWindowMessage("TaskbarCreated")
	if a.taskbarCreated == 0 {
		win32.Debugf("RegisterWindowMessageW(TaskbarCreated) failed; tray will not survive Explorer restarts")
	}

	if err := win32.RegisterClass(ctrlClassName, ctrlWndProcCB, hinst); err != nil {
		return fail(err)
	}
	ctrl, cerr := win32.CreateWindow(0, ctrlClassName, appTitle, win32.WsOverlapped, 0, 0, 0, 0, 0, hinst)
	if ctrl == 0 {
		return fail(win32.WinError("CreateWindowExW(controller)", cerr))
	}
	a.ctrl = ctrl
	undo = append(undo, func() { win32.DestroyWindow(ctrl) })

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

	if a.sessionNotify = win32.WtsRegisterSessionNotification(ctrl); !a.sessionNotify {
		win32.Debugf("WTSRegisterSessionNotification failed; no session-resume resync")
	} else {
		undo = append(undo, func() { win32.WtsUnRegisterSessionNotification(ctrl) })
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

	var msg win32.MsgStruct
	for {
		switch r := win32.GetMessage(&msg); r {
		case 0:
			return nil
		case -1:
			// Broken UI loop (NFR-10). The process exit removes the hook;
			// resources cannot be freed reliably at this point.
			return errors.New("GetMessage が -1 を返しました (UI スレッド異常)")
		default:
			win32.TranslateMessage(&msg)
			win32.DispatchMessage(&msg)
		}
	}
}

func ctrlWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := ui
	if a == nil || a.ctrl != hwnd {
		return win32.DefWindowProc(hwnd, msg, wParam, lParam)
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
	case win32.WmTimer:
		a.onTimer(wParam)
		return 0
	case win32.WmWtsSessionChange:
		if wParam == win32.WtsSessionLock || wParam == win32.WtsSessionUnlock {
			a.postHookReset()
		}
		return 0
	case win32.WmPowerBroadcast:
		if wParam == win32.PbtApmResumeAutomatic || wParam == win32.PbtApmResumeSuspend {
			a.postHookReset()
		}
		return 1 // TRUE
	case win32.WmClose:
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
	return win32.DefWindowProc(hwnd, msg, wParam, lParam)
}

// onSwitchRequest is the UI half of the two-stage dispatch. Every condition
// from architecture §4.4 is re-validated here because the request may have
// sat in two queues: enabled, live target, target still foreground, and the
// trigger Alt actually released.
func (a *app) onSwitchRequest(wParam, lParam uintptr) {
	if a.shuttingDown || !a.enabled {
		return
	}
	open, vk := hookstate.UnpackSwitchWParam(wParam)
	target := lParam
	if target == 0 || !win32.IsWindow(target) || win32.GetForegroundWindow() != target {
		return
	}
	if win32.GetAsyncKeyStateDown(vk) {
		// The hook saw the Alt-up before the async state updated. Re-check
		// briefly on a timer instead of sending Alt+IME-key by accident.
		a.pending = pendingSwitch{
			active:   true,
			open:     open,
			vk:       vk,
			target:   target,
			deadline: win32.GetTickCount64() + config.SwitchRetryDeadlineMs,
		}
		if !win32.SetTimer(a.ctrl, timerIDRetrySwitch, config.SwitchRetryIntervalMs) {
			win32.Debugf("SetTimer(retry) failed; discarding switch request")
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
	if !win32.IsWindow(p.target) || win32.GetForegroundWindow() != p.target {
		a.dropPending() // target changed while waiting: discard, no OSD
		return
	}
	if !win32.GetAsyncKeyStateDown(p.vk) {
		a.dropPending()
		a.doSwitch(p.open, p.target)
		return
	}
	if win32.GetTickCount64() >= p.deadline {
		a.dropPending() // Alt still held past the deadline: not a tap we honor
	}
}

func (a *app) dropPending() {
	if a.pending.active {
		a.pending = pendingSwitch{}
	}
	win32.KillTimer(a.ctrl, timerIDRetrySwitch)
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
	send, composing := hookstate.UnpackGuardWParam(wParam)
	target := lParam
	if target == 0 || !win32.IsWindow(target) || win32.GetForegroundWindow() != target {
		win32.Debugf("guard: replacement dropped (foreground changed)")
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
	if config.GuardTrace {
		win32.Debugf("guard: enter send=%t composing=%t imeOpen=%t imeOK=%t -> plain=%t", send, composing, imeOpen, imeOK, plain)
	}
	if plain {
		a.deliverPlainEnter()
		return
	}
	if n, errno := win32.SendShiftEnter(); n != 4 {
		win32.Debugf("guard: Shift+Enter SendInput inserted %d/4, errno=%d", n, errno)
		// A partial insertion can leave the injected Shift logically down;
		// an unmatched key-up is harmless.
		win32.SendKeyUp(win32.VkLShift)
	}
}

// deliverPlainEnter injects a tagged plain Enter, temporarily releasing
// whichever physical Ctrl side is still down so the target does not observe
// Ctrl+Enter. When the user already released Ctrl by the time this runs,
// win32.SendEnterBypassingCtrl degrades to a bare Enter tap.
func (a *app) deliverPlainEnter() {
	lctrl := win32.GetAsyncKeyStateDown(win32.VkLControl)
	rctrl := win32.GetAsyncKeyStateDown(win32.VkRControl)
	if want, got, errno := win32.SendEnterBypassingCtrl(lctrl, rctrl); got != want {
		win32.Debugf("guard: Ctrl-bypass Enter SendInput inserted %d/%d, errno=%d", got, want, errno)
		// The physically held side(s) must end logically down again; a
		// duplicate key-down over an already-down key is harmless.
		if lctrl {
			win32.SendKeyDown(win32.VkLControl)
		}
		if rctrl {
			win32.SendKeyDown(win32.VkRControl)
		}
	}
}

func (a *app) onTrayEvent(wParam, lParam uintptr) {
	if a.shuttingDown {
		return
	}
	switch uint32(lParam & 0xFFFF) { // NOTIFYICON_VERSION_4: LOWORD(lParam)
	case win32.WmContextMenu, win32.NinSelect, win32.NinKeySelect:
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
	if a.hookRunning && !win32.PostThreadMessage(hook.tid, msgHookSetEnabled, win32.BoolToUintptr(a.enabled), 0) {
		win32.Debugf("PostThreadMessage(msgHookSetEnabled) failed")
	}
}

func (a *app) toggleEnterGuard() {
	a.guardEnabled = !a.guardEnabled
	if a.hookRunning && !win32.PostThreadMessage(hook.tid, msgHookSetEnterGuard, win32.BoolToUintptr(a.guardEnabled), 0) {
		win32.Debugf("PostThreadMessage(msgHookSetEnterGuard) failed")
	}
}

func (a *app) postHookReset() {
	if a.hookRunning && !win32.PostThreadMessage(hook.tid, msgHookReset, 0, 0) {
		win32.Debugf("PostThreadMessage(msgHookReset) failed")
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
	if a.hookRunning && win32.PostThreadMessage(hook.tid, msgHookStop, 0, 0) {
		if win32.SetTimer(a.ctrl, timerIDShutdownFallback, config.ShutdownFallbackMs) {
			return
		}
		win32.Debugf("SetTimer(shutdown fallback) failed; finishing immediately")
	}
	a.finishShutdown()
}

func (a *app) onHookStopped() {
	a.hookRunning = false
	if !a.shuttingDown {
		// The hook loop died without being asked (broken GetMessage): the
		// core feature is gone, so surface it and exit cleanly.
		a.shuttingDown = true
		win32.Debugf("hook thread stopped unexpectedly; exiting")
		win32.MessageBox(a.ctrl, "キーボードフックが停止したため終了します。", appTitle, win32.MbOK|win32.MbIconError)
	}
	a.finishShutdown()
}

func (a *app) onTimer(id uintptr) {
	switch id {
	case timerIDRetrySwitch:
		a.onRetryTimer()
	case timerIDShutdownFallback:
		win32.KillTimer(a.ctrl, timerIDShutdownFallback)
		if !a.shutdownDone {
			win32.Debugf("hook thread did not confirm stop within %dms", config.ShutdownFallbackMs)
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
	win32.KillTimer(a.ctrl, timerIDShutdownFallback)
	if a.osd != nil {
		a.osd.destroy()
	}
	if a.sessionNotify {
		win32.WtsUnRegisterSessionNotification(a.ctrl)
		a.sessionNotify = false
	}
	if a.ctrl != 0 {
		win32.DestroyWindow(a.ctrl)
		a.ctrl = 0
	}
	win32.CloseMutex(a.mutex)
	a.mutex = 0
	win32.PostQuitMessage(0)
}
