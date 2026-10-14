package model

// EngineUsage holds cumulative and per-interval time on one GPU engine.
type EngineUsage struct {
	Engine   string
	TotalNs  Nanoseconds
	DeltaNs  Nanoseconds
	WindowNs Nanoseconds
}

// Percent returns utilisation over the sampling window, or nil w/o baseline.
func (u EngineUsage) Percent() *Percent {
	if u.WindowNs <= 0 || u.TotalNs <= 0 {
		return nil
	}
	v := clampPercent(float64(u.DeltaNs) / float64(u.WindowNs) * 100.0)
	return &v
}

// GpuProcess is one DRM client, deduplicated by (drm-pdev, drm-client-id).
type GpuProcess struct {
	PID        int
	Name       string
	User       string
	BDF        Bdf
	ClientID   ClientId
	Cmdline    []string
	PPID       *int
	Engines    []EngineUsage
	VramUsed   *Bytes
	VramShared *Bytes
	GttUsed    *Bytes
	GttShared  *Bytes
	CpuUsed    *Bytes
}

// Identity returns the row's stable key: the kernel-reported dedup pair.
func (p GpuProcess) Identity() (Bdf, ClientId) { return p.BDF, p.ClientID }

// Command returns the full command line, falling back to comm name.
func (p GpuProcess) Command() string {
	if len(p.Cmdline) == 0 {
		return p.Name
	}
	out := ""
	for i, arg := range p.Cmdline {
		if i > 0 {
			out += " "
		}
		out += arg
	}
	return out
}

// IsKernelThread reports whether this looks like a kernel thread.
func (p GpuProcess) IsKernelThread() bool { return len(p.Cmdline) == 0 }

// EnginePercent returns the sum of per-engine utilisation, clamped to 100.
// Zero (not nil) for a client never scheduled, so a fresh table is real.
func (p GpuProcess) EnginePercent() Percent {
	total := 0.0
	for _, u := range p.Engines {
		if v := u.Percent(); v != nil {
			total += *v
		}
	}
	return clampPercent(total)
}

// IsIdle reports whether the client is doing nothing measurable.
func (p GpuProcess) IsIdle() bool {
	return p.EnginePercent() <= 0.0 && p.MemoryUsed() <= 0
}

// MemoryUsed returns total resident memory attributed to this client.
func (p GpuProcess) MemoryUsed() Bytes {
	var total Bytes
	if p.VramUsed != nil {
		total += *p.VramUsed
	}
	if p.GttUsed != nil {
		total += *p.GttUsed
	}
	if p.CpuUsed != nil {
		total += *p.CpuUsed
	}
	return total
}

// EnginePercentFor returns utilisation for one normalised bucket, or nil.
func (p GpuProcess) EnginePercentFor(bucket string) *Percent {
	total := 0.0
	seen := false
	for _, u := range p.Engines {
		if u.Engine == bucket {
			if v := u.Percent(); v != nil {
				total += *v
				seen = true
			}
		}
	}
	if !seen {
		return nil
	}
	v := clampPercent(total)
	return &v
}

// BucketPercent returns summed utilisation across buckets, or nil if none.
func (p GpuProcess) BucketPercent(buckets ...string) *Percent {
	total := 0.0
	seen := false
	for _, b := range buckets {
		if v := p.EnginePercentFor(b); v != nil {
			total += *v
			seen = true
		}
	}
	if !seen {
		return nil
	}
	v := clampPercent(total)
	return &v
}

// EncDecPercent returns video encode plus decode utilisation.
func (p GpuProcess) EncDecPercent() *Percent { return p.BucketPercent("enc", "dec") }

// EngineBuckets are the display buckets in order.
var EngineBuckets = []string{"gfx", "compute", "dma", "enc", "dec", "other"}

// SortColumn is every value the process table can be ordered by.
type SortColumn string

const (
	SortTotal   SortColumn = "total"
	SortPID     SortColumn = "pid"
	SortUser    SortColumn = "user"
	SortCommand SortColumn = "command"
	SortGfx     SortColumn = "gfx"
	SortCompute SortColumn = "compute"
	SortDMA     SortColumn = "dma"
	SortEncDec  SortColumn = "encdc"
	SortVRAM    SortColumn = "vram"
	SortGTT     SortColumn = "gtt"
)

// SortColumns lists every sortable column.
var SortColumns = []SortColumn{
	SortTotal, SortPID, SortUser, SortCommand, SortGfx,
	SortCompute, SortDMA, SortEncDec, SortVRAM, SortGTT,
}

// ValidSortColumn reports whether s names a sortable column.
func ValidSortColumn(s string) bool {
	for _, c := range SortColumns {
		if string(c) == s {
			return true
		}
	}
	return false
}
