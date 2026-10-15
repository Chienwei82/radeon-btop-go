package ui

import (
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: device tabs + overview do not exist yet.
func TestTabSpansAddUpToDrawnLine(t *testing.T) {
	devices := []model.AmdgpuMetrics{testMetrics(), testMetrics()}
	spans := TabSpans(devices)
	line := Plain(GPUTabs(devices, GetTheme("default"), 0, 200))
	total := 0
	for _, s := range spans {
		total += s.Width + TabGap
		if s.Start+s.Width > runeLen(line)+TabGap {
			t.Fatalf("span %+v exceeds line %q", s, line)
		}
	}
	if total-TabGap != runeLen(line) {
		t.Fatalf("spans total %d != line width %d (%q)", total-TabGap, runeLen(line), line)
	}
}

func TestDeviceAtResolvesClicks(t *testing.T) {
	devices := []model.AmdgpuMetrics{testMetrics(), testMetrics()}
	spans := TabSpans(devices)
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
	if got := DeviceAt(devices, spans[0].Start); got != 0 {
		t.Fatalf("device at tab 0 = %d, want 0", got)
	}
	if got := DeviceAt(devices, spans[1].Start); got != 1 {
		t.Fatalf("device at tab 1 = %d, want 1", got)
	}
	if got := DeviceAt(devices, -1); got != -1 {
		t.Fatalf("device at -1 = %d, want -1", got)
	}
}

func TestOverviewPanelNamesEveryDevice(t *testing.T) {
	devices := []model.AmdgpuMetrics{testMetrics(), fullMetrics()}
	rows := PlainLines(OverviewPanel(devices, GetTheme("default"), 120, 0, nil))
	joined := joinLines(rows)
	if countOccurrences(joined, "AMD Radeon Test") < 2 {
		t.Fatalf("overview = %q, want both devices named", joined)
	}
}

func countOccurrences(s, sub string) int {
	n := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			n++
		}
	}
	return n
}

func runeLen(s string) int { return len([]rune(s)) }
