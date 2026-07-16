//go:build windows

package app

import "github.com/ishibashi-futos/alt-ime-go/internal/win32"

// Application messages. Controller-window messages first, then messages
// posted to the hook thread's message queue.
const (
	msgSwitch      = win32.WmApp + 1 // wParam: packed open+VK, lParam: target HWND
	msgTray        = win32.WmApp + 2 // tray callback (NOTIFYICON_VERSION_4 encoding)
	msgHookStopped = win32.WmApp + 3 // wParam: 1 when the hook loop died unexpectedly
	msgGuardEnter  = win32.WmApp + 4 // wParam: packed send+composing, lParam: target HWND

	msgHookDispatchSwitch = win32.WmApp + 16 // same payload as msgSwitch
	msgHookSetEnabled     = win32.WmApp + 17 // wParam: 0/1
	msgHookReset          = win32.WmApp + 18
	msgHookStop           = win32.WmApp + 19
	msgHookSetEnterGuard  = win32.WmApp + 20 // wParam: 0/1
	msgHookDispatchGuard  = win32.WmApp + 21 // same payload as msgGuardEnter
)

// appIconResourceID is the RT_GROUP_ICON id tools/mkrsrc embeds into the
// executable; the tray loads the same resource so exe and tray share one
// design.
const appIconResourceID = 1
