package ui

import (
	"fmt"
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
	// FlashOn is the current phase of the alert flash. It only matters when
	// Alerts is active: the frame alternates between the alert colour and
	// the track colour, so a crossed threshold is visible at a glance.
	FlashOn bool
	// Notice is a transient message the event loop produced: a refused
	// signal, the theme it just switched to, a state save that failed. It is
	// part of the layout input rather than the app so the whole screen stays
	// one pure function.
	Notice string
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
	if in.Alerts.Active() {
		// The border is a transient seam: it belongs to this render, not to
		// the palette, so the theme itself never learns about the flash.
		th.Border = AlertBorderColor(in.Alerts, in.FlashOn, th)
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
		// The detail block may have used more rows than it was offered (the
		// graphs grow by a row at a time), so the table must yield rather than
		// push the status line and key hints off the bottom of the terminal.
		if available := height - len(out) - footerLines; tableLines > available {
			tableLines = max(0, available)
		}
		out = append(out, processBlock(in, th, tableLines, width)...)
	}

	// Keep the diagnostics and the key hints on the bottom edge. A short
	// overview (or a terminal whose panels need fewer rows than they were
	// offered) would otherwise leave them floating in the middle with the
	// rest of the alternate screen untouched above them.
	for len(out) < height-footerLines {
		out = append(out, Row{})
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
	hasProfile := device.Profiles.Present || device.ODC.Present
	if device.Blocks == nil && !hasProfile {
		return nil
	}
	bodyLines := optionalRowLines - boxOverheadY

	// A single panel still fills the row: a framed box beside an empty
	// column would look like a rendering bug.
	if device.Blocks == nil {
		body := padBody(crop(PowerProfilePanel(*device, th, width-boxOverheadX, nil), bodyLines, width-boxOverheadX), bodyLines)
		return PanelBox("Power profile", body, width, th)
	}
	if !hasProfile {
		body := padBody(crop(BlocksPanel(device.Blocks, th, width-boxOverheadX, nil), bodyLines, width-boxOverheadX), bodyLines)
		return PanelBox(BlocksHeading(device.Blocks), body, width, th)
	}
	if width < 2*(MinOptionalPanelWidth+boxOverheadX)+1 {
		// Too narrow for two framed panels: a split would put each bar below
		// the length at which it says anything. Blocks wins when both exist;
		// the active profile is already named on the sensors panel, whereas
		// per-block utilisation appears nowhere else.
		body := padBody(crop(BlocksPanel(device.Blocks, th, width-boxOverheadX, nil), bodyLines, width-boxOverheadX), bodyLines)
		return PanelBox(BlocksHeading(device.Blocks), body, width, th)
	}
	left := (width - 1) / 2
	right := width - left - 1
	leftBody := padBody(crop(BlocksPanel(device.Blocks, th, left-boxOverheadX, nil), bodyLines, left-boxOverheadX), bodyLines)
	rightBody := padBody(crop(PowerProfilePanel(*device, th, right-boxOverheadX, nil), bodyLines, right-boxOverheadX), bodyLines)
	leftBox := PanelBox(BlocksHeading(device.Blocks), leftBody, left, th)
	rightBox := PanelBox("Power profile", rightBody, right, th)
	return joinColumns(leftBox, rightBox, left, right, 1)
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

// padBody makes a panel body exactly lines rows: short bodies get blank rows,
// long ones are cut. Two frames sharing a row must be the same height, or the
// shorter one's bottom border floats above its sibling's and the division
// stops looking deliberate.
func padBody(rows []Row, lines int) []Row {
	if lines <= 0 {
		return nil
	}
	if len(rows) > lines {
		return rows[:lines]
	}
	for len(rows) < lines {
		rows = append(rows, Row{})
	}
	return rows
}

// detailColumns is the graphs column plus the sensors column: side by side
// on a wide terminal, stacked on a narrow one. Both are framed, so the split
// between them is a drawn boundary rather than a guess about spacing.
//
// The wide split is the reference's 2:1, with the sensor column held at no
// less than forty columns: a run of "label bar value" rows needs that much
// before the bar is too short to read, and the graph can spend the rest.
func detailColumns(device *model.AmdgpuMetrics, in LayoutInput, th Theme, lines, width int) []Row {
	if width < NarrowWidth {
		graphLines := lines
		if lines >= 12 {
			// Give the sensors a real box instead of the title-only strip
			// the old stack degenerated into; two thirds keeps the series
			// legible while the sensors still get a body.
			graphLines = min(lines, max(8, lines*2/3))
		}
		graph := PanelBox("GPU busy", graphBlock(device, in, th, graphLines-boxOverheadY, width-boxOverheadX), width, th)
		remaining := lines - len(graph)
		if remaining < boxOverheadY+1 {
			return graph
		}
		sensors := PanelBox("Sensors", sensorBlock(device, th, remaining-boxOverheadY, width-boxOverheadX), width, th)
		return append(graph, sensors...)
	}

	sensorW := max(40, width/3)
	graphW := width - sensorW - 1
	graph := PanelBox("GPU busy", graphBlock(device, in, th, lines-boxOverheadY, graphW-boxOverheadX), graphW, th)
	sensors := PanelBox("Sensors", sensorBlock(device, th, lines-boxOverheadY, sensorW-boxOverheadX), sensorW, th)
	return joinColumns(graph, sensors, graphW, sensorW, 1)
}

// graphBlock is the utilisation graph with its readout, and the memory
// graphs below them once the terminal is tall and wide enough to draw them.
//
// lines is the space inside the GPU-busy frame. The memory pair is two framed
// panels on one row, not two half-width graphs stacked: stacking them spent a
// row per graph on a title and left the right half of the screen empty.
func graphBlock(device *model.AmdgpuMetrics, in LayoutInput, th Theme, lines, width int) []Row {
	if lines <= 0 {
		return nil
	}
	// Build the readout once: it used to be built twice, first with a nil
	// gradient just to count its rows and then again to paint them, which
	// rebuilt the gradient ramp for every device on every frame.
	var utilisation []Row
	if device != nil {
		grad := NewGradient(th, false)
		utilisation = UtilisationRows(*device, th, width, &grad)
	}
	readout := 1
	if len(utilisation) > 0 {
		readout = min(lines, len(utilisation))
	}
	memory := in.Height >= MinHeightForMemoryGraphs && width >= 40 && lines-readout >= 4
	overhead := readout
	if memory {
		overhead += boxOverheadY
	}
	graphCells := max(1, lines-overhead)
	utilH := max(1, graphCells*2/3)
	memH := max(1, graphCells-utilH)

	out := graphRows(busySeries(in.History, deviceIndex(device)), utilH, width)
	for _, r := range utilisation {
		out = append(out, fitRow(r, width))
	}
	if memory {
		half := (width - 1) / 2
		other := width - half - 1
		vram := PanelBox("VRAM", graphRows(poolSeries(in.History, deviceIndex(device), func(m model.AmdgpuMetrics) model.MemoryPool { return m.VRAM }), memH, half-boxOverheadX), half, th)
		gtt := PanelBox("GTT", graphRows(poolSeries(in.History, deviceIndex(device), func(m model.AmdgpuMetrics) model.MemoryPool { return m.GTT }), memH, other-boxOverheadX), other, th)
		out = append(out, joinColumns(vram, gtt, half, other, 1)...)
	}

	// The frame above expects exactly the space it was offered: a short body
	// would put the bottom border too high, and a long one would push the
	// process table off the screen.
	return padBody(crop(out, lines, width), lines)
}

// sensorBlock is the sensors body, truncated to the space it was given: on a
// short terminal the bottom sensors scroll away rather than pushing the
// graphs off the screen. A truncation costs one row to say "… N more": a
// panel that silently drops half its readings looks like a card that stopped
// reporting them. The frame and title are added by the caller, so a short
// body is padded with blank rows and the bottom border still lines up with
// the graph panel beside it.
func sensorBlock(device *model.AmdgpuMetrics, th Theme, lines, width int) []Row {
	if lines <= 0 {
		return nil
	}
	out := make([]Row, 0, lines)
	if device != nil {
		grad := NewGradient(th, false)
		sensors := SensorsPanel(*device, th, width, &grad)
		hidden := 0
		if len(sensors) > lines {
			hidden = len(sensors) - lines + 1
			keep := lines - 1
			if keep < 0 {
				keep = 0
			}
			sensors = sensors[:keep]
		}
		for _, r := range sensors {
			out = append(out, fitRow(r, width))
		}
		if hidden > 0 {
			marker := Row{Cells: []Cell{{Text: fmt.Sprintf("… %d more", hidden), Color: th.Muted}}}
			out = append(out, fitRow(marker, width))
		}
	}
	for len(out) < lines {
		out = append(out, fitRow(Row{}, width))
	}
	return out
}

// processBlock is the framed process panel: the sort/filter line, the column
// heading and the table itself.
func processBlock(in LayoutInput, th Theme, lines, width int) []Row {
	if lines <= 0 {
		return nil
	}
	contentWidth := width - boxOverheadX
	rows := BuildRows(in.Snapshot.Processes, in.View.Sort, in.View.Filter, in.View.Tree)
	top := BusiestProcesses(in.Snapshot.Processes, in.View.Sort)
	title := PanelTitle(in.View.Sort, in.View.Filter, len(rows), in.Snapshot.TotalProcessCount, top, in.View.Tree)

	body := make([]Row, 0, max(0, lines-boxOverheadY))
	if lines > boxOverheadY {
		body = append(body, Row{Cells: []Cell{{Text: CutWidth(title, contentWidth), Color: th.Muted}}})
	}
	if len(body) < lines-boxOverheadY {
		sel := in.View.Selected
		table := RenderTable(rows, in.View.Sort, TableConfig{
			Width: contentWidth, FullCommand: in.View.FullCommand, Selected: sel, Top: top, Theme: th,
		})
		for _, r := range table {
			if len(body) >= lines-boxOverheadY {
				break
			}
			body = append(body, r)
		}
	}
	// Fill the frame it was given even when the table has fewer rows than the
	// budget: a short box would leave the status line floating above the
	// bottom of the terminal.
	body = padBody(body, lines-boxOverheadY)
	return crop(PanelBox("Processes", body, width, th), lines, width)
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
	if in.Notice != "" && in.Notice != AlertNotice(in.Alerts) {
		row.Cells = append(row.Cells,
			Cell{Text: "  ·  ", Color: th.Track},
			Cell{Text: in.Notice, Color: th.Muted},
		)
	}
	return row
}

// footerHints is the curated key reference shown at the bottom of the
// screen: the handful a user reaches for, in reading order. The full list is
// one key away behind h, and a hint line that wraps is no longer a hint line.
var footerHints = []struct {
	Key   string
	Label string
}{
	{"q", "quit"},
	{"1-9/tab", "GPU"},
	{"o", "overview"},
	{"m", "theme"},
	{"+-", "interval"},
	{"p", "procs"},
	{"/", "filter"},
	{"s", "sort"},
	{"h", "help"},
}

// keyHints is the fixed one-line key reference at the bottom.
//
// Keys and labels are separate runs: the key gets the accent and bold, the
// label stays muted. That is what makes the line scannable as keys rather
// than as a sentence, and it survives --no-color as a readable "q quit"
// because the separation is in the text, not only in the colour.
func keyHints(th Theme, width int) Row {
	cells := make([]Cell, 0, len(footerHints)*3)
	used := 0
	for i, h := range footerHints {
		group := DisplayWidth(h.Key) + 1 + DisplayWidth(h.Label)
		sep := 0
		if i > 0 {
			sep = 3 // " · "
		}
		// Stop at a group boundary: half a key ("1-9/ta") is worse than a
		// shorter hint line, because it turns a target into a typo.
		if used+sep+group > width {
			break
		}
		if sep > 0 {
			cells = append(cells, Cell{Text: " · ", Color: th.Track})
			used += sep
		}
		cells = append(cells, Cell{Text: h.Key, Color: th.Accent + "|bold"}, Cell{Text: " " + h.Label, Color: th.Muted})
		used += group
	}
	if len(cells) == 0 {
		return fitRow(Row{Cells: []Cell{{Text: CutWidth("q quit", width), Color: th.Muted}}}, width)
	}
	return fitRow(Row{Cells: cells}, width)
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
		n += DisplayWidth(c.Text)
	}
	if n < w {
		r.Cells = append(r.Cells, Cell{Text: strings.Repeat(" ", w-n)})
	}
	return r
}
