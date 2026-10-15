package ui

import (
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// LayoutInput is everything the screen composition needs: one snapshot,
// the retained history the graphs plot, and the view state the keys drive.
// Pure input, so the whole screen renders in a test without a terminal.
type LayoutInput struct {
	Snapshot  model.GpuSnapshot
	History   []model.GpuSnapshot
	View      ViewState
	Theme     Theme
	Width     int
	Height    int
	UptimeS   float64
	IntervalS float64
	// Alerts is the threshold state for this sample. The zero value means
	// nothing is crossed, so the status line stays as it is.
	Alerts model.AlertState
}

// headerLines is the fixed device header height; footerLines is the status
// line plus the key hints.
const (
	headerLines = 3
	footerLines = 2
)

// MinHeightForSensorsComplete is the terminal height at which the sensor
// panel shows every one of its rows without scrolling.
const MinHeightForSensorsComplete = 38

// MinHeightForOptionalRow is the terminal height at which the optional row
// of panels may be added.
//
// One threshold for both panels, and it is higher than either needs on its
// own. The cost is not the row's height but what it takes from the sensor
// panel beside it: the sensor panel scrolls, so a row added below the main
// one does not lose its content, it pushes the last lines out of view. An
// optional panel must never be the reason a required one needs scrolling.
const MinHeightForOptionalRow = MinHeightForSensorsComplete + 8

// optionalRowLines is the height the optional row is allowed: the reference
// implementation measured the cost of the row at eight rows, and the panels
// are cropped to it rather than allowed to push the sensors off the screen.
const optionalRowLines = 8

// minLinesForMainRow is the floor the graph and sensor row keeps before the
// optional row is dropped for lack of space.
const minLinesForMainRow = 10

// RenderScreen composes the whole interface for one terminal size.
//
// The graphs are the point of the app, so they are never what gets dropped:
// on a short terminal the process table yields first, and on a narrow one
// the sensors move underneath the graphs instead of squeezing them to
// nothing. Every returned line is at most in.Width cells.
func RenderScreen(in LayoutInput) []Row {
	th := in.Theme
	if th.Name == "" {
		th = DefaultTheme
	}
	width, height := in.Width, in.Height
	device := focusedDevice(in.Snapshot, in.View.DeviceIndex)

	out := []Row{}
	if device != nil {
		header := SplitRows(HeaderPanel(*device, th, in.UptimeS, in.IntervalS))
		for _, r := range header {
			out = append(out, fitRow(r, width))
		}
	}
	if len(in.Snapshot.Devices) > 1 {
		out = append(out, fitRow(GPUTabs(in.Snapshot.Devices, th, in.View.DeviceIndex, width), width))
	}

	rest := height - len(out) - footerLines
	if rest < 4 {
		rest = 4
	}
	tableLines := 0
	short := height < ShortHeight
	if in.View.ShowProcesses && !short && rest >= 8 {
		tableLines = rest / 3
		rest -= tableLines
	}

	if in.View.Overview {
		for _, r := range OverviewPanel(in.Snapshot.Devices, th, width, in.View.DeviceIndex, nil) {
			out = append(out, fitRow(r, width))
			rest--
			if rest <= 0 {
				break
			}
		}
	} else {
		optional := optionalRow(device, in, th, rest, width)
		if optional != nil {
			rest -= len(optional)
		}
		out = append(out, detailColumns(device, in, th, rest, width)...)
		if optional != nil {
			out = append(out, optional...)
		}
	}

	if tableLines > 0 && device != nil {
		out = append(out, processBlock(in, th, tableLines, width)...)
	}

	out = append(out, fitRow(statusLine(in, th), width))
	out = append(out, fitRow(keyHints(th, width), width))
	return out
}

// optionalRow is the row of panels that only appears when there is something
// to show and the terminal is tall enough to afford it: the per-block
// utilisation beside the power profile.
//
// It returns nil when either condition fails, and nil is the signal the
// caller uses to leave the main row every line it had. A panel with nothing
// to say must not appear at all -- a bordered empty box costs the sensor
// panel a row and answers nothing.
func optionalRow(device *model.AmdgpuMetrics, in LayoutInput, th Theme, rest, width int) []Row {
	if device == nil || in.Height < MinHeightForOptionalRow {
		return nil
	}
	if rest-optionalRowLines < minLinesForMainRow {
		return nil
	}
	blocks := BlocksPanel(device.Blocks, th, width, nil)
	profile := PowerProfilePanel(*device, th, width, nil)
	hasProfile := device.Profiles.Present || device.ODC.Present
	if device.Blocks == nil && !hasProfile {
		return nil
	}
	if device.Blocks == nil {
		return crop(append([]Row{titleRow("Power profile", th)}, profile...), optionalRowLines, width)
	}
	if !hasProfile {
		return crop(append([]Row{titleRow(BlocksHeading(device.Blocks), th)}, blocks...), optionalRowLines, width)
	}
	if width < 2*MinOptionalPanelWidth+1 {
		// Too narrow for two columns: stack them rather than squeeze the bars
		// below the length at which they say anything.
		return crop(append(blocks, profile...), optionalRowLines, width)
	}
	left := (width - 1) / 2
	right := width - left - 1
	leftRows := append([]Row{titleRow(BlocksHeading(device.Blocks), th)}, blocks...)
	rightRows := append([]Row{titleRow("Power profile", th)}, profile...)
	return crop(joinColumns(leftRows, rightRows, left, right, 1), optionalRowLines, width)
}

// MinOptionalPanelWidth is the narrowest an optional panel is laid out at:
// its bars are sized from the panel, and a panel narrower than its own label
// column produces a bar of no useful length.
const MinOptionalPanelWidth = 30

// crop trims a block to at most lines rows, each fitted to width.
func crop(rows []Row, lines, width int) []Row {
	if len(rows) > lines {
		rows = rows[:lines]
	}
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		out = append(out, fitRow(r, width))
	}
	return out
}

// detailColumns is the graphs column plus the sensors column: side by side
// on a wide terminal, stacked on a narrow one.
func detailColumns(device *model.AmdgpuMetrics, in LayoutInput, th Theme, lines, width int) []Row {
	narrow := width < NarrowWidth
	graphW := width
	sensorW := width
	if !narrow {
		graphW = width / 2
		sensorW = width - graphW - 1
	}

	graphs := graphBlock(device, in, th, lines, graphW)
	if narrow {
		used := len(graphs)
		sensors := sensorBlock(device, th, max(0, lines-used), sensorW)
		return append(graphs, sensors...)
	}
	return joinColumns(graphs, sensorBlock(device, th, lines, sensorW), graphW, sensorW, 1)
}

// graphBlock is the utilisation graph with its readout, and the memory
// graphs below them once the terminal is tall enough to draw them.
func graphBlock(device *model.AmdgpuMetrics, in LayoutInput, th Theme, lines, width int) []Row {
	out := []Row{titleRow("GPU busy", th)}
	readout := 1
	if device != nil {
		grad := NewGradient(th, false)
		readout = len(UtilisationRows(*device, th, width, &grad))
	}
	memory := in.Height >= MinHeightForMemoryGraphs
	overhead := 1 + readout // graph title + readout rows
	if memory {
		overhead += 2 // VRAM and GTT titles
	}
	graphCells := max(2, lines-overhead)
	utilH := max(1, graphCells*2/3)
	memH := max(1, graphCells-utilH)

	out = append(out, graphRows(busySeries(in.History, deviceIndex(device)), utilH, width)...)
	if device != nil {
		grad := NewGradient(th, false)
		for _, r := range UtilisationRows(*device, th, width, &grad) {
			out = append(out, fitRow(r, width))
		}
	}
	if memory {
		half := (width - 1) / 2
		out = append(out, titleRow("VRAM", th))
		out = append(out, graphRows(poolSeries(in.History, deviceIndex(device), func(m model.AmdgpuMetrics) model.MemoryPool { return m.VRAM }), memH, half)...)
		out = append(out, titleRow("GTT", th))
		out = append(out, graphRows(poolSeries(in.History, deviceIndex(device), func(m model.AmdgpuMetrics) model.MemoryPool { return m.GTT }), memH, width-half-1)...)
	}
	return out
}

// sensorBlock is the sensors title plus its rows, truncated to the space
// it was given: on a short terminal the bottom sensors scroll away rather
// than pushing the graphs off the screen.
func sensorBlock(device *model.AmdgpuMetrics, th Theme, lines, width int) []Row {
	out := []Row{titleRow("Sensors", th)}
	if device == nil || lines <= 1 {
		return out
	}
	grad := NewGradient(th, false)
	rows := SensorsPanel(*device, th, width, &grad)
	for _, r := range rows {
		if len(out) >= lines {
			break
		}
		out = append(out, fitRow(r, width))
	}
	return out
}

// processBlock is the process panel title plus the table itself.
func processBlock(in LayoutInput, th Theme, lines, width int) []Row {
	rows := BuildRows(in.Snapshot.Processes, in.View.Sort, in.View.Filter, in.View.Tree)
	top := BusiestProcesses(in.Snapshot.Processes, in.View.Sort)
	title := PanelTitle(in.View.Sort, in.View.Filter, len(rows), in.Snapshot.TotalProcessCount, top, in.View.Tree)
	out := []Row{fitRow(titleRow(title, th), width)}
	if lines <= 1 {
		return out
	}
	sel := in.View.Selected
	table := RenderTable(rows, in.View.Sort, TableConfig{
		Width: width, FullCommand: in.View.FullCommand, Selected: sel, Top: top, Theme: th,
	})
	for _, r := range table {
		if len(out) >= lines {
			break
		}
		out = append(out, r)
	}
	return out
}

// statusLine is the diagnostics line above the key hints.
//
// The alert notice goes last, after the facts: a crossed threshold is the
// one thing on this line the user has to act on, so it is the thing the
// width is taken from last when the terminal is narrow.
func statusLine(in LayoutInput, th Theme) Row {
	visible, total := in.Snapshot.VisibleProcessCount, in.Snapshot.TotalProcessCount
	row := FooterPanel(th, FooterInfo{
		Sequence: in.Snapshot.Sequence, MetricsSource: in.Snapshot.Source.MetricsABI,
		TickMs: in.IntervalS * 1000, CPU: NA,
		Visible: &visible, Total: &total,
		Warnings: in.Snapshot.Warnings, BlocksStatus: in.Snapshot.BlocksStatus, BlocksHint: in.Snapshot.BlocksHint,
	})
	if notice := AlertNotice(in.Alerts); notice != "" {
		row.Cells = append(row.Cells,
			Cell{Text: "  ·  ", Color: th.Track},
			Cell{Text: notice, Color: th.Warn},
		)
	}
	return row
}

// keyHints is the fixed one-line key reference at the bottom. It is a
// curated subset, not the full table: the full reference is one key away
// behind `h`, and a hint line that wraps is no longer a hint line.
func keyHints(th Theme, width int) Row {
	hint := "q quit  1-9/tab GPU  o overview  m theme  +- interval  p procs  / filter  s sort  h help"
	return Row{Cells: []Cell{{Text: cutRunes(hint, width), Color: th.Muted}}}
}

// titleRow is a panel title line.
func titleRow(text string, th Theme) Row {
	return Row{Cells: []Cell{{Text: text, Color: th.Accent + "|bold"}}}
}

// graphRows rasterises a series into height lines of braille text.
func graphRows(samples []*float64, height, width int) []Row {
	if width <= 0 || height <= 0 {
		return nil
	}
	grad := Gradient{}
	lines := strings.Split(BrailleText(samples, width, height, grad, "", ScalePercent), "\n")
	out := make([]Row, 0, len(lines))
	for _, line := range lines {
		out = append(out, Row{Cells: []Cell{{Text: line}}})
	}
	return out
}

// joinColumns places two row blocks side by side, padding the shorter one.
func joinColumns(left, right []Row, leftW, rightW, gap int) []Row {
	n := max(len(left), len(right))
	out := make([]Row, 0, n)
	fill := Row{Cells: []Cell{{Text: strings.Repeat(" ", leftW)}}}
	gapCells := []Cell{{Text: strings.Repeat(" ", gap)}}
	for i := range n {
		l := fill
		if i < len(left) {
			l = padRowTo(fitRow(left[i], leftW), leftW)
		}
		var r Row
		if i < len(right) {
			r = fitRow(right[i], rightW)
		}
		cells := append(append(append([]Cell{}, l.Cells...), gapCells...), r.Cells...)
		out = append(out, Row{Cells: cells})
	}
	return out
}

// SplitRows turns embedded newlines into separate rows (the header draws
// three logical lines as one Row).
func SplitRows(r Row) []Row {
	var out []Row
	var current Row
	for _, c := range r.Cells {
		parts := strings.Split(c.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				out = append(out, current)
				current = Row{}
			}
			if part != "" {
				current.Cells = append(current.Cells, Cell{Text: part, Color: c.Color})
			}
		}
	}
	out = append(out, current)
	return out
}

// focusedDevice is the device the view is focused on, falling back to the
// first one: a stale focus index must not blank the screen.
func focusedDevice(snap model.GpuSnapshot, index int) *model.AmdgpuMetrics {
	if len(snap.Devices) == 0 {
		return nil
	}
	if m := snap.DeviceByIndex(index); m != nil {
		return m
	}
	return &snap.Devices[0]
}

func deviceIndex(m *model.AmdgpuMetrics) int {
	if m == nil {
		return -1
	}
	return m.Device.Index
}

// busySeries is the GPU busy percentage per retained sample.
func busySeries(history []model.GpuSnapshot, index int) []*float64 {
	out := make([]*float64, 0, len(history))
	for _, snap := range history {
		m := snap.DeviceByIndex(index)
		if m == nil || m.GpuBusyPercent == nil {
			out = append(out, nil)
			continue
		}
		v := float64(*m.GpuBusyPercent)
		out = append(out, &v)
	}
	return out
}

// poolSeries is one memory pool's fill percentage per retained sample.
func poolSeries(history []model.GpuSnapshot, index int, pick func(model.AmdgpuMetrics) model.MemoryPool) []*float64 {
	out := make([]*float64, 0, len(history))
	for _, snap := range history {
		m := snap.DeviceByIndex(index)
		if m == nil {
			out = append(out, nil)
			continue
		}
		if p := pick(*m).Percent(); p != nil {
			v := float64(*p)
			out = append(out, &v)
			continue
		}
		out = append(out, nil)
	}
	return out
}

// padRowTo grows a row to exactly w cells with trailing spaces, so a short
// left column still separates from the one beside it.
func padRowTo(r Row, w int) Row {
	n := 0
	for _, c := range r.Cells {
		n += len([]rune(c.Text))
	}
	if n < w {
		r.Cells = append(r.Cells, Cell{Text: strings.Repeat(" ", w-n)})
	}
	return r
}
