package main

// Enter guard state machine. In guard-target applications a plain Enter is
// blocked and replaced with Shift+Enter (newline) and Ctrl+Enter is blocked
// and replaced with a plain Enter (send). Pure logic with no Win32
// dependency so the full transition table stays unit-testable on any host.
// The machine is owned exclusively by the hook thread; nothing here is safe
// for concurrent use.

type guardPhase int

const (
	guardIdle guardPhase = iota
	// guardSwallow: a physical Enter down was blocked; every event of the
	// same physical press (auto-repeat downs and the matching up) is
	// swallowed too, so the target never observes an unmatched transition.
	guardSwallow
)

// guardAction is what the hook layer must do after feeding one event.
type guardAction struct {
	// block: consume this event instead of passing it down the hook chain.
	block bool
	// injectNewline: emit the tagged Shift+Enter replacement (plain Enter).
	injectNewline bool
	// injectSend: emit the tagged plain-Enter replacement (Ctrl+Enter). The
	// hook layer decides which physical Ctrl side(s) to release around it.
	injectSend bool
}

type guardMachine struct {
	phase guardPhase
	// mods tracks the non-self-injected modifier keys currently down,
	// indexed by VK. Only the eight side-specific modifier codes are ever
	// set; fixed-size so feed() performs no allocation inside the callback.
	mods [256]bool
}

func newGuardMachine() *guardMachine {
	return &guardMachine{}
}

// isGuardModifier reports whether vk is one of the side-specific modifier
// codes the guard tracks. Generic VK_SHIFT/VK_CONTROL/VK_MENU must be
// normalized before feeding (normalizeAltVK / normalizeModVK).
func isGuardModifier(vk uint32) bool {
	switch vk {
	case vkLShift, vkRShift, vkLControl, vkRControl, vkLMenu, vkRMenu, vkLWin, vkRWin:
		return true
	}
	return false
}

// normalizeModVK converts generic VK_SHIFT/VK_CONTROL events into the
// side-specific codes the guard tracks. Physical events already arrive
// side-specific from the low-level hook; generic codes appear only in
// injected input. Right Ctrl is an extended key; Shift carries no side
// information in the flags, so the left code stands in for either side
// (the guard only ever asks "is any Shift down").
func normalizeModVK(vk uint32, extended bool) uint32 {
	switch vk {
	case vkShift:
		return vkLShift
	case vkControl:
		if extended {
			return vkRControl
		}
		return vkLControl
	}
	return vk
}

// resync replaces the modifier view and returns the machine to idle.
// Callers run it outside the hook callback (startup, enable/disable,
// session unlock, power resume) with the keys the OS currently reports as
// down. A press swallowed by the resync leaves an orphan Enter up, which
// passes through harmlessly because idle ignores ups.
func (m *guardMachine) resync(down []uint32) {
	m.mods = [256]bool{}
	for _, vk := range down {
		if vk < 256 && isGuardModifier(vk) {
			m.mods[vk] = true
		}
	}
	m.phase = guardIdle
}

func (m *guardMachine) anyDown(vks ...uint32) bool {
	for _, vk := range vks {
		if m.mods[vk] {
			return true
		}
	}
	return false
}

// feed consumes one keyboard transition and reports the required action.
// active is evaluated by the hook layer only for Enter events: guard
// enabled and the foreground window is a guard target. It is the single
// gate where a future "IME composition in progress" veto belongs.
func (m *guardMachine) feed(ev keyEvent, active bool) guardAction {
	var act guardAction
	if ev.vk == 0 || ev.vk >= 256 {
		return act
	}
	if isGuardModifier(ev.vk) {
		m.mods[ev.vk] = ev.down
		return act
	}
	if ev.vk != vkReturn {
		return act
	}
	switch m.phase {
	case guardIdle:
		if !ev.down || !active || ev.injected {
			return act
		}
		if m.anyDown(vkLShift, vkRShift, vkLMenu, vkRMenu, vkLWin, vkRWin) {
			return act // Shift/Alt/Win chords pass through untouched
		}
		act.block = true
		if m.anyDown(vkLControl, vkRControl) {
			act.injectSend = true
		} else {
			act.injectNewline = true
		}
		m.phase = guardSwallow
	case guardSwallow:
		if ev.injected {
			// Another process injected an Enter while we are swallowing a
			// physical press: leave third-party input alone.
			return act
		}
		act.block = true // auto-repeat down or the matching physical up
		if !ev.down {
			m.phase = guardIdle
		}
	}
	return act
}
