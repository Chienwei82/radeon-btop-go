package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

// blockSample builds a sample with every pipeline block present, every
// auxiliary block and both clocks. Percentages are distinct per block so a
// test can assert on a named block rather than on a position.
func blockSample() model.GpuBlocks {
	blocks := make([]model.BlockReading, 0, len(model.BlockOrder))
	for i, key := range model.BlockOrder {
		pct := model.Percent(i * 5)
		if key == "gpu" {
			pct = 45
		}
		if key == "cb" {
			pct = 65
		}
		blocks = append(blocks, model.BlockReading{Key: key, Label: model.BlockLabel(key), Percent: pct})
	}
	aux := make([]model.BlockReading, 0, len(model.AuxBlockOrder))
	for _, key := range model.AuxBlockOrder {
		aux = append(aux, model.BlockReading{Key: key, Label: model.BlockLabel(key), Percent: 1})
	}
	mclk, sclk := model.Mhz(1000), model.Mhz(2104)
	p50, p25 := model.Percent(50), model.Percent(25)
	return model.GpuBlocks{
		Blocks:    blocks,
		AuxBlocks: aux,
		Clocks: []model.ClockReading{
			{Key: "mclk", Label: "Memory Clock", Percent: &p50, Mhz: &mclk},
			{Key: "sclk", Label: "Shader Clock", Percent: &p25, Mhz: &sclk},
		},
	}
}

func blocksText(rows []Row) []string { return PlainLines(rows) }

func TestBlocksPanelRendersARowPerBlock(t *testing.T) {
	b := blockSample()
	rows := BlocksPanel(&b, DefaultTheme, 44, nil)
	want := len(model.BlockOrder) + len(model.AuxBlockOrder) + 2
	if len(rows) != want {
		t.Fatalf("BlocksPanel rows = %d, want %d", len(rows), want)
	}
}

func TestBlocksPanelEveryRowFitsTheAvailableWidth(t *testing.T) {
	// 40 is the narrowest width the layout hands these panels; a bar never
	// goes below its floor of six cells, and below that the layout's own
	// crop takes over (the reference relies on the widget crop there).
	b := blockSample()
	for _, width := range []int{40, 44, 58} {
		for _, row := range blocksText(BlocksPanel(&b, DefaultTheme, width, nil)) {
			if n := runeLen(row); n > width {
				t.Fatalf("width %d: row is %d cells: %q", width, n, row)
			}
		}
	}
}

func TestBlocksPanelIdleBlockReadsZeroNotNA(t *testing.T) {
	b := model.GpuBlocks{Blocks: []model.BlockReading{{Key: "gpu", Label: "Graphics pipe", Percent: 0}}}
	rows := blocksText(BlocksPanel(&b, DefaultTheme, 44, nil))
	if len(rows) != 1 || !strings.Contains(rows[0], "0%") {
		t.Fatalf("rows = %q, want one row reading 0%%", rows)
	}
	if strings.Contains(rows[0], NA) {
		t.Fatalf("row = %q, a measured zero must not render as N/A", rows[0])
	}
}

func TestBlocksPanelAbsentBlockGetsNoRow(t *testing.T) {
	b := model.GpuBlocks{Blocks: []model.BlockReading{{Key: "gpu", Label: "Graphics pipe", Percent: 9}}}
	for _, row := range blocksText(BlocksPanel(&b, DefaultTheme, 44, nil)) {
		if strings.Contains(row, "Texture Cache") {
			t.Fatalf("absent block still has a row: %q", row)
		}
	}
}

func TestBlocksPanelClocksShowAFrequency(t *testing.T) {
	b := blockSample()
	rows := blocksText(BlocksPanel(&b, DefaultTheme, 44, nil))
	if len(rows) < 2 {
		t.Fatalf("rows = %q, want the clock rows", rows)
	}
	if !strings.Contains(rows[len(rows)-1], "2,104 MHz") {
		t.Fatalf("last row = %q, want the shader clock frequency", rows[len(rows)-1])
	}
	if !strings.Contains(rows[len(rows)-2], "Memory Clock") {
		t.Fatalf("row before it = %q, want the memory clock", rows[len(rows)-2])
	}
}

func TestBlocksPanelClockWithoutFrequencyDegradesToPercentage(t *testing.T) {
	p := model.Percent(25)
	b := model.GpuBlocks{Clocks: []model.ClockReading{{Key: "sclk", Label: "Shader Clock", Percent: &p}}}
	rows := blocksText(BlocksPanel(&b, DefaultTheme, 44, nil))
	if len(rows) != 1 {
		t.Fatalf("rows = %q, want one clock row", rows)
	}
	if !strings.Contains(rows[0], "25%") || strings.Contains(rows[0], NA) {
		t.Fatalf("row = %q, want 25%% and no N/A beside a filled bar", rows[0])
	}
}

func TestBlocksPanelNoSampleYieldsOneExplanatoryLine(t *testing.T) {
	rows := blocksText(BlocksPanel(nil, DefaultTheme, 44, nil))
	if len(rows) != 1 || !strings.Contains(rows[0], "waiting") {
		t.Fatalf("rows = %q, want one line explaining the wait", rows)
	}
	if strings.Contains(rows[0], NA) {
		t.Fatalf("row = %q, must not be a column of N/A", rows[0])
	}
}

func TestBlocksPanelEmptySampleSaysSo(t *testing.T) {
	var empty model.GpuBlocks
	rows := blocksText(BlocksPanel(&empty, DefaultTheme, 44, nil))
	if len(rows) != 1 || !strings.Contains(rows[0], "no blocks") {
		t.Fatalf("rows = %q, want one line saying there are no blocks", rows)
	}
}

func TestBlocksPanelGroupOrderIsPipelineThenAuxThenClocks(t *testing.T) {
	b := blockSample()
	rows := blocksText(BlocksPanel(&b, DefaultTheme, 44, nil))
	if !strings.Contains(rows[0], "Graphics pipe") {
		t.Fatalf("first row = %q, want the graphics pipe", rows[0])
	}
	if !strings.Contains(rows[len(model.BlockOrder)], "UVD") {
		t.Fatalf("row %d = %q, want UVD", len(model.BlockOrder), rows[len(model.BlockOrder)])
	}
	if !strings.Contains(rows[len(rows)-2], "Memory Clock") {
		t.Fatalf("penultimate row = %q, want the memory clock", rows[len(rows)-2])
	}
}

func TestBlocksHeadingNamesTheBusiestBlock(t *testing.T) {
	b := blockSample()
	heading := BlocksHeading(&b)
	for _, want := range []string{"Blocks", "Color Block", "%"} {
		if !strings.Contains(heading, want) {
			t.Fatalf("heading = %q, want %q", heading, want)
		}
	}
}

func TestBlocksHeadingIsNeutralWithoutASample(t *testing.T) {
	if got := BlocksHeading(nil); got != "Blocks" {
		t.Fatalf("BlocksHeading(nil) = %q, want Blocks", got)
	}
	var empty model.GpuBlocks
	if got := BlocksHeading(&empty); got != "Blocks" {
		t.Fatalf("BlocksHeading(empty) = %q, want Blocks", got)
	}
}

func TestBlockLabelWidthLeavesAGutterBeforeTheBar(t *testing.T) {
	longest := 0
	for _, key := range model.BlockOrder {
		if n := runeLen(model.BlockLabel(key)); n > longest {
			longest = n
		}
	}
	if longest+1 != model.BlockLabelWidth {
		t.Fatalf("longest label %d + 1 != BlockLabelWidth %d", longest, model.BlockLabelWidth)
	}
}
