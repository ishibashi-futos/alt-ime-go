package main

import "testing"

type guardStep struct {
	ev     keyEvent
	active bool
	want   guardAction
}

func runGuard(t *testing.T, m *guardMachine, steps []guardStep) {
	t.Helper()
	for i, s := range steps {
		got := m.feed(s.ev, s.active)
		if got != s.want {
			t.Fatalf("step %d (%+v, active=%t): got %+v, want %+v", i, s.ev, s.active, got, s.want)
		}
	}
}

func pass() guardAction    { return guardAction{} }
func newline() guardAction { return guardAction{block: true, injectNewline: true} }
func send() guardAction    { return guardAction{block: true, injectSend: true} }
func swallow() guardAction { return guardAction{block: true} }

func TestNormalizeModVK(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint32
		extended bool
		want     uint32
	}{
		{"generic Shift", vkShift, false, vkLShift},
		{"generic Shift extended flag ignored", vkShift, true, vkLShift},
		{"generic left Ctrl", vkControl, false, vkLControl},
		{"generic right Ctrl", vkControl, true, vkRControl},
		{"specific right Shift", vkRShift, false, vkRShift},
		{"specific left Ctrl", vkLControl, true, vkLControl},
		{"non-modifier", vkReturn, true, vkReturn},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeModVK(tt.vk, tt.extended); got != tt.want {
				t.Fatalf("normalizeModVK(%#x, %t) = %#x, want %#x", tt.vk, tt.extended, got, tt.want)
			}
		})
	}
}

func TestGuardPlainEnterBecomesNewline(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), true, newline()},
		{up(vkReturn, 30), true, swallow()},
	})
	if m.phase != guardIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestGuardAutoRepeatSwallowedWithoutReinjection(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), true, newline()},
		{down(vkReturn, 100), true, swallow()},
		{down(vkReturn, 200), true, swallow()},
		{up(vkReturn, 300), true, swallow()},
	})
}

func TestGuardCtrlEnterBecomesSend(t *testing.T) {
	for _, ctrl := range []uint32{vkLControl, vkRControl} {
		m := newGuardMachine()
		runGuard(t, m, []guardStep{
			{down(ctrl, 0), true, pass()},
			{down(vkReturn, 10), true, send()},
			{up(vkReturn, 30), true, swallow()},
			{up(ctrl, 50), true, pass()},
		})
	}
}

func TestGuardBothCtrlSidesBecomeSend(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkLControl, 0), true, pass()},
		{down(vkRControl, 5), true, pass()},
		{down(vkReturn, 10), true, send()},
		{up(vkReturn, 30), true, swallow()},
	})
}

func TestGuardCtrlReleasedBeforeEnterUp(t *testing.T) {
	// Ctrl up mid-swallow must not change how the pending Enter up is eaten.
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkLControl, 0), true, pass()},
		{down(vkReturn, 10), true, send()},
		{up(vkLControl, 20), true, pass()},
		{up(vkReturn, 30), true, swallow()},
		// The next plain Enter is a newline again.
		{down(vkReturn, 100), true, newline()},
		{up(vkReturn, 130), true, swallow()},
	})
}

func TestGuardOtherModifierChordsPassThrough(t *testing.T) {
	for _, mod := range []uint32{vkLShift, vkRShift, vkLMenu, vkRMenu, vkLWin, vkRWin} {
		m := newGuardMachine()
		runGuard(t, m, []guardStep{
			{down(mod, 0), true, pass()},
			{down(vkReturn, 10), true, pass()},
			{up(vkReturn, 30), true, pass()},
			{up(mod, 50), true, pass()},
		})
	}
}

func TestGuardShiftWinsOverCtrl(t *testing.T) {
	// Ctrl+Shift+Enter is not a plain-send request: stay out of the way.
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkLControl, 0), true, pass()},
		{down(vkLShift, 5), true, pass()},
		{down(vkReturn, 10), true, pass()},
	})
}

func TestGuardInactivePassesThrough(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), false, pass()},
		{up(vkReturn, 30), false, pass()},
	})
}

func TestGuardInjectedEnterPassesThrough(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{injDown(vkReturn, 0), true, pass()},
		{injUp(vkReturn, 30), true, pass()},
	})
}

func TestGuardInjectedEnterDuringSwallowPassesThrough(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), true, newline()},
		{injDown(vkReturn, 10), true, pass()},
		{injUp(vkReturn, 20), true, pass()},
		// The physical up of the swallowed press is still eaten.
		{up(vkReturn, 30), true, swallow()},
	})
}

func TestGuardDeactivatedMidPressStillSwallowsUp(t *testing.T) {
	// Foreground changed between down and up: eat the up anyway so the new
	// app never sees an unmatched Enter release.
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), true, newline()},
		{up(vkReturn, 30), false, swallow()},
	})
}

func TestGuardOrphanEnterUpPassesThrough(t *testing.T) {
	// The down passed through (inactive) or predates tracking: idle ignores ups.
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{up(vkReturn, 0), true, pass()},
	})
}

func TestGuardResyncSeesHeldCtrl(t *testing.T) {
	m := newGuardMachine()
	m.resync([]uint32{vkLControl})
	runGuard(t, m, []guardStep{
		{down(vkReturn, 0), true, send()},
		{up(vkReturn, 30), true, swallow()},
	})
}

func TestGuardResyncClearsSwallowAndMods(t *testing.T) {
	m := newGuardMachine()
	m.feed(down(vkLShift, 0), true)
	m.feed(down(vkReturn, 10), true) // passes (Shift held), phase stays idle
	m.feed(up(vkReturn, 20), true)
	m.feed(down(vkReturn, 30), false)
	m.resync(nil)
	if m.phase != guardIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
	runGuard(t, m, []guardStep{
		// Shift held before resync is forgotten; a plain Enter guards again.
		{down(vkReturn, 100), true, newline()},
		{up(vkReturn, 130), true, swallow()},
	})
}

func TestGuardResyncDuringSwallowDropsThePress(t *testing.T) {
	m := newGuardMachine()
	m.feed(down(vkReturn, 0), true) // blocked, swallow
	m.resync(nil)
	runGuard(t, m, []guardStep{
		// The orphan up of the dropped press passes through harmlessly.
		{up(vkReturn, 30), true, pass()},
	})
}

func TestGuardNonModifierKeysAreIgnored(t *testing.T) {
	m := newGuardMachine()
	runGuard(t, m, []guardStep{
		{down(0x41 /*A*/, 0), true, pass()},
		{down(vkReturn, 10), true, newline()},
		{up(vkReturn, 30), true, swallow()},
		{up(0x41, 40), true, pass()},
	})
}

func TestGuardOutOfRangeVKIsIgnored(t *testing.T) {
	m := newGuardMachine()
	if got := m.feed(keyEvent{vk: 0, down: true}, true); got != pass() {
		t.Fatalf("vk 0 produced action: %+v", got)
	}
	if got := m.feed(keyEvent{vk: 0x1FF, down: true}, true); got != pass() {
		t.Fatalf("vk 0x1FF produced action: %+v", got)
	}
}

func TestMatchGuardTarget(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Users\u\AppData\Local\AnthropicClaude\claude.exe`, true},
		{`C:\Program Files\WindowsApps\M365Copilot.exe`, true},
		{`C:\APPS\CLAUDE.EXE`, true},
		{"claude.exe", true},
		{`C:\Windows\notepad.exe`, false},
		{`C:\apps\claude.exe.bak`, false},
		{`C:\claude.exe\other.exe`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := matchGuardTarget(tt.path); got != tt.want {
			t.Errorf("matchGuardTarget(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}
