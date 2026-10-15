package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: the process table renderer does not exist yet.
func TestTableRendersHeaderAndOneRow(t *testing.T) {
	p := testProcessNamed(1001, "llama-server")
	rows := BuildRows([]model.GpuProcess{p}, DefaultSort, FilterState{ShowIdle: true}, false)
	got := PlainLines(RenderTable(rows, DefaultSort, TableConfig{Width: 120, FullCommand: false, Selected: nil, Top: nil}))
	if len(got) < 2 {
		t.Fatalf("table lines = %d, want header + 1 row", len(got))
	}
	if !strings.Contains(got[0], "PID") || !strings.Contains(got[0], "COMMAND") {
		t.Fatalf("header = %q, want column titles", got[0])
	}
	if !strings.Contains(got[1], "llama-server") || !strings.Contains(got[1], "1001") {
		t.Fatalf("row = %q, want name and pid", got[1])
	}
}

func TestTableFitsWidth(t *testing.T) {
	p := testProcessNamed(1001, "llama-server")
	rows := BuildRows([]model.GpuProcess{p}, DefaultSort, FilterState{ShowIdle: true}, false)
	for _, width := range []int{60, 80, 120, 200} {
		cfg := TableConfig{Width: width}
		for _, line := range PlainLines(RenderTable(rows, DefaultSort, cfg)) {
			if n := len([]rune(line)); n > width {
				t.Fatalf("width %d: line %q is %d cells", width, line, n)
			}
		}
	}
}

func TestTableSelectionSurvivesResort(t *testing.T) {
	a := testProcessNamed(1001, "aaa")
	b := testProcessNamed(1002, "bbb")
	rows := BuildRows([]model.GpuProcess{a, b}, ParseSort("pid", nil), FilterState{ShowIdle: true}, false)
	sel := idOf(b)
	got := PlainLines(RenderTable(rows, ParseSort("pid", nil), TableConfig{Width: 120, Selected: &sel}))
	marked := 0
	for _, line := range got[1:] {
		if strings.Contains(line, "1002") {
			marked++
			if !strings.HasPrefix(strings.TrimSpace(line), TopMarker) && !strings.Contains(line, "▶") {
				t.Fatalf("selected row = %q, want selection marker", line)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("marked rows = %d, want exactly the selected one", marked)
	}
}
