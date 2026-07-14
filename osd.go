//go:build windows

package main

// macOS-style on-screen indicator (UI thread only). The window is layered,
// click-through, non-activating, topmost, and hidden from the taskbar
// (NFR-8). It shows the *delivery result* of a switch request — "A"/"あ"
// only after a successful insert, "!" on failure — never a verified IME
// state (CON-5).

import "syscall"

type osdKind int

const (
	osdOff osdKind = iota
	osdOn
	osdFail
)

type osdWindow struct {
	hwnd    uintptr
	font    uintptr // HFONT for the current DPI; owned here
	bgBrush uintptr // background brush; owned here
	dpi     uint32
	metrics osdMetrics
	kind    osdKind
	// gen invalidates WM_TIMER messages left in the queue by a superseded
	// display; KillTimer does not purge already-posted messages.
	gen      uint32
	alpha    int32
	throttle failThrottle

	textOff, textOn, textFail *uint16
}

var osdWndProcCB = syscall.NewCallback(osdWndProc)

func newOsdWindow(hinst uintptr) (*osdWindow, error) {
	if err := registerClass(osdClassName, osdWndProcCB, hinst); err != nil {
		return nil, err
	}
	hwnd, errno := createWindow(
		wsExLayered|wsExTransparent|wsExToolWindow|wsExTopmost|wsExNoActivate,
		osdClassName, "", wsPopup, 0, 0, 1, 1, 0, hinst,
	)
	if hwnd == 0 {
		return nil, winError("CreateWindowExW(OSD)", errno)
	}
	brush := createSolidBrush(osdColorBack)
	if brush == 0 {
		destroyWindow(hwnd)
		return nil, winError("CreateSolidBrush(OSD)", 0)
	}
	return &osdWindow{
		hwnd:     hwnd,
		bgBrush:  brush,
		textOff:  mustUTF16("A"),
		textOn:   mustUTF16("あ"),
		textFail: mustUTF16("!"),
	}, nil
}

func (o *osdWindow) text() *uint16 {
	switch o.kind {
	case osdOn:
		return o.textOn
	case osdFail:
		return o.textFail
	}
	return o.textOff
}

// show displays the indicator on the monitor of the switch target window.
func (o *osdWindow) show(kind osdKind, target uintptr) {
	if kind == osdFail {
		if !o.throttle.allow(getTickCount64()) {
			return
		}
	} else {
		o.throttle.reset()
	}
	mon := monitorFromWindow(target, monitorDefaultToNearest)
	var mi monitorInfo
	if mon == 0 || !getMonitorInfo(mon, &mi) {
		debugf("OSD: monitor lookup failed for target %#x", target)
		return
	}
	dpi, ok := getDpiForMonitor(mon)
	if !ok || dpi == 0 {
		if dpi = getDpiForWindow(o.hwnd); dpi == 0 {
			dpi = 96
		}
	}
	if dpi != o.dpi {
		o.applyDPI(dpi)
	}
	o.kind = kind
	m := o.metrics
	x := mi.rcWork.left + (mi.rcWork.width()-m.boxW)/2
	y := mi.rcWork.bottom - m.marginBottom - m.boxH

	// Supersede any previous display: bump the generation, stop its timers,
	// and restore full opacity before (re)showing.
	killTimer(o.hwnd, osdTimerID(o.gen, false))
	killTimer(o.hwnd, osdTimerID(o.gen, true))
	o.gen++
	o.alpha = osdAlpha
	setLayeredWindowAttributes(o.hwnd, 0, byte(o.alpha), lwaAlpha)
	setWindowPos(o.hwnd, hwndTopmost, x, y, m.boxW, m.boxH, swpNoActivate)
	o.applyRegion()
	invalidateRect(o.hwnd)
	showWindow(o.hwnd, swShowNoActivate)
	if !setTimer(o.hwnd, osdTimerID(o.gen, false), osdHoldMs) {
		// Without the hold timer the OSD would stay forever; hide it instead.
		debugf("OSD: SetTimer failed; hiding immediately")
		showWindow(o.hwnd, swHide)
	}
}

// applyDPI rebuilds the DPI-dependent resources (font, metrics). The old
// font is kept if creation fails so an existing display stays usable.
func (o *osdWindow) applyDPI(dpi uint32) {
	metrics := osdBase.scaled(dpi)
	font := createOsdFont(metrics.fontHeight)
	if font == 0 {
		debugf("OSD: CreateFontW failed for dpi %d", dpi)
		return
	}
	if o.font != 0 {
		deleteObject(o.font)
	}
	o.font = font
	o.metrics = metrics
	o.dpi = dpi
}

// applyRegion clips the window to a rounded rectangle. On success the
// system owns the region; it is deleted only when SetWindowRgn fails.
func (o *osdWindow) applyRegion() {
	m := o.metrics
	rgn := createRoundRectRgn(0, 0, m.boxW+1, m.boxH+1, m.corner*2, m.corner*2)
	if rgn == 0 {
		debugf("OSD: CreateRoundRectRgn failed")
		return
	}
	if !setWindowRgn(o.hwnd, rgn, true) {
		deleteObject(rgn)
	}
}

func (o *osdWindow) onTimer(id uintptr) {
	holdID := osdTimerID(o.gen, false)
	fadeID := osdTimerID(o.gen, true)
	if id != holdID && id != fadeID {
		// Stale WM_TIMER from a superseded display generation.
		killTimer(o.hwnd, id)
		return
	}
	if id == holdID {
		killTimer(o.hwnd, holdID)
		if !setTimer(o.hwnd, fadeID, osdFadeIntervalMs) {
			showWindow(o.hwnd, swHide)
		}
		return
	}
	o.alpha -= osdFadeAlphaStep
	if o.alpha <= 0 {
		killTimer(o.hwnd, fadeID)
		showWindow(o.hwnd, swHide)
		return
	}
	setLayeredWindowAttributes(o.hwnd, 0, byte(o.alpha), lwaAlpha)
}

// onDpiChanged rebuilds font/region and re-centers within the current
// monitor's work area. The new DPI arrives in LOWORD(wParam); the suggested
// rect is ignored because the OSD derives its own geometry.
func (o *osdWindow) onDpiChanged(dpi uint32) {
	if dpi == 0 || dpi == o.dpi {
		return
	}
	o.applyDPI(dpi)
	mon := monitorFromWindow(o.hwnd, monitorDefaultToNearest)
	var mi monitorInfo
	if mon != 0 && getMonitorInfo(mon, &mi) {
		m := o.metrics
		x := mi.rcWork.left + (mi.rcWork.width()-m.boxW)/2
		y := mi.rcWork.bottom - m.marginBottom - m.boxH
		setWindowPos(o.hwnd, hwndTopmost, x, y, m.boxW, m.boxH, swpNoActivate)
	}
	o.applyRegion()
	invalidateRect(o.hwnd)
}

func (o *osdWindow) paint() {
	var ps paintStruct
	hdc := beginPaint(o.hwnd, &ps)
	if hdc == 0 {
		validateRect(o.hwnd) // avoid an endless WM_PAINT stream
		return
	}
	full := rect{0, 0, o.metrics.boxW, o.metrics.boxH}
	fillRect(hdc, &full, o.bgBrush)
	if o.font != 0 {
		prev := selectObject(hdc, o.font)
		setBkMode(hdc, bkModeTransparent)
		color := uint32(osdColorText)
		if o.kind == osdFail {
			color = osdColorFail
		}
		setTextColor(hdc, color)
		drawText(hdc, o.text(), &full, dtCenter|dtVCenter|dtSingleLine)
		selectObject(hdc, prev)
	}
	endPaint(o.hwnd, &ps)
}

// destroy releases everything in reverse order of acquisition: timers, the
// window, then the GDI objects that are no longer referenced by it.
func (o *osdWindow) destroy() {
	killTimer(o.hwnd, osdTimerID(o.gen, false))
	killTimer(o.hwnd, osdTimerID(o.gen, true))
	if o.hwnd != 0 {
		destroyWindow(o.hwnd)
		o.hwnd = 0
	}
	if o.font != 0 {
		deleteObject(o.font)
		o.font = 0
	}
	if o.bgBrush != 0 {
		deleteObject(o.bgBrush)
		o.bgBrush = 0
	}
}

func osdWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := ui
	if a == nil || a.osd == nil || a.osd.hwnd != hwnd {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
	o := a.osd
	switch uint32(msg) {
	case wmPaint:
		o.paint()
		return 0
	case wmTimer:
		o.onTimer(wParam)
		return 0
	case wmDpiChanged:
		o.onDpiChanged(uint32(wParam & 0xFFFF))
		return 0
	}
	return defWindowProc(hwnd, msg, wParam, lParam)
}
