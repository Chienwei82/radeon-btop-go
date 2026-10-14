package model

// MemoryPool is a used/total memory pool such as VRAM or GTT.
type MemoryPool struct {
	Used   *Bytes
	Total  *Bytes
	Source string
}

// Percent returns the fraction of the pool in use, or nil when unknown.
func (m MemoryPool) Percent() *Percent {
	if m.Total == nil || *m.Total <= 0 || m.Used == nil {
		return nil
	}
	v := clampPercent(float64(*m.Used) / float64(*m.Total) * 100.0)
	return &v
}

// Clock is a clock frequency with its current value and ceiling.
type Clock struct {
	Current *Mhz
	Maximum *Mhz
	Source  string
}

// Percent returns the utilisation of the clock range, or nil when unknown.
func (c Clock) Percent() *Percent {
	if c.Maximum == nil || *c.Maximum <= 0 || c.Current == nil {
		return nil
	}
	v := clampPercent(float64(*c.Current) / float64(*c.Maximum) * 100.0)
	return &v
}

// Power is board power draw against the enforced cap.
type Power struct {
	DrawW   *Watts
	CapW    *Watts
	EnergyM *int64
	Source  string
}

// Percent returns power draw as a percentage of the cap, or nil when unknown.
func (p Power) Percent() *Percent {
	if p.CapW == nil || *p.CapW <= 0 || p.DrawW == nil {
		return nil
	}
	v := clampPercent(*p.DrawW / *p.CapW * 100.0)
	return &v
}

// Fan is fan state; APUs and passively cooled cards report nil throughout.
type Fan struct {
	RPM    *Rpm
	MaxRPM *Rpm
	PWM    *Percent
}

// Percent returns fan duty, preferring measured RPM over the PWM value.
func (f Fan) Percent() *Percent {
	if f.RPM != nil && f.MaxRPM != nil && *f.MaxRPM > 0 {
		v := clampPercent(float64(*f.RPM) / float64(*f.MaxRPM) * 100.0)
		return &v
	}
	return f.PWM
}

// Stopped reports true when the fan is present but not spinning.
func (f Fan) Stopped() bool {
	return (f.RPM != nil && *f.RPM == 0) || (f.PWM != nil && *f.PWM == 0)
}

// Temperature is a single temperature reading.
type Temperature struct {
	Label   string
	Celsius *Celsius
	Source  string
}

// ThrottleInfo holds decoded throttle status bits.
type ThrottleInfo struct {
	Raw    int
	Active []string
}

// IsThrottling reports whether any throttle reason is asserted.
func (t ThrottleInfo) IsThrottling() bool { return len(t.Active) > 0 }

// pcieGenerations maps signalling rate in 0.1 GT/s to generation.
var pcieGenerations = [][2]int{
	{25, 1},
	{50, 2},
	{80, 3},
	{160, 4},
	{320, 5},
	{640, 6},
}

// pcieRateTolerance is how far off a standard rate may be and still count.
const pcieRateTolerance = 0.1

// PcieLink is a negotiated PCIe link width and speed.
// Speed is in tenths of a GT/s, the unit the kernel itself uses.
type PcieLink struct {
	Width int
	Speed int
}

// GtPerSecond returns the signalling rate in GT/s.
func (p PcieLink) GtPerSecond() float64 { return float64(p.Speed) / 10 }

// Generation returns the PCIe generation, or 0 when not a standard rate.
func (p PcieLink) Generation() int {
	if p.Speed <= 0 {
		return 0
	}
	bestRate, bestGen := pcieGenerations[0][0], pcieGenerations[0][1]
	for _, entry := range pcieGenerations[1:] {
		if absInt(entry[0]-p.Speed) < absInt(bestRate-p.Speed) {
			bestRate, bestGen = entry[0], entry[1]
		}
	}
	if floatAbs(float64(bestRate-p.Speed)) > float64(bestRate)*pcieRateTolerance {
		return 0
	}
	return bestGen
}

// Describe returns a human readable form such as "Gen3 x16".
func (p PcieLink) Describe() string {
	if gen := p.Generation(); gen > 0 {
		return "Gen" + itoa(gen) + " x" + itoa(p.Width)
	}
	return trimFloat(p.GtPerSecond()) + " GT/s x" + itoa(p.Width)
}

// VoltageReading is one named voltage rail reading in millivolts.
type VoltageReading struct {
	Name string
	MV   int
}

// AmdgpuMetrics is one immutable sample of a single GPU's state.
type AmdgpuMetrics struct {
	Device           AmdgpuDevice
	Sequence         int
	TimestampNs      Nanoseconds
	IntervalS        float64
	GpuBusyPercent   *Percent
	MemBusyPercent   *Percent
	VRAM             MemoryPool
	GTT              MemoryPool
	SCLK             Clock
	MCLK             Clock
	SocClk           Clock
	VoltagesMv       []VoltageReading
	Temperatures     []Temperature
	Power            Power
	Fan              Fan
	PerformanceLevel *string
	PowerProfile     *string
	Throttle         *ThrottleInfo
	PCIE             *PcieLink
	MetricsABI       *string
	Unavailable      []string
	Blocks           *GpuBlocks
	ODC              OdcVoltage
	Profiles         PowerProfileTable
}

func clampPercent(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func floatAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
