package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: procview logic does not exist yet.
func TestSortDefaultsToTotalBusiestFirst(t *testing.T) {
	if DefaultSort.Column != model.SortTotal || !DefaultSort.Reverse {
		t.Fatalf("default sort = %+v, want total desc", DefaultSort)
	}
}

func TestUnknownValuesSortLastBothDirections(t *testing.T) {
	// Unknown = a client with no gfx engine reading (nil, not 0): the
	// reference pins this on the gfx column, where None is real.
	withEng := testProcess(1001, 40.0)
	withoutEng := testProcessNoEngines(1002)
	desc := SortProcesses([]model.GpuProcess{withoutEng, withEng}, SortState{Column: model.SortGfx, Reverse: true})
	if len(desc) != 2 || desc[0].PID != 1001 {
		t.Fatalf("desc = %v, want measured first", pids(desc))
	}
	asc := SortProcesses([]model.GpuProcess{withoutEng, withEng}, SortState{Column: model.SortGfx, Reverse: false})
	if len(asc) != 2 || asc[0].PID != 1001 {
		t.Fatalf("asc = %v, want measured first (unknowns last both ways)", pids(asc))
	}
}

func TestTextFilterMatchesNameCaseInsensitively(t *testing.T) {
	procs := []model.GpuProcess{testProcessNamed(1001, "llama-server"), testProcessNamed(1002, "firefox")}
	got := FilterProcesses(procs, FilterState{Text: "LLAMA"})
	if len(got) != 1 || got[0].PID != 1001 {
		t.Fatalf("filter = %v, want [1001]", pids(got))
	}
}

func TestTreeChildSitsUnderParent(t *testing.T) {
	parent := testProcessNamed(100, "parent")
	child := testProcessNamed(101, "child")
	ppid := 100
	child.PPID = &ppid
	rows := BuildRows([]model.GpuProcess{parent, child}, SortState{Column: model.SortPID, Reverse: false}, FilterState{}, true)
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].Depth != 0 || rows[1].Depth != 1 {
		t.Fatalf("depths = %d,%d, want 0,1", rows[0].Depth, rows[1].Depth)
	}
}

func pids(procs []model.GpuProcess) []int {
	out := make([]int, 0, len(procs))
	for _, p := range procs {
		out = append(out, p.PID)
	}
	return out
}

func testProcess(pid int, gfx float64) model.GpuProcess {
	win := model.Nanoseconds(1_000_000_000)
	delta := model.Nanoseconds(gfx / 100.0 * float64(win))
	total := delta + win
	if total <= 0 {
		total = 1
	}
	return model.GpuProcess{
		PID: pid, Name: "proc", User: "u", BDF: "0000:0c:00.0", ClientID: pid,
		Engines:  []model.EngineUsage{{Engine: "gfx", TotalNs: total, DeltaNs: delta, WindowNs: win}},
		VramUsed: bytesPtr(0),
	}
}

func testProcessNoEngines(pid int) model.GpuProcess {
	return model.GpuProcess{PID: pid, Name: "idle", User: "u", BDF: "0000:0c:00.0", ClientID: pid}
}

func testProcessNamed(pid int, name string) model.GpuProcess {
	win := model.Nanoseconds(1_000_000_000)
	delta := int64(0.05 * float64(win))
	total := delta + win
	p := model.GpuProcess{
		PID: pid, Name: name, User: "u", BDF: "0000:0c:00.0", ClientID: pid,
		Cmdline:  []string{name, "--arg"},
		Engines:  []model.EngineUsage{{Engine: "gfx", TotalNs: total, DeltaNs: delta, WindowNs: win}},
		VramUsed: bytesPtr(1024),
	}
	return p
}

func bytesPtr(v model.Bytes) *model.Bytes { return &v }

func TestCellValueUnmeasuredEngineIsNANotZero(t *testing.T) {
	p := testProcessNoEngines(1001)
	if got := CellValue(p, model.SortGfx, false); got != NA {
		t.Fatalf("cell = %q, want N/A", got)
	}
}

func TestPanelTitleNamesTopProcess(t *testing.T) {
	procs := []model.GpuProcess{testProcess(1001, 40.0)}
	top := BusiestProcesses(procs, DefaultSort)
	title := PanelTitle(DefaultSort, FilterState{}, len(procs), len(procs), top, false)
	if !strings.Contains(title, "1001") {
		t.Fatalf("title = %q, want top pid", title)
	}
}
