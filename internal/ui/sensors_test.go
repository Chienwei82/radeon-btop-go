package ui

import "testing"

// RED: utilisation / sensors / footer builders do not exist yet.
func TestDiscreteShowsVRAM(t *testing.T) {
	m := testMetrics()
	rows := PlainLines(UtilisationRows(m, GetTheme("default"), 50, nil))
	joined := joinLines(rows)
	if !contains(joined, "VRAM") {
		t.Fatalf("rows = %q, want VRAM on dgpu", joined)
	}
	for _, r := range rows {
		if len(r) >= 3 && r[:3] == "GTT" {
			t.Fatalf("rows = %q, dgpu must not lead with GTT", joined)
		}
	}
}

func TestBareDeviceNeverClaimsZero(t *testing.T) {
	rows := PlainLines(SensorsPanel(testMetrics(), GetTheme("default"), 50, nil))
	joined := joinLines(rows)
	if contains(joined, "0 MHz") || contains(joined, "0.0 W") {
		t.Fatalf("sensors = %q, absence must not render as zero", joined)
	}
	if countNA(joined) < 4 {
		t.Fatalf("sensors = %q, want several N/A markers", joined)
	}
}

func TestSensorsFullDevice(t *testing.T) {
	m := fullMetrics()
	joined := joinLines(PlainLines(SensorsPanel(m, GetTheme("default"), 50, nil)))
	for _, want := range []string{"Core", "Memory", "SoC", "temp edge", "temp junction", "Power", "Fan", "45.0 W", "cap 211 W", "Gen3 x16", "BOOTUP_DEFAULT", "throttle none"} {
		if !contains(joined, want) {
			t.Fatalf("sensors = %q, want %q", joined, want)
		}
	}
}

func TestFooterFlagsPartialView(t *testing.T) {
	th := GetTheme("default")
	vis, tot := 15, 580
	got := Plain(FooterPanel(th, FooterInfo{Sequence: 1, TickMs: 1.0, CPU: "1%", Visible: &vis, Total: &tot}))
	if !contains(got, "partial 15/580 procs") {
		t.Fatalf("footer = %q, want partial note", got)
	}
	vis, tot = 580, 580
	got = Plain(FooterPanel(th, FooterInfo{Sequence: 1, TickMs: 1.0, CPU: "1%", Visible: &vis, Total: &tot}))
	if contains(got, "partial") {
		t.Fatalf("footer = %q, complete view must not say partial", got)
	}
}

func joinLines(rows []string) string {
	out := ""
	for i, r := range rows {
		if i > 0 {
			out += "\n"
		}
		out += r
	}
	return out
}

func countNA(s string) int {
	n := 0
	for i := 0; i+len(NA) <= len(s); i++ {
		if s[i:i+len(NA)] == NA {
			n++
		}
	}
	return n
}
