package hookstate

import (
	"testing"

	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

type guardStep struct {
	ev     KeyEvent
	active bool
	want   GuardAction
}

func runGuard(t *testing.T, m *GuardMachine, steps []guardStep) {
	t.Helper()
	for i, s := range steps {
		got := m.Feed(s.ev, s.active)
		if got != s.want {
			t.Fatalf("step %d (%+v, active=%t): got %+v, want %+v", i, s.ev, s.active, got, s.want)
		}
	}
}

func pass() GuardAction    { return GuardAction{} }
func swallow() GuardAction { return GuardAction{Block: true} }
func dispatched(send, composing bool) GuardAction {
	return GuardAction{Block: true, Dispatch: true, Send: send, Composing: composing}
}

func TestNormalizeModVK(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint32
		extended bool
		want     uint32
	}{
		{"generic Shift", win32.VkShift, false, win32.VkLShift},
		{"generic Shift extended flag ignored", win32.VkShift, true, win32.VkLShift},
		{"generic left Ctrl", win32.VkControl, false, win32.VkLControl},
		{"generic right Ctrl", win32.VkControl, true, win32.VkRControl},
		{"specific right Shift", win32.VkRShift, false, win32.VkRShift},
		{"specific left Ctrl", win32.VkLControl, true, win32.VkLControl},
		{"non-modifier", win32.VkReturn, true, win32.VkReturn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeModVK(tt.vk, tt.extended); got != tt.want {
				t.Fatalf("NormalizeModVK(%#x, %t) = %#x, want %#x", tt.vk, tt.extended, got, tt.want)
			}
		})
	}
}

func TestGuardPlainEnterDispatchesNewline(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), true, dispatched(false, false)},
		{up(win32.VkReturn, 30), true, swallow()},
	})
	if m.phase != guardIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestGuardAutoRepeatSwallowedWithoutRedispatch(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), true, dispatched(false, false)},
		{down(win32.VkReturn, 100), true, swallow()},
		{down(win32.VkReturn, 200), true, swallow()},
		{up(win32.VkReturn, 300), true, swallow()},
	})
}

func TestGuardCtrlEnterDispatchesSend(t *testing.T) {
	for _, ctrl := range []uint32{win32.VkLControl, win32.VkRControl} {
		m := NewGuardMachine()
		runGuard(t, m, []guardStep{
			{down(ctrl, 0), true, pass()},
			{down(win32.VkReturn, 10), true, dispatched(true, false)},
			{up(win32.VkReturn, 30), true, swallow()},
			{up(ctrl, 50), true, pass()},
		})
	}
}

func TestGuardBothCtrlSidesDispatchSend(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkLControl, 0), true, pass()},
		{down(win32.VkRControl, 5), true, pass()},
		{down(win32.VkReturn, 10), true, dispatched(true, false)},
		{up(win32.VkReturn, 30), true, swallow()},
	})
}

func TestGuardCtrlReleasedBeforeEnterUp(t *testing.T) {
	// Ctrl up mid-swallow must not change how the pending Enter up is eaten.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkLControl, 0), true, pass()},
		{down(win32.VkReturn, 10), true, dispatched(true, false)},
		{up(win32.VkLControl, 20), true, pass()},
		{up(win32.VkReturn, 30), true, swallow()},
		// The next plain Enter dispatches a newline again.
		{down(win32.VkReturn, 100), true, dispatched(false, false)},
		{up(win32.VkReturn, 130), true, swallow()},
	})
}

func TestGuardOtherModifierChordsPassThrough(t *testing.T) {
	for _, mod := range []uint32{win32.VkLShift, win32.VkRShift, win32.VkLMenu, win32.VkRMenu, win32.VkLWin, win32.VkRWin} {
		m := NewGuardMachine()
		runGuard(t, m, []guardStep{
			{down(mod, 0), true, pass()},
			{down(win32.VkReturn, 10), true, pass()},
			{up(win32.VkReturn, 30), true, pass()},
			{up(mod, 50), true, pass()},
		})
	}
}

func TestGuardShiftWinsOverCtrl(t *testing.T) {
	// Ctrl+Shift+Enter is not a plain-send request: stay out of the way.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkLControl, 0), true, pass()},
		{down(win32.VkLShift, 5), true, pass()},
		{down(win32.VkReturn, 10), true, pass()},
	})
}

func TestGuardInactivePassesThrough(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), false, pass()},
		{up(win32.VkReturn, 30), false, pass()},
	})
}

func TestGuardInjectedEnterPassesThrough(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{injDown(win32.VkReturn, 0), true, pass()},
		{injUp(win32.VkReturn, 30), true, pass()},
	})
}

func TestGuardInjectedEnterDuringSwallowPassesThrough(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), true, dispatched(false, false)},
		{injDown(win32.VkReturn, 10), true, pass()},
		{injUp(win32.VkReturn, 20), true, pass()},
		// The physical up of the swallowed press is still eaten.
		{up(win32.VkReturn, 30), true, swallow()},
	})
}

func TestGuardDeactivatedMidPressStillSwallowsUp(t *testing.T) {
	// Foreground changed between down and up: eat the up anyway so the new
	// app never sees an unmatched Enter release.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), true, dispatched(false, false)},
		{up(win32.VkReturn, 30), false, swallow()},
	})
}

func TestGuardOrphanEnterUpPassesThrough(t *testing.T) {
	// The down passed through (inactive) or predates tracking: idle ignores ups.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{up(win32.VkReturn, 0), true, pass()},
	})
}

func TestGuardResyncSeesHeldCtrl(t *testing.T) {
	m := NewGuardMachine()
	m.Resync([]uint32{win32.VkLControl})
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 0), true, dispatched(true, false)},
		{up(win32.VkReturn, 30), true, swallow()},
	})
}

func TestGuardResyncClearsSwallowAndMods(t *testing.T) {
	m := NewGuardMachine()
	m.Feed(down(win32.VkLShift, 0), true)
	m.Feed(down(win32.VkReturn, 10), true) // passes (Shift held), phase stays idle
	m.Feed(up(win32.VkReturn, 20), true)
	m.Feed(down(win32.VkReturn, 30), false)
	m.Resync(nil)
	if m.phase != guardIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
	runGuard(t, m, []guardStep{
		// Shift held before resync is forgotten; a plain Enter guards again.
		{down(win32.VkReturn, 100), true, dispatched(false, false)},
		{up(win32.VkReturn, 130), true, swallow()},
	})
}

func TestGuardResyncDuringSwallowDropsThePress(t *testing.T) {
	m := NewGuardMachine()
	m.Feed(down(win32.VkReturn, 0), true) // blocked, swallow
	m.Resync(nil)
	runGuard(t, m, []guardStep{
		// The orphan up of the dropped press passes through harmlessly.
		{up(win32.VkReturn, 30), true, pass()},
	})
}

func TestGuardOutOfRangeVKIsIgnored(t *testing.T) {
	m := NewGuardMachine()
	if got := m.Feed(KeyEvent{VK: 0, Down: true}, true); got != pass() {
		t.Fatalf("vk 0 produced action: %+v", got)
	}
	if got := m.Feed(KeyEvent{VK: 0x1FF, Down: true}, true); got != pass() {
		t.Fatalf("vk 0x1FF produced action: %+v", got)
	}
}

// ---- composition belief (CON-9 heuristic) ----

func TestGuardTypingMarksComposingAndEnterClearsIt(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(0x41 /*A*/, 0), true, pass()},
		{up(0x41, 10), true, pass()},
		// Enter after typing carries the composing belief to the UI...
		{down(win32.VkReturn, 20), true, dispatched(false, true)},
		{up(win32.VkReturn, 40), true, swallow()},
		// ...and the commit clears it: the next Enter is a plain newline.
		{down(win32.VkReturn, 100), true, dispatched(false, false)},
		{up(win32.VkReturn, 130), true, swallow()},
	})
}

func TestGuardCompositionStarters(t *testing.T) {
	starters := []uint32{0x30, 0x39, 0x41, 0x5A, 0xBA, 0xC0, 0xDB, 0xDF, 0xE2}
	for _, vk := range starters {
		m := NewGuardMachine()
		m.Feed(down(vk, 0), true)
		if !m.composing {
			t.Errorf("vk %#x did not set composing", vk)
		}
	}
	nonStarters := []uint32{0x20 /*Space*/, 0x08 /*Backspace*/, 0x09 /*Tab*/, 0x25 /*Left*/, 0x60 /*Numpad0*/, 0x70 /*F1*/}
	for _, vk := range nonStarters {
		m := NewGuardMachine()
		m.Feed(down(vk, 0), true)
		if m.composing {
			t.Errorf("vk %#x set composing", vk)
		}
	}
}

func TestGuardCompositionEnders(t *testing.T) {
	enders := []uint32{win32.VkEscape, win32.VkKana, win32.VkKanji, win32.VkImeOn, win32.VkImeOff, win32.VkOemAuto, win32.VkOemEnlw}
	for _, vk := range enders {
		m := NewGuardMachine()
		m.Feed(down(0x41, 0), true)
		m.Feed(down(vk, 10), true)
		if m.composing {
			t.Errorf("vk %#x did not clear composing", vk)
		}
	}
}

func TestGuardCompositionSurvivesEditingKeys(t *testing.T) {
	// Space (conversion), Backspace, arrows and modifiers keep the belief:
	// the composition is still open until something commits it.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(0x41, 0), true, pass()},
		{down(0x20 /*Space*/, 10), true, pass()},
		{down(0x08 /*Backspace*/, 20), true, pass()},
		{down(0x28 /*Down*/, 30), true, pass()},
		{down(win32.VkLShift, 40), true, pass()},
		{up(win32.VkLShift, 50), true, pass()},
		{down(win32.VkReturn, 60), true, dispatched(false, true)},
	})
}

func TestGuardInjectedPrintableSetsComposing(t *testing.T) {
	// Injected input from other tools reaches the target and can compose.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{injDown(0x41, 0), true, pass()},
		{down(win32.VkReturn, 10), true, dispatched(false, true)},
	})
}

func TestGuardCtrlEnterCarriesComposing(t *testing.T) {
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(0x41, 0), true, pass()},
		{down(win32.VkLControl, 10), true, pass()},
		{down(win32.VkReturn, 20), true, dispatched(true, true)},
	})
}

func TestGuardPassedEnterAlsoClearsComposing(t *testing.T) {
	// An Enter that passes through (inactive) still commits the composition.
	m := NewGuardMachine()
	runGuard(t, m, []guardStep{
		{down(0x41, 0), true, pass()},
		{down(win32.VkReturn, 10), false, pass()},
		{up(win32.VkReturn, 20), false, pass()},
		{down(win32.VkReturn, 100), true, dispatched(false, false)},
	})
}

func TestGuardClearComposing(t *testing.T) {
	// Focus change (hook layer) commits or cancels the composition.
	m := NewGuardMachine()
	m.Feed(down(0x41, 0), true)
	m.ClearComposing()
	runGuard(t, m, []guardStep{
		{down(win32.VkReturn, 10), true, dispatched(false, false)},
	})
}

func TestGuardResyncClearsComposing(t *testing.T) {
	m := NewGuardMachine()
	m.Feed(down(0x41, 0), true)
	m.Resync(nil)
	if m.composing {
		t.Fatal("resync did not clear composing")
	}
}
