package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// RED: the screen composition does not exist yet.
func TestScreenFitsExplicitDimensions(t *testing.T) {
	in := screenInput()
	for _, width := range []int{60, 72, 80, 120, 200} {
		for _, height := range []int{20, 28, 40} {
			in.Width, in.Height = width, height
			for i, row := range RenderScreen(in) {
				if n := len([]rune(Plain(row))); n > width {
					t.Fatalf("%dx%d line %d is %d cells: %q", width, height, i, n, Plain(row))
				}
			}
		}
	}
}

func TestScreenDetailShowsHeaderSensorsAndTable(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	for _, want := range []string{"AMD Radeon Test", "Sensors", "COMMAND", "q quit"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("screen misses %q:\n%s", want, joined)
		}
	}
}

func TestShortTerminalHidesTheProcessTable(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 19
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if strings.Contains(joined, "COMMAND") {
		t.Fatalf("short screen still shows the process table:\n%s", joined)
	}
}

func TestNarrowTerminalStacksSensorsUnderGraphs(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 60, 40
	lines := plainRows(RenderScreen(in))
	graph, sensors := -1, -1
	for i, line := range lines {
		if strings.Contains(line, "GPU busy") && graph < 0 {
			graph = i
		}
		if strings.TrimSpace(line) == "Sensors" && sensors < 0 {
			sensors = i
		}
	}
	if graph < 0 || sensors < 0 {
		t.Fatalf("graph=%d sensors=%d, both expected:\n%s", graph, sensors, strings.Join(lines, "\n"))
	}
	if sensors < graph {
		t.Fatalf("sensors (%d) precede the graph (%d) on a narrow screen", sensors, graph)
	}
}

func TestOverviewReplacesTheDetailView(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	in.View.Overview = true
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if !strings.Contains(joined, "AMD Radeon Test") {
		t.Fatalf("overview misses the device:\n%s", joined)
	}
	if strings.Contains(joined, "Sensors") {
		t.Fatalf("overview still shows the detail view:\n%s", joined)
	}
}

func TestMemoryGraphsHideBelowTheirFloor(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 27
	lines := plainRows(RenderScreen(in))
	in.Height = 40
	tall := plainRows(RenderScreen(in))
	if titledLine(lines, "VRAM") {
		t.Fatalf("27 rows still shows the VRAM graph:\n%s", strings.Join(lines, "\n"))
	}
	if !titledLine(tall, "VRAM") {
		t.Fatalf("40 rows misses the VRAM graph:\n%s", strings.Join(tall, "\n"))
	}
}

func TestScreenGolden(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 100, 40
	got := strings.Join(plainRows(RenderScreen(in)), "\n") + "\n"
	const golden = "testdata/screen_100x40.golden"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run with UPDATE_GOLDEN=1 to create): %v", err)
	}
	if got != string(want) {
		t.Fatalf("screen changed; diff:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// screenInput is a fully populated synthetic machine: one GPU with a
// complete sample, two clients and default view state.
func screenInput() LayoutInput {
	m := fullMetrics()
	snap := model.GpuSnapshot{
		Sequence: 7, Devices: []model.AmdgpuMetrics{m},
		Processes:           []model.GpuProcess{testProcessNamed(1001, "llama-server"), testProcessNamed(1002, "firefox")},
		VisibleProcessCount: 2, TotalProcessCount: 2,
	}
	return LayoutInput{
		Snapshot: snap,
		History:  []model.GpuSnapshot{snap},
		View:     *NewViewState(),
		Theme:    DefaultTheme,
		Width:    100, Height: 40,
		UptimeS: 3725, IntervalS: 1.0,
	}
}

func plainRows(rows []Row) []string { return PlainLines(rows) }

func titledLine(lines []string, title string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == title {
			return true
		}
	}
	return false
}
