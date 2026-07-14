package main

// Compile-time configuration (FR-13/14/15) and the pure helpers built on it.
// Everything here is Win32-free so it stays unit-testable on any host.

// imeControlMethod selects how IME switch requests are delivered (FR-14).
// Exactly one path runs; there is no automatic fallback from one to the
// other because a VK event that the IME silently ignored cannot be detected
// synchronously and doubling up causes its own side effects.
type imeControlMethod int

const (
	imeControlVK imeControlMethod = iota
	imeControlIMM32
)

const (
	// tapMaxHoldMs is the longest press still counted as a tap. 0 (no limit)
	// is intentionally not supported.
	tapMaxHoldMs uint32 = 500

	// suppressAltMenuFocus injects tagged VK 0x07 on a clean physical Alt
	// down, then VK_F24 on its clean Alt-up. The first masks Win32 menus; the
	// second is visible to Electron/Chromium and web keyboard handlers so
	// they observe an Alt+F24 chord. Set false if Alt+F24 conflicts.
	suppressAltMenuFocus = true

	// imeControl selects the delivery path (FR-14).
	imeControl = imeControlVK

	// imm32TimeoutMs bounds the synchronous WM_IME_CONTROL delivery on the
	// IMM32 path. Plain SendMessage (no deadline) is never used.
	imm32TimeoutMs uint32 = 100

	// switchRetryDeadlineMs / switchRetryIntervalMs: when a switch request
	// arrives while the trigger Alt is still reported down, the UI re-checks
	// on a timer for at most this long before discarding the request.
	switchRetryDeadlineMs uint64 = 50
	switchRetryIntervalMs uint32 = 10

	// shutdownFallbackMs: how long the UI waits for the hook thread to
	// confirm msgHookStop before releasing resources anyway.
	shutdownFallbackMs uint32 = 2000

	// failOsdSuppressMs suppresses back-to-back failure OSDs.
	failOsdSuppressMs uint64 = 1500

	// measureHookLatency enables per-event duration tracking inside the hook
	// callback (two QueryPerformanceCounter reads, no I/O). The maximum is
	// reported via OutputDebugString when the hook stops. Debug builds only.
	measureHookLatency = false
)

// ---- OSD tunables, defined at 96 DPI and scaled to the target monitor ----

type osdMetrics struct {
	boxW         int32
	boxH         int32
	corner       int32 // rounded-corner radius
	marginBottom int32 // gap between OSD bottom and the work-area bottom
	fontHeight   int32 // character height passed to CreateFontW (negated)
}

var osdBase = osdMetrics{
	boxW:         110,
	boxH:         110,
	corner:       22,
	marginBottom: 120,
	fontHeight:   60,
}

const (
	osdAlpha          = 224 // initial layered-window alpha
	osdHoldMs         = 650 // fully visible duration before the fade starts
	osdFadeIntervalMs = 30
	osdFadeAlphaStep  = 16

	osdFontFace = "Yu Gothic UI"

	// COLORREF values (0x00BBGGRR).
	osdColorBack = 0x00202020
	osdColorText = 0x00F5F5F5
	osdColorFail = 0x003C46EB // red-ish for the "!" failure indicator
)

// scaleDPI converts a 96-DPI base value to the target DPI, rounding half up.
func scaleDPI(v int32, dpi uint32) int32 {
	return int32((int64(v)*int64(dpi) + 48) / 96)
}

func (b osdMetrics) scaled(dpi uint32) osdMetrics {
	return osdMetrics{
		boxW:         scaleDPI(b.boxW, dpi),
		boxH:         scaleDPI(b.boxH, dpi),
		corner:       scaleDPI(b.corner, dpi),
		marginBottom: scaleDPI(b.marginBottom, dpi),
		fontHeight:   scaleDPI(b.fontHeight, dpi),
	}
}

// ---- OSD timer generations ----

// osdTimerID encodes the display generation into the timer ID so a WM_TIMER
// left in the queue by a superseded display (KillTimer does not purge queued
// messages) can be recognized and discarded. Generations start at 1, so IDs
// are never 0.
func osdTimerID(gen uint32, fade bool) uintptr {
	id := uintptr(gen) << 1
	if fade {
		id |= 1
	}
	return id
}

// ---- failure OSD throttle ----

// failThrottle suppresses repeat failure notifications inside a fixed window
// measured from the last failure actually shown.
type failThrottle struct {
	shown  bool
	lastMs uint64
}

// reset re-arms the throttle; called when a switch succeeds so the next
// failure is reported immediately.
func (t *failThrottle) reset() {
	t.shown = false
}

func (t *failThrottle) allow(nowMs uint64) bool {
	if t.shown && nowMs-t.lastMs < failOsdSuppressMs {
		return false
	}
	t.shown = true
	t.lastMs = nowMs
	return true
}

// ---- switch request packing (hook thread -> UI window message) ----

// The two-stage dispatch carries only plain integers through WPARAM/LPARAM
// (never Go pointers): WPARAM packs the direction and the trigger VK, LPARAM
// carries the target HWND.

const switchOpenBit = 0x100

func packSwitchWParam(open bool, vk uint32) uintptr {
	wp := uintptr(vk & 0xFF)
	if open {
		wp |= switchOpenBit
	}
	return wp
}

func unpackSwitchWParam(wp uintptr) (open bool, vk uint32) {
	return wp&switchOpenBit != 0, uint32(wp & 0xFF)
}
