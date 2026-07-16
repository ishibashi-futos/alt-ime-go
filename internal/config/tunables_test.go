package config

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
		if got := ScaleDPI(c.v, c.dpi); got != c.want {
			t.Errorf("ScaleDPI(%d, %d) = %d, want %d", c.v, c.dpi, got, c.want)
		}
	}
}

func TestOsdMetricsScaled(t *testing.T) {
	m := OsdBase.Scaled(192)
	want := OsdMetrics{
		BoxW:         OsdBase.BoxW * 2,
		BoxH:         OsdBase.BoxH * 2,
		Corner:       OsdBase.Corner * 2,
		MarginBottom: OsdBase.MarginBottom * 2,
		FontHeight:   OsdBase.FontHeight * 2,
	}
	if m != want {
		t.Fatalf("scaled(192) = %+v, want %+v", m, want)
	}
	if OsdBase.Scaled(96) != OsdBase {
		t.Fatal("scaled(96) must be identity")
	}
}

func TestOsdTimerID(t *testing.T) {
	if OsdTimerID(1, false) == OsdTimerID(1, true) {
		t.Fatal("hold and fade IDs must differ")
	}
	if OsdTimerID(1, false) == OsdTimerID(2, false) {
		t.Fatal("generations must produce distinct IDs")
	}
	if OsdTimerID(1, false) == 0 || OsdTimerID(1, true) == 0 {
		t.Fatal("timer IDs must never be 0")
	}
}

func TestFailThrottle(t *testing.T) {
	var th FailThrottle
	if !th.Allow(1000) {
		t.Fatal("first failure must be shown")
	}
	if th.Allow(1000 + FailOsdSuppressMs - 1) {
		t.Fatal("failure inside the window must be suppressed")
	}
	if !th.Allow(1000 + FailOsdSuppressMs) {
		t.Fatal("failure at the window edge must be shown")
	}
	if !th.Allow(1000 + FailOsdSuppressMs + FailOsdSuppressMs + 5) {
		t.Fatal("failure after the window must be shown")
	}
}

func TestMatchGuardTarget(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{`C:\Users\u\AppData\Local\AnthropicClaude\claude.exe`, true},
		{`C:\Program Files\WindowsApps\M365Copilot.exe`, true},
		{`C:\APPS\CLAUDE.EXE`, true},
		{"claude.exe", true},
		{`C:\Windows\notepad.exe`, false},
		{`C:\apps\claude.exe.bak`, false},
		{`C:\claude.exe\other.exe`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := MatchGuardTarget(tt.path); got != tt.want {
			t.Errorf("MatchGuardTarget(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}
