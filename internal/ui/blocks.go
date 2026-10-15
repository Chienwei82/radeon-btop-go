package ui

import (
	"fmt"

	"github.com/gputop/gputop-go/internal/model"
)

// BlocksPanel builds the per-block utilisation panel.
//
// Every block is a bar on a 0-100 scale and the groups run together without
// separators: the model has already grouped them by pipeline stage, and a
// blank row between groups costs a line of a terminal that may not have one
// to spare.
//
// A block at exactly zero still draws an empty bar with a 0% label. That is
// not the same as a missing block: radeontop omits a block the hardware has
// no unit for, so every row here describes something that exists.
//
// A nil sample renders one explanatory line rather than a column of empty
// bars, because fourteen N/A labels answer nothing.
func BlocksPanel(blocks *model.GpuBlocks, th Theme, available int, gradient *Gradient) []Row {
	if blocks == nil {
		return []Row{{Cells: []Cell{{Text: cutRunes("waiting for radeontop", available), Color: th.Muted}}}}
	}
	rows := []Row{}
	for _, group := range blocks.DisplayRows() {
		for _, reading := range group {
			rows = append(rows, blockRow(reading, th, available, gradient))
		}
	}
	if len(rows) == 0 {
		return []Row{{Cells: []Cell{{Text: cutRunes("no blocks reported", available), Color: th.Muted}}}}
	}
	return rows
}

// blockRow is one block's line: a clock prints the frequency being read
// rather than a bare percentage, and a clock whose frequency radeontop did
// not report falls back to the percentage rather than to N/A -- printing N/A
// beside a bar that is plainly filled contradicts itself.
func blockRow(reading model.DisplayReading, th Theme, available int, gradient *Gradient) Row {
	percent := float64(reading.Percent)
	value := FmtPercent(&percent, 0)
	if reading.Mhz != nil {
		mhz := float64(*reading.Mhz)
		value = Fmt(&mhz, "", 0) + " MHz"
	}
	return StatRowWidth(reading.Label, value, &percent, th, available, gradient, model.BlockLabelWidth)
}

// BlocksHeading is the panel title, which answers "which block".
//
// It names the busiest block and its share, or stays neutral when there is
// nothing to report: a heading that names a block on a sample that has none
// is worse than a heading that says nothing.
func BlocksHeading(blocks *model.GpuBlocks) string {
	if blocks == nil || blocks.IsEmpty() {
		return "Blocks"
	}
	if busiest := blocks.Busiest(); busiest != nil {
		return fmt.Sprintf("Blocks · %s %s", busiest.Label, FmtPercent(&busiest.Percent, 0))
	}
	return "Blocks"
}
