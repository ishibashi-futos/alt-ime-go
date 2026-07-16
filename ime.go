//go:build windows

package main

// IME switch delivery (UI thread only). setIME reports whether the switch
// request was successfully handed to the system — "inserted", not "the IME
// really changed state". The OSD layer must show the normal indicator only
// for true results and must never fake success (design decision 6).

func setIME(open bool, target uintptr) bool {
	if imeControl == imeControlIMM32 {
		return setIMEByIMM32(open, target)
	}
	return setIMEByVK(open)
}

// setIMEByVK sends VK_IME_ON or VK_IME_OFF as one tagged down/up pair. The
// absolute keys (not a toggle) are honored by Microsoft IME on Windows 10
// 1903+ per the keyboard-japan-ime design guidance.
func setIMEByVK(open bool) bool {
	vk := uint16(vkImeOff)
	name := "VK_IME_OFF"
	if open {
		vk = uint16(vkImeOn)
		name = "VK_IME_ON"
	}
	n, errno := sendKeyPair(vk)
	if n != 2 {
		// UIPI rejections surface here as a short insert count; the exact
		// cause is not distinguishable from the return value alone.
		debugf("SendInput(%s): inserted %d/2, errno=%d", name, n, errno)
		return false
	}
	return true
}

// queryImeOpen asks for the IME open status of whatever window really holds
// keyboard focus (WM_IME_CONTROL/IMC_GETOPENSTATUS) under the same hard
// deadline as the IMM32 switch path. The default IME window is per thread,
// and in WebView2/Chromium hosts the focused child lives on a different
// thread — often a different process — than the top-level window, so asking
// the top-level's thread would report "closed" while the user is composing.
// ok is false when no answer could be obtained (no focus/IME window,
// timeout, hung target, UIPI denial). Used by the Enter guard only; it is
// deliberately not a universal "real IME state" probe (CON-5 still holds
// for the OSD).
func queryImeOpen(target uintptr) (open, ok bool) {
	focus := guiFocusWindow(0) // 0 = foreground thread
	if focus == 0 {
		focus = guiFocusWindow(windowThreadId(target))
	}
	if focus == 0 {
		focus = target
	}
	imeWnd := immGetDefaultIMEWnd(focus)
	if imeWnd == 0 {
		return false, false
	}
	status, ok, _ := sendMessageTimeout(imeWnd, wmImeControl, imcGetOpenStatus, 0, smtoAbortIfHung|smtoBlock, imm32TimeoutMs)
	return status != 0, ok
}

// setIMEByIMM32 targets the default IME window of the tap-time foreground
// window with WM_IME_CONTROL/IMC_SETOPENSTATUS under a hard deadline.
// A plain SendMessage (unbounded wait on a foreign window) is never used.
func setIMEByIMM32(open bool, target uintptr) bool {
	imeWnd := immGetDefaultIMEWnd(target)
	if imeWnd == 0 {
		debugf("ImmGetDefaultIMEWnd(%#x) returned NULL", target)
		return false
	}
	_, ok, errno := sendMessageTimeout(imeWnd, wmImeControl, imcSetOpenStatus, boolToUintptr(open), smtoAbortIfHung|smtoBlock, imm32TimeoutMs)
	if !ok {
		// Timeout, hung target, or UIPI denial all land here.
		debugf("SendMessageTimeoutW(IMC_SETOPENSTATUS) failed, errno=%d", errno)
		return false
	}
	return true
}
