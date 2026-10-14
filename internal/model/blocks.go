package model

// BlockLabels maps radeontop dump keys to display labels (from ui.c).
var BlockLabels = map[string]string{
	"gpu":  "Graphics pipe",
	"ee":   "Event Engine",
	"vgt":  "Vertex Grouper",
	"ta":   "Texture Addr",
	"tc":   "Texture Cache",
	"sx":   "Shader Export",
	"sh":   "Seq Inst Cache",
	"spi":  "Shader Interp",
	"smx":  "Shader MemExch",
	"sc":   "Scan Converter",
	"pa":   "Prim Assembly",
	"db":   "Depth Block",
	"cr":   "Clip Rect",
	"cb":   "Color Block",
	"uvd":  "UVD",
	"vce0": "VCE",
}

// BlockLabelWidth is the reserved label column width plus one gutter column.
var BlockLabelWidth = maxLabelWidth() + 1

func maxLabelWidth() int {
	n := 0
	for _, l := range BlockLabels {
		if len(l) > n {
			n = len(l)
		}
	}
	for _, l := range ClockLabels {
		if len(l) > n {
			n = len(l)
		}
	}
	return n
}

// BlockOrder is the order blocks are shown: the triangle's pipeline path.
var BlockOrder = []string{
	"gpu", "ee", "vgt", "ta", "tc", "sx", "sh", "spi", "smx", "sc", "pa", "db", "cr", "cb",
}

// AuxBlockOrder lists non-pipeline blocks shown after the pipeline group.
var AuxBlockOrder = []string{"uvd", "vce0"}

// ClockLabels maps clock keys to labels, kept apart from BlockLabels.
var ClockLabels = map[string]string{
	"mclk": "Memory Clock",
	"sclk": "Shader Clock",
}

// ClockKeys are the keys carrying a percentage and an absolute frequency.
var ClockKeys = []string{"mclk", "sclk"}

// MemoryKeys are keys carrying a percentage and an absolute megabyte amount.
var MemoryKeys = []string{"vram", "gtt"}

// BlockLabel returns the display label for a radeontop key.
func BlockLabel(key string) string {
	if l, ok := BlockLabels[key]; ok {
		return l
	}
	if l, ok := ClockLabels[key]; ok {
		return l
	}
	return key
}

// BlocksStatus says why the blocks panel is or is not showing data.
type BlocksStatus string

const (
	BlocksOK           BlocksStatus = "ok"
	BlocksDisabled     BlocksStatus = "disabled"
	BlocksMissing      BlocksStatus = "missing"
	BlocksUnsupported  BlocksStatus = "unsupported"
	BlocksNotPermitted BlocksStatus = "not permitted"
	BlocksFailed       BlocksStatus = "failed"
)

// OK reports whether this status carries data.
func (s BlocksStatus) OK() bool { return s == BlocksOK }

// Hint returns a one-line explanation and remedy, or "" when OK.
func (s BlocksStatus) Hint() string {
	switch s {
	case BlocksOK:
		return ""
	case BlocksDisabled:
		return "blocks off (--blocks to enable)"
	case BlocksMissing:
		return "no radeontop binary (install it, or set blocks.binary)"
	case BlocksUnsupported:
		return "radeontop cannot read this card"
	case BlocksNotPermitted:
		return "radeontop needs root to read the GPU registers"
	default:
		return "radeontop failed to produce data"
	}
}

// BlockReading is one block's utilisation over the last sampling window.
type BlockReading struct {
	Key     string
	Label   string
	Percent Percent
}

// ClockReading is one clock's utilisation plus its absolute frequency.
type ClockReading struct {
	Key     string
	Label   string
	Percent *Percent
	Mhz     *Mhz
}

// GpuBlocks is a complete per-block sample for one device.
type GpuBlocks struct {
	Blocks      []BlockReading
	AuxBlocks   []BlockReading
	Clocks      []ClockReading
	TimestampNs Nanoseconds
	AgeNs       int64
}

// IsEmpty reports whether this sample carries no blocks at all.
func (b GpuBlocks) IsEmpty() bool {
	return len(b.Blocks) == 0 && len(b.AuxBlocks) == 0
}

// ByKey returns the blocks keyed by radeontop key.
func (b GpuBlocks) ByKey() map[string]Percent {
	out := make(map[string]Percent, len(b.Blocks)+len(b.AuxBlocks))
	for _, r := range b.Blocks {
		out[r.Key] = r.Percent
	}
	for _, r := range b.AuxBlocks {
		out[r.Key] = r.Percent
	}
	return out
}

// Busiest returns the busiest block, or nil when there are none.
func (b GpuBlocks) Busiest() *BlockReading {
	var best *BlockReading
	for i := range b.Blocks {
		if best == nil || b.Blocks[i].Percent > best.Percent {
			best = &b.Blocks[i]
		}
	}
	for i := range b.AuxBlocks {
		if best == nil || b.AuxBlocks[i].Percent > best.Percent {
			best = &b.AuxBlocks[i]
		}
	}
	return best
}

// BusiestKey returns the busiest block's key, or "" when there are none.
func (b GpuBlocks) BusiestKey() string {
	if r := b.Busiest(); r != nil {
		return r.Key
	}
	return ""
}
