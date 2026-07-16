package main

// Enter guard state machine. In guard-target applications a plain Enter is
// blocked and replaced with Shift+Enter (newline) and Ctrl+Enter is blocked
// and replaced with a plain Enter (send). The machine only decides to
// consume the physical press; the replacement itself is chosen and injected
// by the UI thread (two-stage dispatch), which can also check the target's
// IME open status with a bounded call — something this hook-side logic must
// never do. Pure logic with no Win32 dependency so the full transition
// table stays unit-testable on any host. The machine is owned exclusively
// by the hook thread; nothing here is safe for concurrent use.

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
	// dispatch: a guarded Enter was consumed; forward a replacement request
	// to the UI thread carrying the two fields below.
	dispatch bool
	// send: Ctrl (and no other modifier) was held — the user asked to send.
	send bool
	// composing: an IME composition is believed to be in progress (CON-9
	// heuristic): a composition-starting key was seen after the last event
	// known to commit or cancel one. The UI thread combines this with the
	// target's actual IME open status before overriding the replacement.
	composing bool
}

type guardMachine struct {
	phase guardPhase
	// mods tracks the non-self-injected modifier keys currently down,
	// indexed by VK. Only the eight side-specific modifier codes are ever
	// set; fixed-size so feed() performs no allocation inside the callback.
	mods [256]bool
	// composing is the CON-9 heuristic bit, maintained from the key stream
	// (this machine cannot see the real cross-process composition state).
	// Stale-true fails open: the Enter is delivered as-is, matching the
	// target's default behavior. Stale-false re-breaks composition commit,
	// so the events that clear it are kept to the ones that reliably end a
	// composition (Enter, Escape, IME toggles, focus change, resync).
	composing bool
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

// isCompositionStarter reports whether a key press plausibly starts or
// extends an IME composition: letters, digits, and the OEM punctuation
// range. Numpad keys are excluded (Microsoft IME commits them directly by
// default) and so is Space, which converts an existing composition but
// never starts one.
func isCompositionStarter(vk uint32) bool {
	switch {
	case vk >= 0x30 && vk <= 0x39: // 0-9
		return true
	case vk >= 0x41 && vk <= 0x5A: // A-Z
		return true
	case vk >= 0xBA && vk <= 0xC0: // OEM_1..OEM_3 (;: /? @` etc.)
		return true
	case vk >= 0xDB && vk <= 0xDF: // OEM_4..OEM_8 ([{ \| ]} '~ etc.)
		return true
	case vk == 0xE2: // OEM_102
		return true
	}
	return false
}

// endsComposition reports the keys that reliably commit or cancel a
// composition besides Enter: Escape and the IME mode toggles (which commit
// any pending text before switching).
func endsComposition(vk uint32) bool {
	switch vk {
	case vkEscape, vkKana, vkKanji, vkImeOn, vkImeOff, vkOemAuto, vkOemEnlw:
		return true
	}
	return false
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
	m.composing = false
}

// clearComposing marks any believed composition as ended. The hook layer
// calls it when the foreground window changes, because losing focus commits
// or cancels a composition.
func (m *guardMachine) clearComposing() {
	m.composing = false
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
// enabled and the foreground window is a guard target.
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
		if ev.down {
			// Injected keys from other processes reach the target too, so
			// they move the composition belief exactly like physical ones.
			if isCompositionStarter(ev.vk) {
				m.composing = true
			} else if endsComposition(ev.vk) {
				m.composing = false
			}
		}
		return act
	}
	// Enter: whether it is guarded, passed through, or injected by another
	// process, a down commits or replaces whatever composition was open.
	composingAtPress := m.composing
	if ev.down {
		m.composing = false
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
		act.dispatch = true
		act.send = m.anyDown(vkLControl, vkRControl)
		act.composing = composingAtPress
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
