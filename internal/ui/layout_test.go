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
		if strings.Contains(line, "╭─ GPU busy") && graph < 0 {
			graph = i
		}
		if strings.Contains(line, "╭─ Sensors") && sensors < 0 {
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

// profileInput is the golden machine with the SCPP tables the power-profile
// panel reads. Nothing else changes, so a difference between this and
// screenInput() is the optional row and nothing else.
func profileInput() LayoutInput {
	in := screenInput()
	in.Snapshot.Devices[0].Profiles = model.PowerProfileTable{
		Profiles: []model.PowerProfileEntry{
			{Index: 0, Name: "BOOTUP_DEFAULT", Active: true},
			{Index: 1, Name: "3D_FULL_SCREEN"},
		},
		Present: true,
	}
	return in
}

func TestOptionalRowIsHiddenWithoutAnyData(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 160, 50
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if strings.Contains(joined, "Power profile") || strings.Contains(joined, "Blocks ·") {
		t.Fatalf("a panel with nothing to say must not appear at all:\n%s", joined)
	}
}

func TestOptionalRowAppearsWhenTheProfileTablesExist(t *testing.T) {
	in := profileInput()
	in.Width, in.Height = 160, 50
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	for _, want := range []string{"Power profile", "BOOTUP_DEFAULT", "read-only"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("screen misses %q:\n%s", want, joined)
		}
	}
}

func TestOptionalRowWaitsForItsHeightFloor(t *testing.T) {
	in := profileInput()
	in.Width = 160
	in.Height = MinHeightForOptionalRow - 1
	if joined := strings.Join(plainRows(RenderScreen(in)), "\n"); strings.Contains(joined, "Power profile") {
		t.Fatalf("optional row drawn at %d rows, below its floor:\n%s", in.Height, joined)
	}
	in.Height = MinHeightForOptionalRow
	if joined := strings.Join(plainRows(RenderScreen(in)), "\n"); !strings.Contains(joined, "Power profile") {
		t.Fatalf("optional row missing at its own floor (%d rows):\n%s", in.Height, joined)
	}
}

func TestOneThresholdGovernsBothOptionalPanels(t *testing.T) {
	if MinHeightForOptionalRow <= MinHeightForSensorsComplete {
		t.Fatalf("optional row floor %d must sit above the sensors' %d",
			MinHeightForOptionalRow, MinHeightForSensorsComplete)
	}
}

// The invariant behind MinHeightForOptionalRow: the sensor panel is what
// scrolls, so an optional row must never be the reason a primary reading
// leaves the screen.
func TestOptionalRowNeverCostsAPrimarySensorReading(t *testing.T) {
	primary := []string{"Core", "Memory", "SoC", "temp edge", "temp junction", "Power", "cap 211 W"}
	for _, height := range []int{MinHeightForOptionalRow, MinHeightForOptionalRow + 6} {
		in := profileInput()
		in.Width, in.Height = 120, height
		joined := strings.Join(plainRows(RenderScreen(in)), "\n")
		for _, want := range primary {
			if !strings.Contains(joined, want) {
				t.Fatalf("%q scrolled off at height %d:\n%s", want, height, joined)
			}
		}
	}
}

func TestOptionalRowShowsTheBlocksPanelBesideTheProfilePanel(t *testing.T) {
	in := profileInput()
	b := blockSample()
	in.Snapshot.Devices[0].Blocks = &b
	in.Width, in.Height = 160, 50
	lines := plainRows(RenderScreen(in))
	joined := strings.Join(lines, "\n")
	sameLine := false
	for _, line := range lines {
		if strings.Contains(line, "Blocks") && strings.Contains(line, "Power profile") {
			sameLine = true
		}
	}
	if !sameLine {
		t.Fatalf("the two optional panels are not on one row:\n%s", joined)
	}
	for _, want := range []string{"Graphics pipe", "Event Engine", "Texture Cache"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("screen misses %q:\n%s", want, joined)
		}
	}
}

// The optional row is cropped to the height the reference measured it at,
// rather than allowed to grow until the sensor panel starts scrolling.
func TestOptionalRowIsCroppedToItsMeasuredHeight(t *testing.T) {
	in := profileInput()
	b := blockSample()
	in.Snapshot.Devices[0].Blocks = &b
	in.Width, in.Height = 160, 50
	lines := plainRows(RenderScreen(in))
	blocksRows := 0
	for _, line := range lines {
		if strings.Contains(line, "Graphics pipe") || strings.Contains(line, "Vertex Grouper") ||
			strings.Contains(line, "Texture Cache") || strings.Contains(line, "Depth Block") {
			blocksRows++
		}
	}
	if blocksRows > optionalRowLines {
		t.Fatalf("blocks panel drew %d rows, above its budget of %d", blocksRows, optionalRowLines)
	}
}

func TestAlertNoticeReachesTheStatusLine(t *testing.T) {
	in := screenInput()
	in.Alerts = model.AlertState{
		Level:    model.AlertAlert,
		Breaches: []model.AlertBreach{{Metric: "temp junction", Reading: "94C", Limit: "90C", Level: model.AlertAlert}},
	}
	in.Width, in.Height = 120, 40
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if !strings.Contains(joined, "temp junction 94C > 90C") {
		t.Fatalf("status line misses the alert:\n%s", joined)
	}
}

func TestNoAlertLeavesTheStatusLineAlone(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	for _, line := range plainRows(RenderScreen(in)) {
		if strings.HasPrefix(line, "#7") && strings.Contains(line, " > ") {
			t.Fatalf("status line carries an alert with nothing crossed: %q", line)
		}
	}
}

// A cropped sensor panel must say that it is cropped: a dozen readings with
// the last six silently missing looks like the card stopped reporting them.
func TestTruncatedSensorsSayHowManyRowsAreHidden(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 20
	m := in.Snapshot.Devices[0]
	for i := 0; i < 6; i++ {
		c := model.Celsius(30 + i)
		m.Temperatures = append(m.Temperatures, model.Temperature{Label: "extra", Celsius: &c})
	}
	in.Snapshot.Devices[0] = m
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if !strings.Contains(joined, "… ") || !strings.Contains(joined, " more") {
		t.Fatalf("cropped sensors carry no scroll marker:\n%s", joined)
	}
}

func TestCompleteSensorsCarryNoScrollMarker(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	joined := strings.Join(plainRows(RenderScreen(in)), "\n")
	if strings.Contains(joined, " more") {
		t.Fatalf("complete sensor panel claims hidden rows:\n%s", joined)
	}
}

func titledLine(lines []string, title string) bool {
	for _, line := range lines {
		if strings.TrimSpace(line) == title || strings.Contains(line, "╭─ "+title) {
			return true
		}
	}
	return false
}
