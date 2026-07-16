package main

import "testing"

func TestScaleDPI(t *testing.T) {
	cases := []struct {
		v    int32
		dpi  uint32
		want int32
	}{
		{96, 96, 96},
		{110, 96, 110},
		{110, 144, 165}, // 150%
		{110, 192, 220}, // 200%
		{1, 144, 2},     // 1.5 rounds half up
		{0, 192, 0},
		{22, 120, 28}, // 27.5 rounds up
	}
	for _, c := range cases {
		if got := scaleDPI(c.v, c.dpi); got != c.want {
			t.Errorf("scaleDPI(%d, %d) = %d, want %d", c.v, c.dpi, got, c.want)
		}
	}
}

func TestOsdMetricsScaled(t *testing.T) {
	m := osdBase.scaled(192)
	want := osdMetrics{
		boxW:         osdBase.boxW * 2,
		boxH:         osdBase.boxH * 2,
		corner:       osdBase.corner * 2,
		marginBottom: osdBase.marginBottom * 2,
		fontHeight:   osdBase.fontHeight * 2,
	}
	if m != want {
		t.Fatalf("scaled(192) = %+v, want %+v", m, want)
	}
	if osdBase.scaled(96) != osdBase {
		t.Fatal("scaled(96) must be identity")
	}
}

func TestOsdTimerID(t *testing.T) {
	if osdTimerID(1, false) == osdTimerID(1, true) {
		t.Fatal("hold and fade IDs must differ")
	}
	if osdTimerID(1, false) == osdTimerID(2, false) {
		t.Fatal("generations must produce distinct IDs")
	}
	if osdTimerID(1, false) == 0 || osdTimerID(1, true) == 0 {
		t.Fatal("timer IDs must never be 0")
	}
}

func TestFailThrottle(t *testing.T) {
	var th failThrottle
	if !th.allow(1000) {
		t.Fatal("first failure must be shown")
	}
	if th.allow(1000 + failOsdSuppressMs - 1) {
		t.Fatal("failure inside the window must be suppressed")
	}
	if !th.allow(1000 + failOsdSuppressMs) {
		t.Fatal("failure at the window edge must be shown")
	}
	if !th.allow(1000 + failOsdSuppressMs + failOsdSuppressMs + 5) {
		t.Fatal("failure after the window must be shown")
	}
}

func TestGuardWParamPacking(t *testing.T) {
	for _, send := range []bool{false, true} {
		for _, composing := range []bool{false, true} {
			gotSend, gotComposing := unpackGuardWParam(packGuardWParam(send, composing))
			if gotSend != send || gotComposing != composing {
				t.Errorf("roundtrip(%v, %v) = (%v, %v)", send, composing, gotSend, gotComposing)
			}
		}
	}
}

func TestSwitchWParamPacking(t *testing.T) {
	cases := []struct {
		open bool
		vk   uint32
	}{
		{false, vkLMenu},
		{true, vkRMenu},
		{true, 0xFF},
		{false, 1},
	}
	for _, c := range cases {
		open, vk := unpackSwitchWParam(packSwitchWParam(c.open, c.vk))
		if open != c.open || vk != c.vk {
			t.Errorf("roundtrip(%v, %#x) = (%v, %#x)", c.open, c.vk, open, vk)
		}
	}
}
