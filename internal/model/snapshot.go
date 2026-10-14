package model

// SourceReport says which source each metric family resolved from.
type SourceReport struct {
	MetricsABI string
	PerMetric  map[string]string
}

// UsingBinaryMetrics reports whether gpu_metrics produced this sample.
func (s SourceReport) UsingBinaryMetrics() bool { return s.MetricsABI != "" }

// SamplerStats holds rolling health counters for the sampler.
type SamplerStats struct {
	Ticks          int
	TickErrors     int
	ReadErrors     int
	LastDurationMs float64
	AvgDurationMs  float64
}

// GpuSnapshot is a complete immutable view of all GPUs at one instant.
type GpuSnapshot struct {
	Sequence            int
	TimestampNs         Nanoseconds
	Devices             []AmdgpuMetrics
	Processes           []GpuProcess
	VisibleProcessCount int
	TotalProcessCount   int
	Stats               SamplerStats
	Source              SourceReport
	Warnings            []string
	BlocksStatus        BlocksStatus
	BlocksHint          string
	WallClockNs         int64
}

// DeviceByIndex returns the sample for a device index, or nil.
func (s GpuSnapshot) DeviceByIndex(index int) *AmdgpuMetrics {
	for i := range s.Devices {
		if s.Devices[i].Device.Index == index {
			return &s.Devices[i]
		}
	}
	return nil
}

// IsPartialProcessView reports whether /proc permissions hid processes.
func (s GpuSnapshot) IsPartialProcessView() bool {
	return s.TotalProcessCount > s.VisibleProcessCount
}
