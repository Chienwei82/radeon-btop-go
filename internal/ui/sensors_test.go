package ui

import (
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

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

// The status line is facts, not prose: values and separators are separate
// runs so a warning or an alert can be painted without repainting the line.
func TestFooterPanelSeparatesValuesFromSeparators(t *testing.T) {
	th := GetTheme("default")
	vis, tot := 15, 580
	row := FooterPanel(th, FooterInfo{
		Sequence: 7, MetricsSource: "gpu_metrics", TickMs: 4, CPU: "2.6%",
		Visible: &vis, Total: &tot,
		Warnings:     []string{"warn a", "warn b"},
		BlocksStatus: model.BlocksMissing, BlocksHint: "radeontop missing",
	})
	want := "#7  ·  metrics gpu_metrics  ·  tick 4 ms  ·  cpu 2.6%  ·  " +
		"partial 15/580 procs  ·  warn a (+1 more)  ·  blocks: radeontop missing"
	if got := Plain(row); got != want {
		t.Fatalf("footer text = %q, want %q", got, want)
	}
	separators := 0
	sequence := false
	for _, c := range row.Cells {
		if c.Text == "  ·  " {
			separators++
			if c.Color != th.Track {
				t.Fatalf("separator colour = %q, want the track colour", c.Color)
			}
		}
		if c.Text == "#7" && c.Color == th.Accent {
			sequence = true
		}
	}
	if separators != 6 {
		t.Fatalf("separators = %d, want 6 (six facts after the sequence)", separators)
	}
	if !sequence {
		t.Fatalf("sequence number is not its own accent run: %#v", row.Cells)
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
