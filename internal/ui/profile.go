package ui

import (
	"fmt"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// ODPrefix marks an overdrive row, so a reader (and a test) can tell the
// table rows from the profile rows at a glance.
const ODPrefix = "OD "

// PowerProfilePanel builds the read-only power-profile panel: which SCPP
// profile is active, which alternatives the card offers, and the overdrive
// table with its own units.
//
// Nothing here writes. The panel exists so a user can see what the card is
// doing without a second tool, and the closing line says where the change
// has to be made, because this app will not make it.
func PowerProfilePanel(m model.AmdgpuMetrics, th Theme, available int, gradient *Gradient) []Row {
	rows := []Row{detailText("profile "+m.Profiles.Describe(), th.Text, available)}
	rows = append(rows, profileAlternatives(m, th, available)...)
	rows = append(rows, overdriveRows(m, th, available)...)
	rows = append(rows, detailText("read-only (change it with the vendor tool)", th.Muted, available))
	return rows
}

// profileAlternatives lists the profiles the card offers, or N/A.
func profileAlternatives(m model.AmdgpuMetrics, th Theme, available int) []Row {
	names := m.Profiles.Names()
	if len(names) == 0 {
		return []Row{detailText("profiles "+NA, th.Muted, available)}
	}
	return []Row{detailText("available "+strings.Join(names, " · "), th.Muted, available)}
}

// overdriveRows renders the pp_od_clk_voltage table. A legacy table keeps
// its millivolt unit rather than inheriting the modern one.
func overdriveRows(m model.AmdgpuMetrics, th Theme, available int) []Row {
	odc := m.ODC
	if len(odc.Domains) == 0 && odc.VddgfxOffsetMv == nil {
		return []Row{detailText("overdrive "+NA, th.Muted, available)}
	}
	rows := make([]Row, 0, len(odc.Domains)+1)
	for _, d := range odc.Domains {
		rows = append(rows, detailText(ODPrefix+d.Label()+" "+d.Describe(), th.Muted, available))
	}
	if odc.VddgfxOffsetMv != nil {
		rows = append(rows, detailText(ODPrefix+fmt.Sprintf("vddgfx %+d mV", *odc.VddgfxOffsetMv), th.Muted, available))
	}
	return rows
}

// detailText is a plain, trimmed row: a row that renders wider than the
// panel lays out would draw over the panel's own border.
func detailText(text, color string, available int) Row {
	return Row{Cells: []Cell{{Text: CutWidth(text, available), Color: color}}}
}
