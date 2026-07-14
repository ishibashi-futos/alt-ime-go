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
