package main

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

// keyEvent is one keyboard transition observed by the low-level hook.
// Self-injected events (LLKHF_INJECTED with ownInputTag) must be filtered
// out by the caller before feeding; injected events from other processes are
// fed with injected=true because they still reach the target application and
// therefore cancel a tap in progress.
type keyEvent struct {
	vk       uint32
	down     bool
	injected bool
	time     uint32 // KBDLLHOOKSTRUCT.time; wraps at 2^32 ms
}

// tapAction is what the hook layer must do after feeding one event.
type tapAction struct {
	// beginTap: a physical Alt press just started a clean tap candidate.
	// This is the only point where the menu-focus suppressor may be sent.
	beginTap bool
	// dispatch: a tap completed; request an IME switch.
	dispatch  bool
	imeOpen   bool   // dispatch: true = IME ON (right Alt), false = OFF (left Alt)
	triggerVK uint32 // dispatch: the Alt that triggered, for release re-checks
}

type tapMachine struct {
	phase     tapPhase
	targetVK  uint32
	downTime  uint32
	maxHoldMs uint32
	// held tracks every non-self-injected key currently down, indexed by VK.
	// Fixed-size so feed() performs no allocation inside the hook callback.
	held      [256]bool
	heldCount int
}

func newTapMachine(maxHoldMs uint32) *tapMachine {
	return &tapMachine{maxHoldMs: maxHoldMs}
}

// resync replaces the held-key view and returns the machine to idle. Callers
// run it outside the hook callback (startup, enable/disable, session unlock,
// power resume) with the keys the OS currently reports as down, so keys held
// from before tracking started still cancel taps.
func (m *tapMachine) resync(down []uint32) {
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
func (m *tapMachine) invalidate() {
	if m.phase == tapTracking {
		m.phase = tapCanceled
	}
}

func (m *tapMachine) otherHeld(vk uint32) bool {
	n := m.heldCount
	if m.held[vk] {
		n--
	}
	return n > 0
}

// feed consumes one keyboard transition and reports the required action.
func (m *tapMachine) feed(ev keyEvent) tapAction {
	var act tapAction
	if ev.vk == 0 || ev.vk >= 256 {
		// Out-of-contract VK (KBDLLHOOKSTRUCT documents 1..254): treat as an
		// inconsistent event.
		m.invalidate()
		return act
	}
	if ev.down {
		m.feedDown(ev, &act)
	} else {
		m.feedUp(ev, &act)
	}
	return act
}

func (m *tapMachine) feedDown(ev keyEvent, act *tapAction) {
	isAlt := ev.vk == vkLMenu || ev.vk == vkRMenu
	otherHeld := m.otherHeld(ev.vk)
	wasDown := m.held[ev.vk]
	if !wasDown {
		m.held[ev.vk] = true
		m.heldCount++
	}
	switch m.phase {
	case tapIdle:
		if !isAlt || ev.injected {
			// Never starts a tap. The held-key update above is enough: a
			// later Alt press sees this key and is canceled.
			return
		}
		if otherHeld || wasDown {
			// Alt pressed over already-held keys, or a repeat of an Alt whose
			// fresh press we never observed (e.g. held across a resync):
			// no tap until this Alt is released.
			m.phase = tapCanceled
			m.targetVK = ev.vk
			return
		}
		m.phase = tapTracking
		m.targetVK = ev.vk
		m.downTime = ev.time
		act.beginTap = true
	case tapTracking:
		if ev.vk == m.targetVK && !ev.injected {
			return // auto-repeat of the tracked Alt
		}
		// Opposite Alt, any other key, or an injected duplicate of the
		// target: the tap is no longer "empty".
		m.phase = tapCanceled
	case tapCanceled:
		// Stay canceled until the target Alt is released.
	}
}

func (m *tapMachine) feedUp(ev keyEvent, act *tapAction) {
	if m.held[ev.vk] {
		m.held[ev.vk] = false
		m.heldCount--
	}
	if ev.vk != m.targetVK {
		if m.phase == tapTracking {
			// A key we did not believe to be down was released mid-tap: the
			// held-key view is stale, so fail toward not switching.
			m.phase = tapCanceled
		}
		return
	}
	switch m.phase {
	case tapTracking:
		if ev.injected {
			// The tracked Alt was "released" by an injected event while the
			// physical key state is unknown: fail safe, wait for the real up.
			m.phase = tapCanceled
			return
		}
		if ev.time-m.downTime <= m.maxHoldMs { // uint32 wraparound-safe
			act.dispatch = true
			act.imeOpen = ev.vk == vkRMenu
			act.triggerVK = ev.vk
		}
		m.phase = tapIdle
		m.targetVK = 0
	case tapCanceled:
		m.phase = tapIdle
		m.targetVK = 0
	}
}
