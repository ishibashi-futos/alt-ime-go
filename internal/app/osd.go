//go:build windows

package app

// macOS-style on-screen indicator (UI thread only). The window is layered,
// click-through, non-activating, topmost, and hidden from the taskbar
// (NFR-8). It shows the *delivery result* of a switch request — "A"/"あ"
// only after a successful insert, "!" on failure — never a verified IME
// state (CON-5).

import (
	"syscall"

	"github.com/ishibashi-futos/alt-ime-go/internal/config"
	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

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
	metrics config.OsdMetrics
	kind    osdKind
	// gen invalidates WM_TIMER messages left in the queue by a superseded
	// display; KillTimer does not purge already-posted messages.
	gen      uint32
	alpha    int32
	throttle config.FailThrottle

	textOff, textOn, textFail *uint16
}

var osdWndProcCB = syscall.NewCallback(osdWndProc)

func newOsdWindow(hinst uintptr) (*osdWindow, error) {
	if err := win32.RegisterClass(osdClassName, osdWndProcCB, hinst); err != nil {
		return nil, err
	}
	hwnd, errno := win32.CreateWindow(
		win32.WsExLayered|win32.WsExTransparent|win32.WsExToolWindow|win32.WsExTopmost|win32.WsExNoActivate,
		osdClassName, "", win32.WsPopup, 0, 0, 1, 1, 0, hinst,
	)
	if hwnd == 0 {
		return nil, win32.WinError("CreateWindowExW(OSD)", errno)
	}
	brush := win32.CreateSolidBrush(config.OsdColorBack)
	if brush == 0 {
		win32.DestroyWindow(hwnd)
		return nil, win32.WinError("CreateSolidBrush(OSD)", 0)
	}
	return &osdWindow{
		hwnd:     hwnd,
		bgBrush:  brush,
		textOff:  win32.MustUTF16("A"),
		textOn:   win32.MustUTF16("あ"),
		textFail: win32.MustUTF16("!"),
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
		if !o.throttle.Allow(win32.GetTickCount64()) {
			return
		}
	} else {
		o.throttle.Reset()
	}
	mon := win32.MonitorFromWindow(target, win32.MonitorDefaultToNearest)
	var mi win32.MonitorInfo
	if mon == 0 || !win32.GetMonitorInfo(mon, &mi) {
		win32.Debugf("OSD: monitor lookup failed for target %#x", target)
		return
	}
	dpi, ok := win32.GetDpiForMonitor(mon)
	if !ok || dpi == 0 {
		if dpi = win32.GetDpiForWindow(o.hwnd); dpi == 0 {
			dpi = 96
		}
	}
	if dpi != o.dpi {
		o.applyDPI(dpi)
	}
	o.kind = kind
	m := o.metrics
	x := mi.RcWork.Left + (mi.RcWork.Width()-m.BoxW)/2
	y := mi.RcWork.Bottom - m.MarginBottom - m.BoxH

	// Supersede any previous display: bump the generation, stop its timers,
	// and restore full opacity before (re)showing.
	win32.KillTimer(o.hwnd, config.OsdTimerID(o.gen, false))
	win32.KillTimer(o.hwnd, config.OsdTimerID(o.gen, true))
	o.gen++
	o.alpha = config.OsdAlpha
	win32.SetLayeredWindowAttributes(o.hwnd, 0, byte(o.alpha), win32.LwaAlpha)
	win32.SetWindowPos(o.hwnd, win32.HwndTopmost, x, y, m.BoxW, m.BoxH, win32.SwpNoActivate)
	o.applyRegion()
	win32.InvalidateRect(o.hwnd)
	win32.ShowWindow(o.hwnd, win32.SwShowNoActivate)
	if !win32.SetTimer(o.hwnd, config.OsdTimerID(o.gen, false), config.OsdHoldMs) {
		// Without the hold timer the OSD would stay forever; hide it instead.
		win32.Debugf("OSD: SetTimer failed; hiding immediately")
		win32.ShowWindow(o.hwnd, win32.SwHide)
	}
}

// applyDPI rebuilds the DPI-dependent resources (font, metrics). The old
// font is kept if creation fails so an existing display stays usable.
func (o *osdWindow) applyDPI(dpi uint32) {
	metrics := config.OsdBase.Scaled(dpi)
	font := win32.CreateFont(config.OsdFontFace, metrics.FontHeight)
	if font == 0 {
		win32.Debugf("OSD: CreateFontW failed for dpi %d", dpi)
		return
	}
	if o.font != 0 {
		win32.DeleteObject(o.font)
	}
	o.font = font
	o.metrics = metrics
	o.dpi = dpi
}

// applyRegion clips the window to a rounded rectangle. On success the
// system owns the region; it is deleted only when SetWindowRgn fails.
func (o *osdWindow) applyRegion() {
	m := o.metrics
	rgn := win32.CreateRoundRectRgn(0, 0, m.BoxW+1, m.BoxH+1, m.Corner*2, m.Corner*2)
	if rgn == 0 {
		win32.Debugf("OSD: CreateRoundRectRgn failed")
		return
	}
	if !win32.SetWindowRgn(o.hwnd, rgn, true) {
		win32.DeleteObject(rgn)
	}
}

func (o *osdWindow) onTimer(id uintptr) {
	holdID := config.OsdTimerID(o.gen, false)
	fadeID := config.OsdTimerID(o.gen, true)
	if id != holdID && id != fadeID {
		// Stale WM_TIMER from a superseded display generation.
		win32.KillTimer(o.hwnd, id)
		return
	}
	if id == holdID {
		win32.KillTimer(o.hwnd, holdID)
		if !win32.SetTimer(o.hwnd, fadeID, config.OsdFadeIntervalMs) {
			win32.ShowWindow(o.hwnd, win32.SwHide)
		}
		return
	}
	o.alpha -= config.OsdFadeAlphaStep
	if o.alpha <= 0 {
		win32.KillTimer(o.hwnd, fadeID)
		win32.ShowWindow(o.hwnd, win32.SwHide)
		return
	}
	win32.SetLayeredWindowAttributes(o.hwnd, 0, byte(o.alpha), win32.LwaAlpha)
}

// onDpiChanged rebuilds font/region and re-centers within the current
// monitor's work area. The new DPI arrives in LOWORD(wParam); the suggested
// rect is ignored because the OSD derives its own geometry.
func (o *osdWindow) onDpiChanged(dpi uint32) {
	if dpi == 0 || dpi == o.dpi {
		return
	}
	o.applyDPI(dpi)
	mon := win32.MonitorFromWindow(o.hwnd, win32.MonitorDefaultToNearest)
	var mi win32.MonitorInfo
	if mon != 0 && win32.GetMonitorInfo(mon, &mi) {
		m := o.metrics
		x := mi.RcWork.Left + (mi.RcWork.Width()-m.BoxW)/2
		y := mi.RcWork.Bottom - m.MarginBottom - m.BoxH
		win32.SetWindowPos(o.hwnd, win32.HwndTopmost, x, y, m.BoxW, m.BoxH, win32.SwpNoActivate)
	}
	o.applyRegion()
	win32.InvalidateRect(o.hwnd)
}

func (o *osdWindow) paint() {
	var ps win32.PaintStruct
	hdc := win32.BeginPaint(o.hwnd, &ps)
	if hdc == 0 {
		win32.ValidateRect(o.hwnd) // avoid an endless WM_PAINT stream
		return
	}
	full := win32.Rect{Right: o.metrics.BoxW, Bottom: o.metrics.BoxH}
	win32.FillRect(hdc, &full, o.bgBrush)
	if o.font != 0 {
		prev := win32.SelectObject(hdc, o.font)
		win32.SetBkMode(hdc, win32.BkModeTransparent)
		color := uint32(config.OsdColorText)
		if o.kind == osdFail {
			color = config.OsdColorFail
		}
		win32.SetTextColor(hdc, color)
		win32.DrawText(hdc, o.text(), &full, win32.DtCenter|win32.DtVCenter|win32.DtSingleLine)
		win32.SelectObject(hdc, prev)
	}
	win32.EndPaint(o.hwnd, &ps)
}

// destroy releases everything in reverse order of acquisition: timers, the
// window, then the GDI objects that are no longer referenced by it.
func (o *osdWindow) destroy() {
	win32.KillTimer(o.hwnd, config.OsdTimerID(o.gen, false))
	win32.KillTimer(o.hwnd, config.OsdTimerID(o.gen, true))
	if o.hwnd != 0 {
		win32.DestroyWindow(o.hwnd)
		o.hwnd = 0
	}
	if o.font != 0 {
		win32.DeleteObject(o.font)
		o.font = 0
	}
	if o.bgBrush != 0 {
		win32.DeleteObject(o.bgBrush)
		o.bgBrush = 0
	}
}

func osdWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	a := ui
	if a == nil || a.osd == nil || a.osd.hwnd != hwnd {
		return win32.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	o := a.osd
	switch uint32(msg) {
	case win32.WmPaint:
		o.paint()
		return 0
	case win32.WmTimer:
		o.onTimer(wParam)
		return 0
	case win32.WmDpiChanged:
		o.onDpiChanged(uint32(wParam & 0xFFFF))
		return 0
	}
	return win32.DefWindowProc(hwnd, msg, wParam, lParam)
}
