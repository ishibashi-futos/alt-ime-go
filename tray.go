//go:build windows

package main

// Notification-area icon and its context menu (UI thread only). The icon
// uses NOTIFYICON_VERSION_4, answers mouse and keyboard interaction
// (WM_CONTEXTMENU / NIN_SELECT / NIN_KEYSELECT), and re-registers itself
// after Explorer restarts (TaskbarCreated).

import "unsafe"

const (
	trayIconID = 1

	cmdToggleEnabled = 1
	cmdExit          = 2
)

type trayIcon struct {
	owner uintptr // controller window receiving msgTray
	menu  uintptr
	added bool
}

func newTrayIcon(owner uintptr) (*trayIcon, error) {
	menu := createPopupMenu()
	if menu == 0 {
		return nil, winError("CreatePopupMenu", 0)
	}
	if !appendMenu(menu, mfString, cmdToggleEnabled, "有効") ||
		!appendMenu(menu, mfString, cmdExit, "終了") {
		destroyMenu(menu)
		return nil, winError("AppendMenuW", 0)
	}
	t := &trayIcon{owner: owner, menu: menu}
	if err := t.register(); err != nil {
		destroyMenu(menu)
		return nil, err
	}
	return t, nil
}

func (t *trayIcon) baseNID() notifyIconDataW {
	return notifyIconDataW{
		cbSize: uint32(unsafe.Sizeof(notifyIconDataW{})),
		hWnd:   t.owner,
		uID:    trayIconID,
	}
}

func (t *trayIcon) fullNID() notifyIconDataW {
	nid := t.baseNID()
	nid.uFlags = nifMessage | nifIcon | nifTip | nifShowTip
	nid.uCallbackMessage = msgTray
	// Shared system icon: must never be destroyed by us.
	nid.hIcon = loadIcon(0, idiApplication)
	copyUTF16(nid.szTip[:], appTitle)
	return nid
}

// register performs NIM_ADD and then always NIM_SETVERSION(4); without the
// version the callback encoding would differ from what onTrayEvent expects.
func (t *trayIcon) register() error {
	nid := t.fullNID()
	if !shellNotifyIcon(nimAdd, &nid) {
		return winError("Shell_NotifyIconW(NIM_ADD)", 0)
	}
	t.added = true
	ver := t.baseNID()
	ver.uVersion = notifyIconVersion4
	if !shellNotifyIcon(nimSetVersion, &ver) {
		t.remove()
		return winError("Shell_NotifyIconW(NIM_SETVERSION)", 0)
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
	if shellNotifyIcon(nimModify, &nid) {
		t.added = true
		ver := t.baseNID()
		shellNotifyIcon(nimSetVersion, &ver)
		return
	}
	debugf("tray: re-registration after TaskbarCreated failed")
}

// showMenu runs the context menu at the anchor the shell provided and
// returns the chosen command (0 = dismissed). TPM_RETURNCMD keeps the
// dispatch here instead of WM_COMMAND round-trips.
func (t *trayIcon) showMenu(x, y int32, enabled bool) int {
	check := uint32(mfByCommand | mfUnchecked)
	if enabled {
		check = mfByCommand | mfChecked
	}
	checkMenuItem(t.menu, cmdToggleEnabled, check)
	// Required foreground dance: without it the menu will not dismiss when
	// the user clicks elsewhere, and WM_NULL afterwards works around the
	// matching quirk (KB135788).
	setForegroundWindow(t.owner)
	cmd := trackPopupMenuEx(t.menu, tpmReturnCmd|tpmNoNotify|tpmRightButton, x, y, t.owner)
	postMessage(t.owner, wmNull, 0, 0)
	// Hand keyboard focus back to the notification area (design §7).
	nid := t.baseNID()
	shellNotifyIcon(nimSetFocus, &nid)
	return int(cmd)
}

func (t *trayIcon) remove() {
	if !t.added {
		return
	}
	nid := t.baseNID()
	if !shellNotifyIcon(nimDelete, &nid) {
		debugf("Shell_NotifyIconW(NIM_DELETE) failed")
	}
	t.added = false
}

func (t *trayIcon) destroy() {
	t.remove()
	if t.menu != 0 {
		destroyMenu(t.menu)
		t.menu = 0
	}
}
