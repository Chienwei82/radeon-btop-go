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
			if !strings.Contains(line, SelectMarker) {
				t.Fatalf("selected row = %q, want the %q marker", line, SelectMarker)
			}
		}
	}
	if marked != 1 {
		t.Fatalf("marked rows = %d, want exactly the selected one", marked)
	}
}

// A table taller than its frame scrolls to keep the selected row visible: the
// window centres the selection rather than letting it move off-screen.
func TestProcessBlockScrollsToKeepSelectionVisible(t *testing.T) {
	procs := make([]model.GpuProcess, 0, 60)
	for i := 0; i < 60; i++ {
		procs = append(procs, testProcessNamed(2000+i, "client"))
	}
	snap := model.GpuSnapshot{
		Sequence: 1, Devices: []model.AmdgpuMetrics{fullMetrics()},
		Processes: procs, VisibleProcessCount: len(procs), TotalProcessCount: len(procs),
	}
	sel := idOf(procs[50]) // near the end of the list
	in := LayoutInput{
		Snapshot: snap, View: ViewState{Sort: ParseSort("pid", nil), Selected: &sel},
		Theme: DefaultTheme, Width: 120, Height: 40,
	}
	lines := plainRows(processBlock(in, DefaultTheme, 11, 120))
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "2050") {
		t.Fatalf("scrolled table misses the selected row 2050:\n%s", joined)
	}
	if strings.Contains(joined, "2000") {
		t.Fatalf("scrolled table still shows the first row 2000:\n%s", joined)
	}
}

// The selection and the "busiest" hint are different glyphs: the cursor row
// carries SelectMarker, a top row that is not selected carries TopMarker, so
// the two never collapse into one ambiguous highlight.
func TestSelectionAndTopUseDifferentMarkers(t *testing.T) {
	a := testProcessNamed(1001, "aaa")
	b := testProcessNamed(1002, "bbb")
	procs := []model.GpuProcess{a, b}
	rows := BuildRows(procs, ParseSort("pid", nil), FilterState{ShowIdle: true}, false)
	sel := idOf(b) // b is selected; a is top (busiest)
	got := PlainLines(RenderTable(rows, ParseSort("pid", nil), TableConfig{
		Width: 120, Selected: &sel, Top: []model.GpuProcess{a},
	}))
	var aLine, bLine string
	for _, line := range got[1:] {
		if strings.Contains(line, "1001") {
			aLine = line
		}
		if strings.Contains(line, "1002") {
			bLine = line
		}
	}
	if !strings.Contains(aLine, TopMarker) {
		t.Fatalf("top row = %q, want the %q marker", aLine, TopMarker)
	}
	if !strings.Contains(bLine, SelectMarker) {
		t.Fatalf("selected row = %q, want the %q marker", bLine, SelectMarker)
	}
}
