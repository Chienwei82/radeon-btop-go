package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// MaxTempC is the fixed temperature scale: every AMD part throttles hard
// at 125 C, so temperatures are bars on a 0-125 span, not on an
// arbitrary range.
const MaxTempC = 125.0

// UtilisationRows builds the live utilisation rows for the focused device.
// On an integrated GPU the working set is shared system memory, so GTT
// leads; VRAM is still listed, demoted below the primary pool.
func UtilisationRows(m model.AmdgpuMetrics, th Theme, available int, gradient *Gradient) []Row {
	var pct *float64
	if m.GpuBusyPercent != nil {
		v := float64(*m.GpuBusyPercent)
		pct = &v
	}
	var pctS string
	if pct != nil {
		pctS = FmtPercent(m.GpuBusyPercent, 0)
	} else {
		pctS = NA
	}
	rows := []Row{StatRow("GPU", pctS, pct, th, available, gradient)}
	if m.Device.IsIntegrated() {
		rows = append(rows, PoolRow("GTT", m.GTT, th, available, gradient))
		rows = append(rows, PoolRow("VRAM", m.VRAM, th, available, gradient))
	} else {
		rows = append(rows, PoolRow("VRAM", m.VRAM, th, available, gradient))
	}
	return rows
}

func clockRow(label string, clock model.Clock, th Theme, available int, gradient *Gradient) Row {
	var value string
	if clock.Current == nil {
		value = NA
	} else {
		cur := float64(*clock.Current)
		value = fmt.Sprintf("%s MHz", Fmt(&cur, "", 0))
	}
	var pct *float64
	if p := clock.Percent(); p != nil {
		v := float64(*p)
		pct = &v
	}
	return StatRow(label, value, pct, th, available, gradient)
}

func detailRow(label, value, color string) Row {
	return Row{Cells: []Cell{
		{Text: strings.Repeat(" ", LabelWidth) + label + " " + value, Color: color},
	}}
}

// SensorsPanel builds the sensor rows: clocks, temperatures, power, fan
// and throttle. Every row is bar-or-N/A.
func SensorsPanel(m model.AmdgpuMetrics, th Theme, available int, gradient *Gradient) []Row {
	rows := []Row{clockRow("Core", m.SCLK, th, available, gradient)}
	if !m.Device.IsIntegrated() {
		rows = append(rows, clockRow("Memory", m.MCLK, th, available, gradient))
	}
	rows = append(rows, clockRow("SoC", m.SocClk, th, available, gradient))

	for _, temp := range m.Temperatures {
		var fraction *float64
		value := NA
		if temp.Celsius != nil {
			value = FmtTemp(temp.Celsius)
			v := float64(*temp.Celsius) / MaxTempC * 100.0
			fraction = &v
		}
		rows = append(rows, StatRow("temp "+temp.Label, value, fraction, th, available, gradient))
	}

	var drawW, capW *float64
	if m.Power.DrawW != nil {
		v := float64(*m.Power.DrawW)
		drawW = &v
	}
	if m.Power.CapW != nil {
		v := float64(*m.Power.CapW)
		capW = &v
	}
	var powerPct *float64
	if p := m.Power.Percent(); p != nil {
		v := float64(*p)
		powerPct = &v
	}
	powerValue := NA
	if drawW != nil {
		powerValue = fmt.Sprintf("%s W", Fmt(drawW, "", 1))
	}
	rows = append(rows, StatRow("Power", powerValue, powerPct, th, available, gradient))
	rows = append(rows, detailRow("cap", Fmt(capW, "", 0)+" W", th.Muted))

	if m.Device.SupportsFan() {
		var rpmS string
		if m.Fan.RPM == nil {
			rpmS = NA
		} else {
			v := float64(*m.Fan.RPM)
			rpmS = Fmt(&v, "", 0) + " rpm"
		}
		var fanPct *float64
		if p := m.Fan.Percent(); p != nil {
			v := float64(*p)
			fanPct = &v
		}
		rows = append(rows, StatRow("Fan", rpmS, fanPct, th, available, gradient))
	}

	if len(m.VoltagesMv) > 0 {
		parts := make([]string, 0, len(m.VoltagesMv))
		for _, v := range m.VoltagesMv {
			parts = append(parts, fmt.Sprintf("%s %dmV", v.Name, v.MV))
		}
		rows = append(rows, detailRow(strings.Join(parts, "  "), "", th.Muted))
	}

	if m.PCIE != nil {
		rows = append(rows, detailRow("PCIe "+m.PCIE.Describe(), "", th.Muted))
	}

	profile := NA
	if m.PowerProfile != nil {
		profile = *m.PowerProfile
	} else if m.PerformanceLevel != nil {
		profile = *m.PerformanceLevel
	}
	rows = append(rows, detailRow("profile "+profile, "", th.Muted))

	if m.Throttle == nil {
		rows = append(rows, detailRow("throttle "+NA, "", th.Muted))
	} else if m.Throttle.IsThrottling() {
		active := append([]string(nil), m.Throttle.Active...)
		sort.Strings(active)
		rows = append(rows, Row{Cells: []Cell{
			{Text: strings.Repeat(" ", LabelWidth) + "throttle " + strings.Join(active, ", "), Color: th.Alert},
		}})
	} else {
		rows = append(rows, Row{Cells: []Cell{
			{Text: strings.Repeat(" ", LabelWidth) + "throttle none", Color: th.OK},
		}})
	}
	return rows
}

// FooterInfo carries the status-line inputs.
type FooterInfo struct {
	Sequence      int
	MetricsSource string
	TickMs        float64
	CPU           string
	Visible       *int
	Total         *int
	Warnings      []string
	BlocksStatus  model.BlocksStatus
	BlocksHint    string
}

// FooterPanel builds the status line above the key hints.
//
// Each fact is its own run with the separator in the track colour, so a
// warning or an alert can be painted without repainting the whole line. The
// plain text is unchanged from the single-run version: the line is still
// "#7  ·  metrics sysfs  ·  tick 1000 ms  ·  cpu N/A".
//
// The blocks hint is filtered here rather than by the caller so that the
// policy -- explain a problem, but stay silent about a deliberate choice
// -- lives with the line that shows it.
func FooterPanel(th Theme, info FooterInfo) Row {
	cells := []Cell{{Text: fmt.Sprintf("#%d", info.Sequence), Color: th.Accent}}
	add := func(text, color string) {
		cells = append(cells,
			Cell{Text: "  ·  ", Color: th.Track},
			Cell{Text: text, Color: color},
		)
	}
	source := info.MetricsSource
	if source == "" {
		source = "sysfs"
	}
	add("metrics "+source, th.Text)
	add(fmt.Sprintf("tick %.0f ms", info.TickMs), th.Text)
	add("cpu "+info.CPU, th.Text)
	if info.Visible != nil && info.Total != nil && *info.Total > *info.Visible {
		add(fmt.Sprintf("partial %d/%d procs", *info.Visible, *info.Total), th.Warn)
	}
	if len(info.Warnings) > 0 {
		warning := info.Warnings[0]
		if len(info.Warnings) > 1 {
			warning += fmt.Sprintf(" (+%d more)", len(info.Warnings)-1)
		}
		add(warning, th.Warn)
	}
	if info.BlocksStatus != "" && info.BlocksStatus != model.BlocksOK && info.BlocksStatus != model.BlocksDisabled {
		hint := info.BlocksHint
		if hint == "" {
			hint = info.BlocksStatus.Hint()
		}
		add("blocks: "+hint, th.Muted)
	}
	return Row{Cells: cells}
}
