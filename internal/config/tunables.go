package config

// Compile-time configuration (FR-13/14/15) and the pure helpers built on it.
// Everything here is Win32-free so it stays unit-testable on any host.

import "strings"

// ImeControlMethod selects how IME switch requests are delivered (FR-14).
// Exactly one path runs; there is no automatic fallback from one to the
// other because a VK event that the IME silently ignored cannot be detected
// synchronously and doubling up causes its own side effects.
type ImeControlMethod int

const (
	ImeControlVK ImeControlMethod = iota
	ImeControlIMM32
)

const (
	// TapMaxHoldMs is the longest press still counted as a tap. 0 (no limit)
	// is intentionally not supported.
	TapMaxHoldMs uint32 = 500

	// SuppressAltMenuFocus injects tagged VK 0x07 on a clean physical Alt
	// down, then VK_F24 on its clean Alt-up. The first masks Win32 menus; the
	// second is visible to Electron/Chromium and web keyboard handlers so
	// they observe an Alt+F24 chord. Set false if Alt+F24 conflicts.
	SuppressAltMenuFocus = true

	// ImeControl selects the delivery path (FR-14).
	ImeControl = ImeControlVK

	// Imm32TimeoutMs bounds the synchronous WM_IME_CONTROL delivery on the
	// IMM32 path. Plain SendMessage (no deadline) is never used.
	Imm32TimeoutMs uint32 = 100

	// SwitchRetryDeadlineMs / SwitchRetryIntervalMs: when a switch request
	// arrives while the trigger Alt is still reported down, the UI re-checks
	// on a timer for at most this long before discarding the request.
	SwitchRetryDeadlineMs uint64 = 50
	SwitchRetryIntervalMs uint32 = 10

	// ShutdownFallbackMs: how long the UI waits for the hook thread to
	// confirm msgHookStop before releasing resources anyway.
	ShutdownFallbackMs uint32 = 2000

	// FailOsdSuppressMs suppresses back-to-back failure OSDs.
	FailOsdSuppressMs uint64 = 1500

	// MeasureHookLatency enables per-event duration tracking inside the hook
	// callback (two QueryPerformanceCounter reads, no I/O). The maximum is
	// reported via OutputDebugString when the hook stops. Debug builds only.
	MeasureHookLatency = false

	// EnterGuardDefaultEnabled is the Enter guard state at startup; the tray
	// menu toggles it at runtime (FR-20..23).
	EnterGuardDefaultEnabled = true

	// GuardTrace logs every guard replacement decision (send/composing/IME
	// answer) to OutputDebugString from the UI thread. Kept on while the
	// CON-9 heuristic is under real-hardware validation; set false once the
	// feature is signed off.
	GuardTrace = true
)

// ---- Enter guard targets (FR-20) ----

// enterGuardTargetExes lists the executable basenames (lowercase) whose
// foreground windows get the Enter guard: plain Enter becomes Shift+Enter
// (newline) and Ctrl+Enter becomes a plain Enter (send).
var enterGuardTargetExes = []string{
	"m365copilot.exe",
	"claude.exe",
}

// MatchGuardTarget reports whether a full process image path names one of
// the guard targets. Windows paths are compared by backslash-delimited
// basename, case-insensitively.
func MatchGuardTarget(imagePath string) bool {
	if imagePath == "" {
		return false
	}
	base := imagePath
	if i := strings.LastIndexByte(base, '\\'); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	for _, exe := range enterGuardTargetExes {
		if base == exe {
			return true
		}
	}
	return false
}

// ---- OSD tunables, defined at 96 DPI and scaled to the target monitor ----

type OsdMetrics struct {
	BoxW         int32
	BoxH         int32
	Corner       int32 // rounded-corner radius
	MarginBottom int32 // gap between OSD bottom and the work-area bottom
	FontHeight   int32 // character height passed to CreateFontW (negated)
}

var OsdBase = OsdMetrics{
	BoxW:         110,
	BoxH:         110,
	Corner:       22,
	MarginBottom: 120,
	FontHeight:   60,
}

const (
	OsdAlpha          = 224 // initial layered-window alpha
	OsdHoldMs         = 650 // fully visible duration before the fade starts
	OsdFadeIntervalMs = 30
	OsdFadeAlphaStep  = 16

	OsdFontFace = "Yu Gothic UI"

	// COLORREF values (0x00BBGGRR).
	OsdColorBack = 0x00202020
	OsdColorText = 0x00F5F5F5
	OsdColorFail = 0x003C46EB // red-ish for the "!" failure indicator
)

// ScaleDPI converts a 96-DPI base value to the target DPI, rounding half up.
func ScaleDPI(v int32, dpi uint32) int32 {
	return int32((int64(v)*int64(dpi) + 48) / 96)
}

func (b OsdMetrics) Scaled(dpi uint32) OsdMetrics {
	return OsdMetrics{
		BoxW:         ScaleDPI(b.BoxW, dpi),
		BoxH:         ScaleDPI(b.BoxH, dpi),
		Corner:       ScaleDPI(b.Corner, dpi),
		MarginBottom: ScaleDPI(b.MarginBottom, dpi),
		FontHeight:   ScaleDPI(b.FontHeight, dpi),
	}
}

// ---- OSD timer generations ----

// OsdTimerID encodes the display generation into the timer ID so a WM_TIMER
// left in the queue by a superseded display (KillTimer does not purge queued
// messages) can be recognized and discarded. Generations start at 1, so IDs
// are never 0.
func OsdTimerID(gen uint32, fade bool) uintptr {
	id := uintptr(gen) << 1
	if fade {
		id |= 1
	}
	return id
}

// ---- failure OSD throttle ----

// FailThrottle suppresses repeat failure notifications inside a fixed window
// measured from the last failure actually shown.
type FailThrottle struct {
	shown  bool
	lastMs uint64
}

// Reset re-arms the throttle; called when a switch succeeds so the next
// failure is reported immediately.
func (t *FailThrottle) Reset() {
	t.shown = false
}

func (t *FailThrottle) Allow(nowMs uint64) bool {
	if t.shown && nowMs-t.lastMs < FailOsdSuppressMs {
		return false
	}
	t.shown = true
	t.lastMs = nowMs
	return true
}
