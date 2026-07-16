package hookstate

import "github.com/ishibashi-futos/alt-ime-go/internal/win32"

// Alt tap detection state machine. Pure logic with no Win32 dependency so
// the full transition table stays unit-testable on any host. The machine is
// owned exclusively by the hook thread; nothing here is safe for concurrent
// use.

type tapPhase int

const (
	tapIdle tapPhase = iota
	tapTracking
	tapCanceled
)

// KeyEvent is one keyboard transition observed by the low-level hook.
// Self-injected events (LLKHF_INJECTED with ownInputTag) must be filtered
// out by the caller before feeding; injected events from other processes are
// fed with injected=true because they still reach the target application and
// therefore cancel a tap in progress.
type KeyEvent struct {
	VK       uint32
	Down     bool
	Injected bool
	Time     uint32 // KBDLLHOOKSTRUCT.time; wraps at 2^32 ms
}

// TapAction is what the hook layer must do after feeding one event.
type TapAction struct {
	// BeginTap: a physical Alt press just started a clean tap candidate.
	// This is where the legacy menu-focus suppressor may be sent.
	BeginTap bool
	// EndTap: the tracked physical Alt was released without another key,
	// including after the IME-switch hold limit. This is where the
	// Electron/DOM-visible suppressor may be sent before the Alt-up passes.
	EndTap bool
	// Dispatch: a tap completed; request an IME switch.
	Dispatch  bool
	ImeOpen   bool   // Dispatch: true = IME ON (right Alt), false = OFF (left Alt)
	TriggerVK uint32 // Dispatch: the Alt that triggered, for release re-checks
}

type TapMachine struct {
	phase     tapPhase
	targetVK  uint32
	downTime  uint32
	maxHoldMs uint32
	// held tracks every non-self-injected key currently down, indexed by VK.
	// Fixed-size so feed() performs no allocation inside the hook callback.
	held      [256]bool
	heldCount int
}

func NewTapMachine(maxHoldMs uint32) *TapMachine {
	return &TapMachine{maxHoldMs: maxHoldMs}
}

// NormalizeAltVK converts a generic VK_MENU event into the left/right code
// expected by the tap machine. Some keyboard input paths preserve VK_MENU in
// the event and carry the side only in the extended-key flag: right Alt is an
// extended key, while left Alt is not. Already-specific and non-Alt VKs pass
// through unchanged.
func NormalizeAltVK(vk uint32, extended bool) uint32 {
	if vk != win32.VkMenu {
		return vk
	}
	if extended {
		return win32.VkRMenu
	}
	return win32.VkLMenu
}

// resync replaces the held-key view and returns the machine to idle. Callers
// run it outside the hook callback (startup, enable/disable, session unlock,
// power resume) with the keys the OS currently reports as down, so keys held
// from before tracking started still cancel taps.
func (m *TapMachine) Resync(down []uint32) {
	m.held = [256]bool{}
	m.heldCount = 0
	for _, vk := range down {
		if vk > 0 && vk < 256 && !m.held[vk] {
			m.held[vk] = true
			m.heldCount++
		}
	}
	m.phase = tapIdle
	m.targetVK = 0
}

// invalidate cancels a tap in progress after an event the hook could not
// interpret (contradictory down/up encoding). The held-key view is left
// untouched: a stale "held" entry only suppresses future taps, which is the
// safe direction. Never produces a switch.
func (m *TapMachine) Invalidate() {
	if m.phase == tapTracking {
		m.phase = tapCanceled
	}
}

func (m *TapMachine) otherHeld(vk uint32) bool {
	n := m.heldCount
	if m.held[vk] {
		n--
	}
	return n > 0
}

// feed consumes one keyboard transition and reports the required action.
func (m *TapMachine) Feed(ev KeyEvent) TapAction {
	var act TapAction
	if ev.VK == 0 || ev.VK >= 256 {
		// Out-of-contract VK (KBDLLHOOKSTRUCT documents 1..254): treat as an
		// inconsistent event.
		m.Invalidate()
		return act
	}
	if ev.Down {
		m.feedDown(ev, &act)
	} else {
		m.feedUp(ev, &act)
	}
	return act
}

func (m *TapMachine) feedDown(ev KeyEvent, act *TapAction) {
	isAlt := ev.VK == win32.VkLMenu || ev.VK == win32.VkRMenu
	otherHeld := m.otherHeld(ev.VK)
	wasDown := m.held[ev.VK]
	if !wasDown {
		m.held[ev.VK] = true
		m.heldCount++
	}
	switch m.phase {
	case tapIdle:
		if !isAlt || ev.Injected {
			// Never starts a tap. The held-key update above is enough: a
			// later Alt press sees this key and is canceled.
			return
		}
		if otherHeld || wasDown {
			// Alt pressed over already-held keys, or a repeat of an Alt whose
			// fresh press we never observed (e.g. held across a resync):
			// no tap until this Alt is released.
			m.phase = tapCanceled
			m.targetVK = ev.VK
			return
		}
		m.phase = tapTracking
		m.targetVK = ev.VK
		m.downTime = ev.Time
		act.BeginTap = true
	case tapTracking:
		if ev.VK == m.targetVK && !ev.Injected {
			return // auto-repeat of the tracked Alt
		}
		// Opposite Alt, any other key, or an injected duplicate of the
		// target: the tap is no longer "empty".
		m.phase = tapCanceled
	case tapCanceled:
		// Stay canceled until the target Alt is released.
	}
}

func (m *TapMachine) feedUp(ev KeyEvent, act *TapAction) {
	if m.held[ev.VK] {
		m.held[ev.VK] = false
		m.heldCount--
	}
	if ev.VK != m.targetVK {
		if m.phase == tapTracking {
			// A key we did not believe to be down was released mid-tap: the
			// held-key view is stale, so fail toward not switching.
			m.phase = tapCanceled
		}
		return
	}
	switch m.phase {
	case tapTracking:
		if ev.Injected {
			// The tracked Alt was "released" by an injected event while the
			// physical key state is unknown: fail safe, wait for the real up.
			m.phase = tapCanceled
			return
		}
		act.EndTap = true
		if ev.Time-m.downTime <= m.maxHoldMs { // uint32 wraparound-safe
			act.Dispatch = true
			act.ImeOpen = ev.VK == win32.VkRMenu
			act.TriggerVK = ev.VK
		}
		m.phase = tapIdle
		m.targetVK = 0
	case tapCanceled:
		m.phase = tapIdle
		m.targetVK = 0
	}
}
