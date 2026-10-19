package ui

import (
	"fmt"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// TabSeparator sits between tabs: a box-drawing character rather than a
// space so an idle tab is still a visible target to click.
const TabSeparator = "│"

// TabBarCells is the mini usage bar width inside a tab.
const TabBarCells = 4

// MinTabWidth is the smallest a tab may become before the name is
// truncated rather than the bar dropped.
const MinTabWidth = 18

// TabChrome is the cells a tab spends on everything that is neither the
// name, the kind, the percentage nor the badge number: the space either
// side of each of the three badges (6), the mini bar (4), the gap before
// the percentage (1) and the trailing active-marker cell (2). The badge's
// digits are deliberately not counted: their width depends on which device
// this is, which is what TabWidth adds.
const TabChrome = 13

// TabGap is the cells the separator costs, for every tab but the first.
const TabGap = 3

// DeviceBusy is the utilisation used for a tab's mini bar, preferring the
// busy metric. mem_busy is the fallback because on a card that reports
// neither, a memory-bound workload is still visibly doing something, and
// an empty bar next to a name would read as "this GPU is fine".
func DeviceBusy(m model.AmdgpuMetrics) *float64 {
	if m.GpuBusyPercent != nil {
		v := float64(*m.GpuBusyPercent)
		return &v
	}
	if m.MemBusyPercent != nil {
		v := float64(*m.MemBusyPercent)
		return &v
	}
	return nil
}

func miniBar(percent *float64, th Theme) Row {
	var cells []Cell
	if percent == nil {
		cells = append(cells, Cell{Text: strings.Repeat(BarEmpty, TabBarCells), Color: th.Track})
		return Row{Cells: cells}
	}
	filled := model.RoundHalfEven(min(100.0, max(0.0, *percent)) / 100.0 * TabBarCells)
	cells = append(cells, Cell{Text: strings.Repeat(BarFilled, filled), Color: th.Accent})
	cells = append(cells, Cell{Text: strings.Repeat(BarEmpty, TabBarCells-filled), Color: th.Track})
	return Row{Cells: cells}
}

// TabWidth measures how wide a tab is in cells: the label as drawn.
func TabWidth(devices []model.AmdgpuMetrics, position int) int {
	m := devices[position]
	plain := Plain(tabLabel(m, GetTheme("default"), m.Device.Index == 0, position+1))
	return max(MinTabWidth, len([]rune(plain)))
}

// TabSpan is one tab's drawn extent: the device position and its cell span.
type TabSpan struct {
	Position int
	Start    int
	Width    int
}

// TabSpans lays tabs out left to right with a gutter between them. The
// gutter cost belongs to the tab that follows it: a click on a gutter cell
// selects the right-hand device, which treats the tab as a word and the
// gutter as its leading space.
func TabSpans(devices []model.AmdgpuMetrics) []TabSpan {
	spans := make([]TabSpan, 0, len(devices))
	cursor := 0
	for i := range devices {
		if i > 0 {
			cursor += TabGap
		}
		w := TabWidth(devices, i)
		spans = append(spans, TabSpan{Position: i, Start: cursor, Width: w})
		cursor += w
	}
	return spans
}

// DeviceAt returns the device position covering cell x, or -1.
func DeviceAt(devices []model.AmdgpuMetrics, x int) int {
	if x < 0 {
		return -1
	}
	for _, s := range TabSpans(devices) {
		if x >= s.Start && x < s.Start+s.Width {
			return s.Position
		}
		// The gutter belongs to the tab that follows it.
		if s.Position > 0 && x >= s.Start-TabGap && x < s.Start {
			return s.Position
		}
	}
	return -1
}

func tabLabel(m model.AmdgpuMetrics, th Theme, active bool, number int) Row {
	var cells []Cell
	cells = append(cells, Cell{Text: "[", Color: th.Muted})
	badge := fmt.Sprintf(" %d ", number)
	if active {
		cells = append(cells, Cell{Text: badge, Color: th.Text + "|reverse"})
	} else {
		cells = append(cells, Cell{Text: badge, Color: th.Muted})
	}
	kind := "dGPU"
	if m.Device.IsIntegrated() {
		kind = "iGPU"
	}
	cells = append(cells, Cell{Text: "] " + m.Device.Name + " " + kind + " ", Color: th.Text})
	cells = append(cells, miniBar(DeviceBusy(m), th).Cells...)
	cells = append(cells, Cell{Text: " " + FmtPercent(floatPtr(DeviceBusy(m)), 0), Color: th.Muted})
	if active {
		cells = append(cells, Cell{Text: "◂", Color: th.Accent + "|bold"})
	} else {
		cells = append(cells, Cell{Text: " ", Color: ""})
	}
	return Row{Cells: cells}
}

func floatPtr(v *float64) *model.Percent {
	if v == nil {
		return nil
	}
	p := model.Percent(*v)
	return &p
}

// GPUTabs draws the device tab bar. Tabs are laid out left to right with a
// separator; a single device still gets one tab (the focused marker is the
// point, not the choice).
func GPUTabs(devices []model.AmdgpuMetrics, th Theme, index, available int) Row {
	var cells []Cell
	for i, m := range devices {
		if i > 0 {
			cells = append(cells, Cell{Text: " " + TabSeparator + " ", Color: th.Track})
		}
		cells = append(cells, tabLabel(m, th, m.Device.Index == index, i+1).Cells...)
	}
	return Row{Cells: cells}
}

// OverviewPanel draws one card per device: name, utilisation bar, memory
// and temperature. The focused card carries the marker.
func OverviewPanel(devices []model.AmdgpuMetrics, th Theme, available, index int, gradient *Gradient) []Row {
	var rows []Row
	for _, m := range devices {
		marker := "  "
		if m.Device.Index == index {
			marker = "▶ "
		}
		rows = append(rows, Row{Cells: []Cell{
			{Text: marker + m.Device.Name, Color: th.Text + "|bold"},
		}})
		var pct *float64
		var pctS string
		if b := DeviceBusy(m); b != nil {
			pct = b
			pctS = FmtPercent(floatPtr(b), 0)
		} else {
			pctS = NA
		}
		rows = append(rows, StatRow("busy", pctS, pct, th, available, gradient))
		rows = append(rows, PoolRow("VRAM", m.VRAM, th, available, gradient))
		rows = append(rows, PoolRow("GTT", m.GTT, th, available, gradient))
	}
	return rows
}
