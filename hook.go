//go:build windows

package main

// The WH_KEYBOARD_LL hook and its dedicated OS thread. The callback performs
// only fixed-cost work: state-machine updates, the optional two-event menu
// suppressor, and a PostThreadMessage to this thread's own queue. Everything
// else — including all logging — happens in the hook thread's message loop
// after the callback has returned (NFR-1/2), and the actual IME work happens
// on the UI thread via the second stage of the dispatch.

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type hookThread struct {
	ctrl    uintptr // controller window owned by the UI thread
	machine *tapMachine
	// tid is published to the UI thread through the startup channel receive
	// (happens-before), then only read.
	tid uint32
	// enabled is owned by the hook thread: written in the message loop,
	// read in the callback, both on the same OS thread.
	enabled bool
	hhook   uintptr

	// Diagnostics counters, incremented in the callback (where logging is
	// forbidden) and drained to OutputDebugString by the message loop.
	suppressShort atomic.Uint32 // menu suppressor inserted fewer than 2 events
	postFailed    atomic.Uint32 // PostThreadMessage from the callback failed
	maxLatency    atomic.Uint64 // QPC ticks; only when measureHookLatency
}

// hook is the process-wide singleton the raw callback needs to reach.
var hook *hookThread

var hookProcCB = syscall.NewCallback(hookProc)

func newHookThread(ctrl uintptr) *hookThread {
	return &hookThread{ctrl: ctrl, machine: newTapMachine(tapMaxHoldMs), enabled: true}
}

// hookProc is the LowLevelKeyboardProc. lParam is typed unsafe.Pointer
// because for WH_KEYBOARD_LL it is always a KBDLLHOOKSTRUCT pointer supplied
// by the OS; it is only dereferenced for HC_ACTION. The return value is
// always CallNextHookEx so physical keys are never blocked.
func hookProc(nCode, wParam uintptr, lParam unsafe.Pointer) uintptr {
	h := hook
	if h != nil && int32(uint32(nCode)) == hcAction && h.enabled {
		if measureHookLatency {
			start := queryPerformanceCounter()
			h.handleKey(wParam, (*kbdllHookStruct)(lParam))
			if d := queryPerformanceCounter() - start; d > h.maxLatency.Load() {
				h.maxLatency.Store(d)
			}
		} else {
			h.handleKey(wParam, (*kbdllHookStruct)(lParam))
		}
	}
	return callNextHookEx(nCode, wParam, uintptr(lParam))
}

func (h *hookThread) handleKey(wParam uintptr, k *kbdllHookStruct) {
	var down bool
	switch wParam {
	case wmKeyDown, wmSysKeyDown:
		down = true
	case wmKeyUp, wmSysKeyUp:
		down = false
	default:
		return
	}
	if down == (k.flags&llkhfUp != 0) {
		// Message kind contradicts LLKHF_UP: cancel any tap in progress.
		h.machine.invalidate()
		return
	}
	injected := k.flags&llkhfInjected != 0
	if injected && k.dwExtraInfo == ownInputTag {
		return // self-injected (IME keys, menu suppressor): invisible to the machine
	}
	vk := normalizeAltVK(k.vkCode, k.flags&llkhfExtended != 0)
	act := h.machine.feed(keyEvent{vk: vk, down: down, injected: injected, time: k.time})
	if act.beginTap && suppressAltMenuFocus {
		// Original alt-ime-ahk compatibility: inject the tagged unassigned
		// VK 0x07 so a lone Alt press does not focus the menu bar. Failure
		// only loses the suppression; the physical Alt still passes through.
		if n, _ := sendKeyPair(vkMenuSuppress); n != 2 {
			h.suppressShort.Add(1)
		}
	}
	if act.dispatch {
		// Stage one of the two-stage dispatch: capture the tap-time
		// foreground window and post to this thread's own queue, so the
		// switch request is forwarded to the UI only after this callback
		// has returned.
		if target := getForegroundWindow(); target != 0 {
			if !postThreadMessage(h.tid, msgHookDispatchSwitch, packSwitchWParam(act.imeOpen, act.triggerVK), target) {
				h.postFailed.Add(1)
			}
		}
	}
}

// run owns the hook for its whole life on one locked OS thread. The first
// (and only) value sent on ready reports whether SetWindowsHookExW
// succeeded.
func (h *hookThread) run(ready chan<- error) {
	runtime.LockOSThread()
	var msg msgStruct
	// Force the thread message queue into existence before publishing tid.
	peekMessage(&msg, pmNoRemove)
	h.tid = getCurrentThreadId()
	h.machine.resync(scanHeldKeys())
	hhook, err := setWindowsHookEx(whKeyboardLL, hookProcCB, getModuleHandle(), 0)
	if err != nil {
		ready <- err
		return
	}
	h.hhook = hhook
	ready <- nil

	for {
		switch r := getMessage(&msg); r {
		case 0, -1:
			// Nothing posts WM_QUIT here and -1 is not expected for a pure
			// thread-message loop: treat the loop as broken, remove the hook
			// and tell the UI it died.
			h.unhook()
			debugf("hook thread: GetMessage returned %d; stopping", r)
			postMessage(h.ctrl, msgHookStopped, 1, 0)
			return
		}
		switch msg.message {
		case msgHookDispatchSwitch:
			// Stage two: the callback has long returned; hand the request to
			// the UI thread. The UI re-validates the target window and the
			// Alt release before touching the IME.
			if h.enabled {
				if !postMessage(h.ctrl, msgSwitch, msg.wParam, msg.lParam) {
					debugf("hook: PostMessage(msgSwitch) failed")
				}
			}
		case msgHookSetEnabled:
			h.enabled = msg.wParam != 0
			h.machine.resync(scanHeldKeys())
		case msgHookReset:
			// Session unlock / power resume: the OS may have swallowed
			// arbitrary key transitions while we were not watching.
			h.machine.resync(scanHeldKeys())
		case msgHookStop:
			h.unhook()
			h.drainDiagnostics()
			h.reportLatency()
			if !postMessage(h.ctrl, msgHookStopped, 0, 0) {
				debugf("hook: PostMessage(msgHookStopped) failed")
			}
			return
		}
		h.drainDiagnostics()
	}
}

func (h *hookThread) unhook() {
	if h.hhook != 0 {
		if !unhookWindowsHookEx(h.hhook) {
			debugf("UnhookWindowsHookEx failed")
		}
		h.hhook = 0
	}
}

func (h *hookThread) drainDiagnostics() {
	if n := h.suppressShort.Swap(0); n != 0 {
		debugf("menu suppressor SendInput inserted <2 events (%d times)", n)
	}
	if n := h.postFailed.Swap(0); n != 0 {
		debugf("PostThreadMessage(dispatch) failed inside callback (%d times)", n)
	}
}

func (h *hookThread) reportLatency() {
	if !measureHookLatency {
		return
	}
	if freq := queryPerformanceFrequency(); freq != 0 {
		debugf("hook callback max latency: %dus", h.maxLatency.Load()*1_000_000/freq)
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
		case vkShift, vkControl, vkMenu:
			continue
		}
		if getAsyncKeyStateDown(vk) {
			down = append(down, vk)
		}
	}
	return down
}
