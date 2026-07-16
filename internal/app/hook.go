//go:build windows

package app

// The WH_KEYBOARD_LL hook and its dedicated OS thread. The callback performs
// only fixed-cost work: state-machine updates, the optional two-stage menu
// suppressor, and a PostThreadMessage to this thread's own queue. Everything
// else — including all logging — happens in the hook thread's message loop
// after the callback has returned (NFR-1/2). The actual IME work and the
// Enter-guard replacement injections happen on the UI thread via the second
// stage of their respective dispatches. The guard's foreground-exe cache is
// maintained by an EVENT_SYSTEM_FOREGROUND WinEvent hook on this thread's
// pump; the rare synchronous fallback inside the callback is bounded and
// counted (guardSyncResolve).

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/ishibashi-futos/alt-ime-go/internal/config"
	"github.com/ishibashi-futos/alt-ime-go/internal/hookstate"
	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

type hookThread struct {
	ctrl    uintptr // controller window owned by the UI thread
	machine *hookstate.TapMachine
	guard   *hookstate.GuardMachine
	// tid is published to the UI thread through the startup channel receive
	// (happens-before), then only read.
	tid uint32
	// enabled and guardEnabled are owned by the hook thread: written in the
	// message loop, read in the callback, both on the same OS thread.
	enabled      bool
	guardEnabled bool
	hhook        uintptr
	winEvent     uintptr // EVENT_SYSTEM_FOREGROUND hook feeding fg below

	// fg caches whether the foreground window is an Enter-guard target. It
	// is refreshed by the WinEvent callback (delivered through this thread's
	// message pump, never inside the keyboard callback) and, as a fallback
	// for the delivery-lag window, synchronously on a guarded Enter down.
	fg struct {
		hwnd     uintptr
		isTarget bool
	}

	// Diagnostics counters, incremented in the callback (where logging is
	// forbidden) and drained to OutputDebugString by the message loop.
	suppressDownShort atomic.Uint32 // Alt-down suppressor inserted fewer than 2 events
	suppressUpShort   atomic.Uint32 // Alt-up suppressor inserted fewer than 2 events
	suppressCleanup   atomic.Uint32 // best-effort key-up cleanup was not inserted
	postFailed        atomic.Uint32 // PostThreadMessage from the callback failed
	guardSyncResolve  atomic.Uint32 // guard resolved the foreground exe inside the callback
	maxLatency        atomic.Uint64 // QPC ticks; only when config.MeasureHookLatency
}

// hook is the process-wide singleton the raw callback needs to reach.
var hook *hookThread

var hookProcCB = syscall.NewCallback(hookProc)

var winEventProcCB = syscall.NewCallback(winEventProc)

func newHookThread(ctrl uintptr) *hookThread {
	return &hookThread{
		ctrl:         ctrl,
		machine:      hookstate.NewTapMachine(config.TapMaxHoldMs),
		guard:        hookstate.NewGuardMachine(),
		enabled:      true,
		guardEnabled: config.EnterGuardDefaultEnabled,
	}
}

// hookProc is the LowLevelKeyboardProc. lParam is typed unsafe.Pointer
// because for WH_KEYBOARD_LL it is always a KBDLLHOOKSTRUCT pointer supplied
// by the OS; it is only dereferenced for HC_ACTION. Physical Alt is never
// blocked; the only events consumed (non-zero return without CallNextHookEx)
// are the Enter presses the guard machine replaces.
func hookProc(nCode, wParam uintptr, lParam unsafe.Pointer) uintptr {
	h := hook
	if h != nil && int32(uint32(nCode)) == win32.HcAction && h.enabled {
		var block bool
		if config.MeasureHookLatency {
			start := win32.QueryPerformanceCounter()
			block = h.handleKey(wParam, (*win32.KbdllHookStruct)(lParam))
			if d := win32.QueryPerformanceCounter() - start; d > h.maxLatency.Load() {
				h.maxLatency.Store(d)
			}
		} else {
			block = h.handleKey(wParam, (*win32.KbdllHookStruct)(lParam))
		}
		if block {
			return 1
		}
	}
	return win32.CallNextHookEx(nCode, wParam, uintptr(lParam))
}

func (h *hookThread) handleKey(wParam uintptr, k *win32.KbdllHookStruct) (block bool) {
	var down bool
	switch wParam {
	case win32.WmKeyDown, win32.WmSysKeyDown:
		down = true
	case win32.WmKeyUp, win32.WmSysKeyUp:
		down = false
	default:
		return false
	}
	if down == (k.Flags&win32.LlkhfUp != 0) {
		// Message kind contradicts LLKHF_UP: cancel any tap in progress.
		h.machine.Invalidate()
		return false
	}
	injected := k.Flags&win32.LlkhfInjected != 0
	if injected && k.DwExtraInfo == win32.OwnInputTag {
		return false // self-injected (IME keys, suppressors, guard): invisible to the machines
	}
	extended := k.Flags&win32.LlkhfExtended != 0
	vk := hookstate.NormalizeAltVK(k.VkCode, extended)
	act := h.machine.Feed(hookstate.KeyEvent{VK: vk, Down: down, Injected: injected, Time: k.Time})
	if act.BeginTap && config.SuppressAltMenuFocus {
		// Preserve the original alt-ime-ahk mask on Alt-down for Win32-style
		// menus. It is deliberately paired with the assigned-key suppressor
		// below because unassigned VK 0x07 may not reach modern app layers.
		h.sendSuppressor(win32.VkMenuSuppressLegacy, &h.suppressDownShort)
	}
	if act.EndTap && config.SuppressAltMenuFocus {
		// The callback runs before the physical Alt-up is posted. Insert an
		// assigned F24 pair now so Electron/Chromium and DOM keyboard handlers
		// observe a chord rather than a lone Alt release. Canceled chords have
		// their real second key and never take this path.
		h.sendSuppressor(win32.VkMenuSuppressDOM, &h.suppressUpShort)
	}
	if act.Dispatch {
		// Stage one of the two-stage dispatch: capture the tap-time
		// foreground window and post to this thread's own queue, so the
		// switch request is forwarded to the UI only after this callback
		// has returned.
		if target := win32.GetForegroundWindow(); target != 0 {
			if !win32.PostThreadMessage(h.tid, msgHookDispatchSwitch, hookstate.PackSwitchWParam(act.ImeOpen, act.TriggerVK), target) {
				h.postFailed.Add(1)
			}
		}
	}
	return h.feedGuard(hookstate.KeyEvent{VK: hookstate.NormalizeModVK(vk, extended), Down: down, Injected: injected, Time: k.Time})
}

// feedGuard runs the Enter-guard machine over the same event stream as the
// tap machine. A guarded Enter is only consumed here; choosing and
// injecting the replacement happens on the UI thread (stage two), which can
// combine the machine's composition belief with the target's actual IME
// open status — a bounded external call this callback must never make.
// Only a physical Enter down needs the foreground evaluation; every other
// event just updates the guard's modifier and composition tracking.
func (h *hookThread) feedGuard(ev hookstate.KeyEvent) bool {
	active := false
	if ev.VK == win32.VkReturn && ev.Down && !ev.Injected && h.guardEnabled {
		active = h.guardForeground()
	}
	act := h.guard.Feed(ev, active)
	if act.Dispatch {
		// Stage one, mirroring the IME-switch dispatch: post to this
		// thread's own queue so the request reaches the UI only after this
		// callback has returned. h.fg.hwnd is the foreground window the
		// active decision was just made against.
		if target := h.fg.hwnd; target != 0 {
			if !win32.PostThreadMessage(h.tid, msgHookDispatchGuard, hookstate.PackGuardWParam(act.Send, act.Composing), target) {
				h.postFailed.Add(1)
			}
		}
	}
	return act.Block
}

// guardForeground reports whether the foreground window is a guard target.
// The cache is normally maintained by the WinEvent hook; when the keyboard
// callback outruns that delivery (Alt+Tab then an immediate Enter) it falls
// back to resolving the exe synchronously — three bounded syscalls, counted
// so the frequency of this deviation from the fixed-cost rule is observable.
func (h *hookThread) guardForeground() bool {
	fg := win32.GetForegroundWindow()
	if fg != h.fg.hwnd {
		h.guardSyncResolve.Add(1)
		h.refreshForeground(fg)
	}
	return h.fg.isTarget
}

func (h *hookThread) refreshForeground(hwnd uintptr) {
	isTarget := false
	if hwnd != 0 {
		if path, ok := win32.ProcessImagePath(win32.WindowProcessId(hwnd)); ok {
			isTarget = config.MatchGuardTarget(path)
		}
	}
	h.fg.hwnd = hwnd
	h.fg.isTarget = isTarget
	// Losing or changing focus commits or cancels any open composition.
	h.guard.ClearComposing()
}

// winEventProc receives EVENT_SYSTEM_FOREGROUND. WINEVENT_OUTOFCONTEXT
// delivers it through the hook thread's message pump — never inside the
// keyboard callback — so the process query in refreshForeground stays out
// of the fixed-cost path and the cache needs no synchronization.
func winEventProc(hWinEventHook, event, hwnd, idObject, idChild, idEventThread, dwmsEventTime uintptr) uintptr {
	if h := hook; h != nil && uint32(event) == win32.EventSystemForeground {
		h.refreshForeground(hwnd)
	}
	return 0
}

// sendSuppressor is callback-safe and fixed-cost. A short pair insertion can
// theoretically leave the down half in the input stream, so always attempt a
// standalone key-up cleanup before returning. The caller still passes the
// physical Alt event regardless of every SendInput result.
func (h *hookThread) sendSuppressor(vk uint16, short *atomic.Uint32) {
	if n, _ := win32.SendKeyPair(vk); n != 2 {
		short.Add(1)
		if cleanup, _ := win32.SendKeyUp(vk); cleanup != 1 {
			h.suppressCleanup.Add(1)
		}
	}
}

// run owns the hook for its whole life on one locked OS thread. The first
// (and only) value sent on ready reports whether SetWindowsHookExW
// succeeded.
func (h *hookThread) run(ready chan<- error) {
	runtime.LockOSThread()
	var msg win32.MsgStruct
	// Force the thread message queue into existence before publishing tid.
	win32.PeekMessage(&msg, win32.PmNoRemove)
	h.tid = win32.GetCurrentThreadId()
	h.resyncMachines()
	hhook, err := win32.SetWindowsHookEx(win32.WhKeyboardLL, hookProcCB, win32.GetModuleHandle(), 0)
	if err != nil {
		ready <- err
		return
	}
	h.hhook = hhook
	// Foreground tracking for the Enter guard. On failure the guard still
	// works through the synchronous per-Enter fallback in guardForeground.
	if h.winEvent = win32.SetWinEventHook(win32.EventSystemForeground, winEventProcCB); h.winEvent == 0 {
		win32.Debugf("SetWinEventHook(foreground) failed; Enter guard falls back to per-Enter resolution")
	}
	ready <- nil

	for {
		switch r := win32.GetMessage(&msg); r {
		case 0, -1:
			// Nothing posts WM_QUIT here and -1 is not expected for a pure
			// thread-message loop: treat the loop as broken, remove the hook
			// and tell the UI it died.
			h.unhook()
			win32.Debugf("hook thread: GetMessage returned %d; stopping", r)
			win32.PostMessage(h.ctrl, msgHookStopped, 1, 0)
			return
		}
		switch msg.Message {
		case msgHookDispatchSwitch:
			// Stage two: the callback has long returned; hand the request to
			// the UI thread. The UI re-validates the target window and the
			// Alt release before touching the IME.
			if h.enabled {
				if !win32.PostMessage(h.ctrl, msgSwitch, msg.WParam, msg.LParam) {
					win32.Debugf("hook: PostMessage(msgSwitch) failed")
				}
			}
		case msgHookDispatchGuard:
			// Stage two of the guard dispatch: the physical Enter is already
			// consumed; the UI re-validates the target and injects the
			// replacement.
			if h.enabled && h.guardEnabled {
				if !win32.PostMessage(h.ctrl, msgGuardEnter, msg.WParam, msg.LParam) {
					win32.Debugf("hook: PostMessage(msgGuardEnter) failed")
				}
			}
		case msgHookSetEnabled:
			h.enabled = msg.WParam != 0
			h.resyncMachines()
		case msgHookSetEnterGuard:
			h.guardEnabled = msg.WParam != 0
			h.resyncMachines()
		case msgHookReset:
			// Session unlock / power resume: the OS may have swallowed
			// arbitrary key transitions while we were not watching.
			h.resyncMachines()
		case msgHookStop:
			h.unhook()
			h.drainDiagnostics()
			h.reportLatency()
			if !win32.PostMessage(h.ctrl, msgHookStopped, 0, 0) {
				win32.Debugf("hook: PostMessage(msgHookStopped) failed")
			}
			return
		}
		h.drainDiagnostics()
	}
}

// resyncMachines rebuilds both state machines' held-key views from the OS.
// Runs only outside the callback (startup, enable/disable, guard toggle,
// session unlock, power resume) and refreshes the guard's foreground cache
// while it is at it.
func (h *hookThread) resyncMachines() {
	held := scanHeldKeys()
	h.machine.Resync(held)
	h.guard.Resync(held)
	h.refreshForeground(win32.GetForegroundWindow())
}

func (h *hookThread) unhook() {
	if h.winEvent != 0 {
		if !win32.UnhookWinEvent(h.winEvent) {
			win32.Debugf("UnhookWinEvent failed")
		}
		h.winEvent = 0
	}
	if h.hhook != 0 {
		if !win32.UnhookWindowsHookEx(h.hhook) {
			win32.Debugf("UnhookWindowsHookEx failed")
		}
		h.hhook = 0
	}
}

func (h *hookThread) drainDiagnostics() {
	if n := h.suppressDownShort.Swap(0); n != 0 {
		win32.Debugf("Alt-down menu suppressor SendInput inserted <2 events (%d times)", n)
	}
	if n := h.suppressUpShort.Swap(0); n != 0 {
		win32.Debugf("Alt-up DOM suppressor SendInput inserted <2 events (%d times)", n)
	}
	if n := h.suppressCleanup.Swap(0); n != 0 {
		win32.Debugf("menu suppressor key-up cleanup SendInput failed (%d times)", n)
	}
	if n := h.postFailed.Swap(0); n != 0 {
		win32.Debugf("PostThreadMessage(dispatch) failed inside callback (%d times)", n)
	}
	if n := h.guardSyncResolve.Swap(0); n != 0 {
		win32.Debugf("Enter guard resolved the foreground exe inside the callback (%d times)", n)
	}
}

func (h *hookThread) reportLatency() {
	if !config.MeasureHookLatency {
		return
	}
	if freq := win32.QueryPerformanceFrequency(); freq != 0 {
		win32.Debugf("hook callback max latency: %dus", h.maxLatency.Load()*1_000_000/freq)
	}
}

// scanHeldKeys snapshots the keys the OS currently reports as down. Runs
// only outside the callback. Mouse buttons (0x01..0x06) are excluded because
// the keyboard hook will never deliver their release. Generic modifier codes
// (VK_SHIFT/CONTROL/MENU) are excluded: Alt events are normalized to their
// left/right codes at the hook boundary, and the state snapshot queries those
// specific codes directly.
func scanHeldKeys() []uint32 {
	var down []uint32
	for vk := uint32(0x08); vk <= 0xFE; vk++ {
		switch vk {
		case win32.VkShift, win32.VkControl, win32.VkMenu:
			continue
		}
		if win32.GetAsyncKeyStateDown(vk) {
			down = append(down, vk)
		}
	}
	return down
}
