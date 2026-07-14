package main

import "testing"

// Shorthand event constructors.
func down(vk uint32, t uint32) keyEvent { return keyEvent{vk: vk, down: true, time: t} }
func up(vk uint32, t uint32) keyEvent   { return keyEvent{vk: vk, down: false, time: t} }
func injDown(vk uint32, t uint32) keyEvent {
	return keyEvent{vk: vk, down: true, injected: true, time: t}
}
func injUp(vk uint32, t uint32) keyEvent {
	return keyEvent{vk: vk, down: false, injected: true, time: t}
}

type step struct {
	ev   keyEvent
	want tapAction
}

func run(t *testing.T, m *tapMachine, steps []step) {
	t.Helper()
	for i, s := range steps {
		got := m.feed(s.ev)
		if got != s.want {
			t.Fatalf("step %d (%+v): got %+v, want %+v", i, s.ev, got, s.want)
		}
	}
}

func begin() tapAction { return tapAction{beginTap: true} }
func none() tapAction  { return tapAction{} }
func fire(vk uint32) tapAction {
	return tapAction{dispatch: true, imeOpen: vk == vkRMenu, triggerVK: vk}
}

func TestNormalizeAltVK(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint32
		extended bool
		want     uint32
	}{
		{"generic left Alt", vkMenu, false, vkLMenu},
		{"generic right Alt", vkMenu, true, vkRMenu},
		{"specific left Alt", vkLMenu, false, vkLMenu},
		{"specific left Alt extended flag ignored", vkLMenu, true, vkLMenu},
		{"specific right Alt", vkRMenu, true, vkRMenu},
		{"specific right Alt without extended flag", vkRMenu, false, vkRMenu},
		{"non-Alt", 0x41, true, 0x41},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAltVK(tt.vk, tt.extended); got != tt.want {
				t.Fatalf("normalizeAltVK(%#x, %t) = %#x, want %#x", tt.vk, tt.extended, got, tt.want)
			}
		})
	}
}

func TestGenericAltEventsFireAfterNormalization(t *testing.T) {
	tests := []struct {
		name     string
		extended bool
		wantVK   uint32
	}{
		{"left Alt", false, vkLMenu},
		{"right Alt", true, vkRMenu},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTapMachine(500)
			vk := normalizeAltVK(vkMenu, tt.extended)
			run(t, m, []step{
				{down(vk, 1000), begin()},
				{up(vk, 1100), fire(tt.wantVK)},
			})
		})
	}
}

func TestTapLeftAltFiresImeOff(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 1000), begin()},
		{up(vkLMenu, 1100), fire(vkLMenu)},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestTapRightAltFiresImeOn(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkRMenu, 0), begin()},
		{up(vkRMenu, 499), fire(vkRMenu)},
	})
}

func TestHoldBoundary(t *testing.T) {
	// Exactly maxHoldMs still fires; one millisecond beyond does not.
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 1000), begin()},
		{up(vkLMenu, 1500), fire(vkLMenu)},
		{down(vkLMenu, 2000), begin()},
		{up(vkLMenu, 2501), none()},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle after expired tap", m.phase)
	}
}

func TestTimeWraparound(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0xFFFFFF00), begin()},
		{up(vkLMenu, 0x00000030), fire(vkLMenu)}, // elapsed 0x130 = 304ms
	})
}

func TestModifierHeldBeforeAltCancels(t *testing.T) {
	for _, mod := range []uint32{0xA0 /*LShift*/, 0xA2 /*LCtrl*/, 0x5B /*LWin*/} {
		m := newTapMachine(500)
		run(t, m, []step{
			{down(mod, 0), none()},
			{down(vkLMenu, 10), none()}, // canceled: no beginTap, no suppressor
			{up(vkLMenu, 50), none()},
			{up(mod, 60), none()},
			// A clean tap afterwards works again.
			{down(vkLMenu, 100), begin()},
			{up(vkLMenu, 150), fire(vkLMenu)},
		})
	}
}

func TestChordCancels(t *testing.T) {
	// Alt+Tab style chord: another key pressed during tracking.
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{down(0x09 /*Tab*/, 20), none()},
		{up(0x09, 40), none()},
		{up(vkLMenu, 60), none()}, // canceled tap never fires
		{down(vkLMenu, 100), begin()},
		{up(vkLMenu, 120), fire(vkLMenu)},
	})
}

func TestBothAltsCancel(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{down(vkRMenu, 10), none()}, // opposite Alt cancels
		{up(vkRMenu, 20), none()},
		{up(vkLMenu, 30), none()},
		{down(vkRMenu, 100), begin()},
		{up(vkRMenu, 130), fire(vkRMenu)},
	})
}

func TestAltRepeatIsIgnored(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{down(vkLMenu, 100), none()}, // auto-repeat: still tracking
		{down(vkLMenu, 200), none()},
		{up(vkLMenu, 300), fire(vkLMenu)},
	})
}

func TestInjectedAltDoesNotStartTap(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{injDown(vkLMenu, 0), none()}, // no beginTap: injected Alt never tracks
		{injUp(vkLMenu, 50), none()},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestInjectedKeyCancelsTracking(t *testing.T) {
	// External tools' injected input reaches the target app, so it cancels.
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{injDown(0x41 /*A*/, 10), none()},
		{injUp(0x41, 20), none()},
		{up(vkLMenu, 30), none()}, // canceled
	})
}

func TestInjectedTargetAltRepeatCancels(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{injDown(vkLMenu, 10), none()}, // injected duplicate of the target
		{up(vkLMenu, 30), none()},
	})
}

func TestInjectedTargetAltUpCancels(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{injUp(vkLMenu, 10), none()}, // injected release: physical state unknown
		{up(vkLMenu, 30), none()},    // canceled -> idle on the physical up
		{down(vkLMenu, 100), begin()},
		{up(vkLMenu, 130), fire(vkLMenu)},
	})
}

func TestResyncHeldKeyBlocksTap(t *testing.T) {
	// Keys already down at startup/re-enable must cancel taps (resync).
	m := newTapMachine(500)
	m.resync([]uint32{0x41})
	run(t, m, []step{
		{down(vkLMenu, 0), none()}, // canceled: 'A' held from before
		{up(vkLMenu, 30), none()},
		{up(0x41, 40), none()},
		{down(vkLMenu, 100), begin()},
		{up(vkLMenu, 130), fire(vkLMenu)},
	})
}

func TestResyncWithHeldAltThenRepeat(t *testing.T) {
	// Alt physically held across an enable: its repeats must not tap.
	m := newTapMachine(500)
	m.resync([]uint32{vkLMenu})
	run(t, m, []step{
		{down(vkLMenu, 0), none()}, // repeat of a press we never saw
		{up(vkLMenu, 30), none()},
		{down(vkLMenu, 100), begin()},
		{up(vkLMenu, 130), fire(vkLMenu)},
	})
}

func TestResyncClearsTracking(t *testing.T) {
	m := newTapMachine(500)
	m.feed(down(vkLMenu, 0))
	m.resync(nil)
	if m.phase != tapIdle || m.heldCount != 0 {
		t.Fatalf("resync did not reset: phase=%v held=%d", m.phase, m.heldCount)
	}
	// The Alt-up for the press consumed by resync must not fire.
	if got := m.feed(up(vkLMenu, 100)); got != none() {
		t.Fatalf("up after resync fired: %+v", got)
	}
}

func TestInvalidateCancelsTracking(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{{down(vkLMenu, 0), begin()}})
	m.invalidate()
	run(t, m, []step{
		{up(vkLMenu, 30), none()},
		{down(vkLMenu, 100), begin()},
		{up(vkLMenu, 130), fire(vkLMenu)},
	})
}

func TestInvalidateWhileIdleIsHarmless(t *testing.T) {
	m := newTapMachine(500)
	m.invalidate()
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{up(vkLMenu, 30), fire(vkLMenu)},
	})
}

func TestUnexpectedUpDuringTrackingCancels(t *testing.T) {
	// An up for a key we never saw go down means our view is stale.
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{up(0x41, 10), none()}, // 'A' up, but 'A' was not held
		{up(vkLMenu, 30), none()},
	})
}

func TestOutOfRangeVKInvalidates(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{down(0x1FF, 10), none()}, // out of the documented 1..254 contract
		{up(vkLMenu, 30), none()}, // canceled by the invalidation
	})
	if got := m.feed(keyEvent{vk: 0, down: true, time: 40}); got != none() {
		t.Fatalf("vk 0 produced action: %+v", got)
	}
}

func TestConsecutiveTaps(t *testing.T) {
	m := newTapMachine(500)
	run(t, m, []step{
		{down(vkLMenu, 0), begin()},
		{up(vkLMenu, 100), fire(vkLMenu)},
		{down(vkRMenu, 200), begin()},
		{up(vkRMenu, 300), fire(vkRMenu)},
		{down(vkLMenu, 400), begin()},
		{up(vkLMenu, 450), fire(vkLMenu)},
	})
}

func TestKeyHeldAcrossTapBlocksUntilReleased(t *testing.T) {
	// Key stays down over two Alt presses; both are canceled.
	m := newTapMachine(500)
	run(t, m, []step{
		{down(0x41, 0), none()},
		{down(vkLMenu, 10), none()},
		{up(vkLMenu, 20), none()},
		{down(vkLMenu, 30), none()},
		{up(vkLMenu, 40), none()},
		{up(0x41, 50), none()},
		{down(vkLMenu, 60), begin()},
		{up(vkLMenu, 70), fire(vkLMenu)},
	})
}
