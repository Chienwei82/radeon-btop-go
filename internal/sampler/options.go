package sampler

import "github.com/gputop/gputop-go/internal/model"

// DefaultIntervalS is the default sampling period in seconds.
const DefaultIntervalS = 1.0

// Interval floors and ceilings. The floor exists because a faster tick would
// spend more time in procfs than in the data it collects, and the procfs scan
// is the expensive part; the ceiling bounds set-interval so a key press cannot
// park the loop for hours.
const (
	MinIntervalS = 0.05
	MaxIntervalS = 600.0
)

// Warning caps. A permanently missing file warns once and is bounded, so it
// cannot grow the snapshot without limit or push out a real one-shot error.
const (
	MaxWarnings       = 8
	MaxSeenWarnings   = 256
	snapshotQueueSize = 2
)

// Default roots, matching where the kernel exposes the data.
const (
	DefaultDrmRoot  = "/sys/class/drm"
	DefaultProcRoot = "/proc"
	DefaultPciRoot  = "/sys/bus/pci/devices"
)

// SamplerOptions is the immutable sampling configuration.
type SamplerOptions struct {
	// IntervalS is the sampling period in seconds.
	IntervalS float64
	// HistoryLength is how many snapshots the history ring retains.
	HistoryLength int
	// CollectProcesses enables the /proc per-process scan.
	CollectProcesses bool
	// DrmRoot, ProcRoot and PciRoot are the kernel trees to read. Empty means
	// the real root; tests point them at synthetic trees.
	DrmRoot  string
	ProcRoot string
	PciRoot  string
	// KindOverride forces a device classification, or nil to auto-detect.
	KindOverride *model.DeviceKind
	// NameOverrides maps a PCI address to a display name.
	NameOverrides map[string]string
	// OnlyDevices restricts monitoring to these cards (matched by BDF in full
	// and shortened form, and by cardN). Empty watches every discovered card.
	OnlyDevices []string
}

// validated returns a copy with a usable interval, history length and roots.
//
// The interval floor exists because a faster tick would spend more time in
// procfs than in the data it collects, and the procfs scan is the expensive
// part.
func (o SamplerOptions) validated() SamplerOptions {
	out := o
	if out.IntervalS < MinIntervalS {
		out.IntervalS = MinIntervalS
	}
	if out.HistoryLength < 1 {
		out.HistoryLength = 1
	}
	if out.DrmRoot == "" {
		out.DrmRoot = DefaultDrmRoot
	}
	if out.ProcRoot == "" {
		out.ProcRoot = DefaultProcRoot
	}
	if out.PciRoot == "" {
		out.PciRoot = DefaultPciRoot
	}
	// Copy the map so a later mutation by the caller cannot reach in.
	if len(out.NameOverrides) > 0 {
		names := make(map[string]string, len(out.NameOverrides))
		for k, v := range out.NameOverrides {
			names[k] = v
		}
		out.NameOverrides = names
	}
	return out
}

// clampInterval floors and ceilings an interval for set-interval, which is
// clamped rather than rejected: the only caller is a key press and the clamp
// is exactly the behaviour the key advertises.
func clampInterval(intervalS float64) float64 {
	if intervalS < MinIntervalS {
		return MinIntervalS
	}
	if intervalS > MaxIntervalS {
		return MaxIntervalS
	}
	return intervalS
}
