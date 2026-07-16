//go:build windows

package app

// Notification-area icon and its context menu (UI thread only). The icon
// uses NOTIFYICON_VERSION_4, answers mouse and keyboard interaction
// (WM_CONTEXTMENU / NIN_SELECT / NIN_KEYSELECT), and re-registers itself
// after Explorer restarts (TaskbarCreated).

import (
	"unsafe"

	"github.com/ishibashi-futos/alt-ime-go/internal/win32"
)

const (
	trayIconID = 1

	cmdToggleEnabled    = 1
	cmdExit             = 2
	cmdToggleEnterGuard = 3
)

type trayIcon struct {
	owner uintptr // controller window receiving msgTray
	menu  uintptr
	icon  uintptr // shared resource loaded from the executable; do not destroy
	added bool
}

func newTrayIcon(owner, hinst uintptr) (*trayIcon, error) {
	icon, errno := win32.LoadIcon(hinst, appIconResourceID)
	if icon == 0 {
		return nil, win32.WinError("LoadIconW(application)", errno)
	}
	menu := win32.CreatePopupMenu()
	if menu == 0 {
		return nil, win32.WinError("CreatePopupMenu", 0)
	}
	if !win32.AppendMenu(menu, win32.MfString, cmdToggleEnabled, "有効") ||
		!win32.AppendMenu(menu, win32.MfString, cmdToggleEnterGuard, "Enter送信ガード") ||
		!win32.AppendMenu(menu, win32.MfString, cmdExit, "終了") {
		win32.DestroyMenu(menu)
		return nil, win32.WinError("AppendMenuW", 0)
	}
	t := &trayIcon{owner: owner, menu: menu, icon: icon}
	if err := t.register(); err != nil {
		win32.DestroyMenu(menu)
		return nil, err
	}
	return t, nil
}

func (t *trayIcon) baseNID() win32.NotifyIconDataW {
	return win32.NotifyIconDataW{
		CbSize: uint32(unsafe.Sizeof(win32.NotifyIconDataW{})),
		HWnd:   t.owner,
		UID:    trayIconID,
	}
}

func (t *trayIcon) fullNID() win32.NotifyIconDataW {
	nid := t.baseNID()
	nid.UFlags = win32.NifMessage | win32.NifIcon | win32.NifTip | win32.NifShowTip
	nid.UCallbackMessage = msgTray
	nid.HIcon = t.icon
	win32.CopyUTF16(nid.SzTip[:], appTitle)
	return nid
}

// register performs NIM_ADD and then always NIM_SETVERSION(4); without the
// version the callback encoding would differ from what onTrayEvent expects.
func (t *trayIcon) register() error {
	nid := t.fullNID()
	if !win32.ShellNotifyIcon(win32.NimAdd, &nid) {
		return win32.WinError("Shell_NotifyIconW(NIM_ADD)", 0)
	}
	t.added = true
	ver := t.baseNID()
	ver.UVersion = win32.NotifyIconVersion4
	if !win32.ShellNotifyIcon(win32.NimSetVersion, &ver) {
		t.remove()
		return win32.WinError("Shell_NotifyIconW(NIM_SETVERSION)", 0)
	}
	return nil
}

// reRegister restores the icon after a TaskbarCreated broadcast (Explorer
// restart). If the icon somehow survived, fall back to NIM_MODIFY. Failure
// here is logged, not fatal: Explorer may broadcast again.
func (t *trayIcon) reRegister() {
	t.added = false
	if err := t.register(); err == nil {
		return
	}
	nid := t.fullNID()
	if win32.ShellNotifyIcon(win32.NimModify, &nid) {
		t.added = true
		ver := t.baseNID()
		win32.ShellNotifyIcon(win32.NimSetVersion, &ver)
		return
	}
	win32.Debugf("tray: re-registration after TaskbarCreated failed")
}

// showMenu runs the context menu at the anchor the shell provided and
// returns the chosen command (0 = dismissed). TPM_RETURNCMD keeps the
// dispatch here instead of WM_COMMAND round-trips.
func (t *trayIcon) showMenu(x, y int32, enabled, guardEnabled bool) int {
	win32.CheckMenuItem(t.menu, cmdToggleEnabled, checkFlags(enabled))
	win32.CheckMenuItem(t.menu, cmdToggleEnterGuard, checkFlags(guardEnabled))
	// Required foreground dance: without it the menu will not dismiss when
	// the user clicks elsewhere, and WM_NULL afterwards works around the
	// matching quirk (KB135788).
	win32.SetForegroundWindow(t.owner)
	cmd := win32.TrackPopupMenuEx(t.menu, win32.TpmReturnCmd|win32.TpmNoNotify|win32.TpmRightButton, x, y, t.owner)
	win32.PostMessage(t.owner, win32.WmNull, 0, 0)
	// Hand keyboard focus back to the notification area (design §7).
	nid := t.baseNID()
	win32.ShellNotifyIcon(win32.NimSetFocus, &nid)
	return int(cmd)
}

func checkFlags(checked bool) uint32 {
	if checked {
		return win32.MfByCommand | win32.MfChecked
	}
	return win32.MfByCommand | win32.MfUnchecked
}

func (t *trayIcon) remove() {
	if !t.added {
		return
	}
	nid := t.baseNID()
	if !win32.ShellNotifyIcon(win32.NimDelete, &nid) {
		win32.Debugf("Shell_NotifyIconW(NIM_DELETE) failed")
	}
	t.added = false
}

func (t *trayIcon) destroy() {
	t.remove()
	if t.menu != 0 {
		win32.DestroyMenu(t.menu)
		t.menu = 0
	}
}
