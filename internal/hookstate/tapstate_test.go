package hookstate

import (
	"testing"

	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

// Shorthand event constructors.
func down(vk uint32, t uint32) KeyEvent { return KeyEvent{VK: vk, Down: true, Time: t} }
func up(vk uint32, t uint32) KeyEvent   { return KeyEvent{VK: vk, Down: false, Time: t} }
func injDown(vk uint32, t uint32) KeyEvent {
	return KeyEvent{VK: vk, Down: true, Injected: true, Time: t}
}
func injUp(vk uint32, t uint32) KeyEvent {
	return KeyEvent{VK: vk, Down: false, Injected: true, Time: t}
}

type step struct {
	ev   KeyEvent
	want TapAction
}

func run(t *testing.T, m *TapMachine, steps []step) {
	t.Helper()
	for i, s := range steps {
		got := m.Feed(s.ev)
		if got != s.want {
			t.Fatalf("step %d (%+v): got %+v, want %+v", i, s.ev, got, s.want)
		}
	}
}

func begin() TapAction { return TapAction{BeginTap: true} }
func none() TapAction  { return TapAction{} }
func fire(vk uint32) TapAction {
	return TapAction{EndTap: true, Dispatch: true, ImeOpen: vk == win32.VkRMenu, TriggerVK: vk}
}

func endTap() TapAction { return TapAction{EndTap: true} }

func TestNormalizeAltVK(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint32
		extended bool
		want     uint32
	}{
		{"generic left Alt", win32.VkMenu, false, win32.VkLMenu},
		{"generic right Alt", win32.VkMenu, true, win32.VkRMenu},
		{"specific left Alt", win32.VkLMenu, false, win32.VkLMenu},
		{"specific left Alt extended flag ignored", win32.VkLMenu, true, win32.VkLMenu},
		{"specific right Alt", win32.VkRMenu, true, win32.VkRMenu},
		{"specific right Alt without extended flag", win32.VkRMenu, false, win32.VkRMenu},
		{"non-Alt", 0x41, true, 0x41},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeAltVK(tt.vk, tt.extended); got != tt.want {
				t.Fatalf("NormalizeAltVK(%#x, %t) = %#x, want %#x", tt.vk, tt.extended, got, tt.want)
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
		{"left Alt", false, win32.VkLMenu},
		{"right Alt", true, win32.VkRMenu},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewTapMachine(500)
			vk := NormalizeAltVK(win32.VkMenu, tt.extended)
			run(t, m, []step{
				{down(vk, 1000), begin()},
				{up(vk, 1100), fire(tt.wantVK)},
			})
		})
	}
}

func TestTapLeftAltFiresImeOff(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 1000), begin()},
		{up(win32.VkLMenu, 1100), fire(win32.VkLMenu)},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestTapRightAltFiresImeOn(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkRMenu, 0), begin()},
		{up(win32.VkRMenu, 499), fire(win32.VkRMenu)},
	})
}

func TestHoldBoundary(t *testing.T) {
	// Exactly maxHoldMs still fires; one millisecond beyond does not.
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 1000), begin()},
		{up(win32.VkLMenu, 1500), fire(win32.VkLMenu)},
		{down(win32.VkLMenu, 2000), begin()},
		{up(win32.VkLMenu, 2501), endTap()},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle after expired tap", m.phase)
	}
}

func TestCanceledChordDoesNotRequestAltUpSuppressor(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{down(0x09 /*Tab*/, 20), none()},
		{up(0x09, 40), none()},
		{up(win32.VkLMenu, 60), none()},
	})
}

func TestTimeWraparound(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0xFFFFFF00), begin()},
		{up(win32.VkLMenu, 0x00000030), fire(win32.VkLMenu)}, // elapsed 0x130 = 304ms
	})
}

func TestModifierHeldBeforeAltCancels(t *testing.T) {
	for _, mod := range []uint32{0xA0 /*LShift*/, 0xA2 /*LCtrl*/, 0x5B /*LWin*/} {
		m := NewTapMachine(500)
		run(t, m, []step{
			{down(mod, 0), none()},
			{down(win32.VkLMenu, 10), none()}, // canceled: no beginTap, no suppressor
			{up(win32.VkLMenu, 50), none()},
			{up(mod, 60), none()},
			// A clean tap afterwards works again.
			{down(win32.VkLMenu, 100), begin()},
			{up(win32.VkLMenu, 150), fire(win32.VkLMenu)},
		})
	}
}

func TestChordCancels(t *testing.T) {
	// Alt+Tab style chord: another key pressed during tracking.
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{down(0x09 /*Tab*/, 20), none()},
		{up(0x09, 40), none()},
		{up(win32.VkLMenu, 60), none()}, // canceled tap never fires
		{down(win32.VkLMenu, 100), begin()},
		{up(win32.VkLMenu, 120), fire(win32.VkLMenu)},
	})
}

func TestBothAltsCancel(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{down(win32.VkRMenu, 10), none()}, // opposite Alt cancels
		{up(win32.VkRMenu, 20), none()},
		{up(win32.VkLMenu, 30), none()},
		{down(win32.VkRMenu, 100), begin()},
		{up(win32.VkRMenu, 130), fire(win32.VkRMenu)},
	})
}

func TestAltRepeatIsIgnored(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{down(win32.VkLMenu, 100), none()}, // auto-repeat: still tracking
		{down(win32.VkLMenu, 200), none()},
		{up(win32.VkLMenu, 300), fire(win32.VkLMenu)},
	})
}

func TestInjectedAltDoesNotStartTap(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{injDown(win32.VkLMenu, 0), none()}, // no BeginTap: injected Alt never tracks
		{injUp(win32.VkLMenu, 50), none()},
	})
	if m.phase != tapIdle {
		t.Fatalf("phase = %v, want idle", m.phase)
	}
}

func TestInjectedKeyCancelsTracking(t *testing.T) {
	// External tools' injected input reaches the target app, so it cancels.
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{injDown(0x41 /*A*/, 10), none()},
		{injUp(0x41, 20), none()},
		{up(win32.VkLMenu, 30), none()}, // canceled
	})
}

func TestInjectedTargetAltRepeatCancels(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{injDown(win32.VkLMenu, 10), none()}, // injected duplicate of the target
		{up(win32.VkLMenu, 30), none()},
	})
}

func TestInjectedTargetAltUpCancels(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{injUp(win32.VkLMenu, 10), none()}, // injected release: physical state unknown
		{up(win32.VkLMenu, 30), none()},    // canceled -> idle on the physical up
		{down(win32.VkLMenu, 100), begin()},
		{up(win32.VkLMenu, 130), fire(win32.VkLMenu)},
	})
}

func TestResyncHeldKeyBlocksTap(t *testing.T) {
	// Keys already down at startup/re-enable must cancel taps (resync).
	m := NewTapMachine(500)
	m.Resync([]uint32{0x41})
	run(t, m, []step{
		{down(win32.VkLMenu, 0), none()}, // canceled: 'A' held from before
		{up(win32.VkLMenu, 30), none()},
		{up(0x41, 40), none()},
		{down(win32.VkLMenu, 100), begin()},
		{up(win32.VkLMenu, 130), fire(win32.VkLMenu)},
	})
}

func TestResyncWithHeldAltThenRepeat(t *testing.T) {
	// Alt physically held across an enable: its repeats must not tap.
	m := NewTapMachine(500)
	m.Resync([]uint32{win32.VkLMenu})
	run(t, m, []step{
		{down(win32.VkLMenu, 0), none()}, // repeat of a press we never saw
		{up(win32.VkLMenu, 30), none()},
		{down(win32.VkLMenu, 100), begin()},
		{up(win32.VkLMenu, 130), fire(win32.VkLMenu)},
	})
}

func TestResyncClearsTracking(t *testing.T) {
	m := NewTapMachine(500)
	m.Feed(down(win32.VkLMenu, 0))
	m.Resync(nil)
	if m.phase != tapIdle || m.heldCount != 0 {
		t.Fatalf("resync did not reset: phase=%v held=%d", m.phase, m.heldCount)
	}
	// The Alt-up for the press consumed by resync must not fire.
	if got := m.Feed(up(win32.VkLMenu, 100)); got != none() {
		t.Fatalf("up after resync fired: %+v", got)
	}
}

func TestInvalidateCancelsTracking(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{{down(win32.VkLMenu, 0), begin()}})
	m.Invalidate()
	run(t, m, []step{
		{up(win32.VkLMenu, 30), none()},
		{down(win32.VkLMenu, 100), begin()},
		{up(win32.VkLMenu, 130), fire(win32.VkLMenu)},
	})
}

func TestInvalidateWhileIdleIsHarmless(t *testing.T) {
	m := NewTapMachine(500)
	m.Invalidate()
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{up(win32.VkLMenu, 30), fire(win32.VkLMenu)},
	})
}

func TestUnexpectedUpDuringTrackingCancels(t *testing.T) {
	// An up for a key we never saw go down means our view is stale.
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{up(0x41, 10), none()}, // 'A' up, but 'A' was not held
		{up(win32.VkLMenu, 30), none()},
	})
}

func TestOutOfRangeVKInvalidates(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{down(0x1FF, 10), none()},       // out of the documented 1..254 contract
		{up(win32.VkLMenu, 30), none()}, // canceled by the invalidation
	})
	if got := m.Feed(KeyEvent{VK: 0, Down: true, Time: 40}); got != none() {
		t.Fatalf("vk 0 produced action: %+v", got)
	}
}

func TestConsecutiveTaps(t *testing.T) {
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(win32.VkLMenu, 0), begin()},
		{up(win32.VkLMenu, 100), fire(win32.VkLMenu)},
		{down(win32.VkRMenu, 200), begin()},
		{up(win32.VkRMenu, 300), fire(win32.VkRMenu)},
		{down(win32.VkLMenu, 400), begin()},
		{up(win32.VkLMenu, 450), fire(win32.VkLMenu)},
	})
}

func TestKeyHeldAcrossTapBlocksUntilReleased(t *testing.T) {
	// Key stays down over two Alt presses; both are canceled.
	m := NewTapMachine(500)
	run(t, m, []step{
		{down(0x41, 0), none()},
		{down(win32.VkLMenu, 10), none()},
		{up(win32.VkLMenu, 20), none()},
		{down(win32.VkLMenu, 30), none()},
		{up(win32.VkLMenu, 40), none()},
		{up(0x41, 50), none()},
		{down(win32.VkLMenu, 60), begin()},
		{up(win32.VkLMenu, 70), fire(win32.VkLMenu)},
	})
}
