package model

import "testing"

// blockSample is a complete per-block sample: every pipeline block, every
// auxiliary block and both clocks, with distinct percentages so an
// assertion can name a block instead of a position.
func blockSample() GpuBlocks {
	blocks := make([]BlockReading, 0, len(BlockOrder))
	for i, key := range BlockOrder {
		blocks = append(blocks, BlockReading{Key: key, Label: BlockLabel(key), Percent: Percent(i * 5)})
	}
	aux := make([]BlockReading, 0, len(AuxBlockOrder))
	for _, key := range AuxBlockOrder {
		aux = append(aux, BlockReading{Key: key, Label: BlockLabel(key), Percent: 1})
	}
	return GpuBlocks{
		Blocks:    blocks,
		AuxBlocks: aux,
		Clocks: []ClockReading{
			{Key: "mclk", Label: "Memory Clock", Percent: ptr[Percent](50), Mhz: ptr[Mhz](1000)},
			{Key: "sclk", Label: "Shader Clock", Percent: ptr[Percent](25), Mhz: ptr[Mhz](2104)},
		},
	}
}

func TestDisplayRowsGroupsPipelineThenAuxThenClocks(t *testing.T) {
	groups := blockSample().DisplayRows()
	if len(groups) != 3 {
		t.Fatalf("DisplayRows() = %d groups, want 3", len(groups))
	}
	if groups[0][0].Key != BlockOrder[0] {
		t.Fatalf("first group starts with %q, want %q", groups[0][0].Key, BlockOrder[0])
	}
	if len(groups[0]) != len(BlockOrder) {
		t.Fatalf("pipeline group = %d rows, want %d", len(groups[0]), len(BlockOrder))
	}
	if groups[1][0].Key != AuxBlockOrder[0] {
		t.Fatalf("aux group starts with %q, want %q", groups[1][0].Key, AuxBlockOrder[0])
	}
	if groups[2][0].Key != "mclk" || !groups[2][0].IsClock() {
		t.Fatalf("clock group starts with %+v, want the mclk clock", groups[2][0])
	}
}

func TestDisplayRowsOmitsEmptyGroups(t *testing.T) {
	only := GpuBlocks{Blocks: []BlockReading{{Key: "gpu", Label: "Graphics pipe", Percent: 9}}}
	if got := only.DisplayRows(); len(got) != 1 {
		t.Fatalf("blocks-only sample = %d groups, want 1", len(got))
	}
	if got := (GpuBlocks{}).DisplayRows(); len(got) != 0 {
		t.Fatalf("empty sample = %d groups, want 0", len(got))
	}
}

func TestDisplayRowsKeepsAClockWithoutAFrequency(t *testing.T) {
	b := GpuBlocks{Clocks: []ClockReading{{Key: "sclk", Label: "Shader Clock", Percent: ptr[Percent](25)}}}
	groups := b.DisplayRows()
	if len(groups) != 1 || len(groups[0]) != 1 {
		t.Fatalf("DisplayRows() = %v, want one group of one", groups)
	}
	if groups[0][0].IsClock() {
		t.Fatal("a clock with no frequency must not claim to carry one")
	}
}
