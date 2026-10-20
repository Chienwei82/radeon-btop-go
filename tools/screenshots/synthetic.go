package main

import (
	"math"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/ui"
)

// Byte sizes used by the synthetic machine.
const (
	mib = model.Bytes(1024 * 1024)
	gib = model.Bytes(1024 * 1024 * 1024)
)

// historyLen is one minute of one-second samples: enough to fill the widest
// graph in the gallery without inventing a different history per screenshot.
const historyLen = 120

// dgpuBDF and igpuBDF are the two addresses the synthetic machine uses.
const (
	dgpuBDF model.Bdf = "0000:0c:00.0"
	igpuBDF model.Bdf = "0000:03:00.0"
)

// machineInput builds the deterministic two-GPU machine every screenshot
// shares. Only the view state and the terminal size differ between shots.
func machineInput() ui.LayoutInput {
	history := historySamples()
	last := len(history) - 1
	decorateDGpu(&history[last].Devices[0])

	snap := history[last]
	snap.Processes = machineProcesses()
	snap.VisibleProcessCount = len(snap.Processes)
	snap.TotalProcessCount = len(snap.Processes)
	snap.Source = model.SourceReport{MetricsABI: "v1.3"}
	snap.BlocksStatus = model.BlocksOK
	snap.Stats = model.SamplerStats{
		Ticks: len(history), LastDurationMs: 18.6, AvgDurationMs: 19.2,
	}

	return ui.LayoutInput{
		Snapshot:  snap,
		History:   history,
		View:      *ui.NewViewState(),
		Theme:     ui.DefaultTheme,
		Width:     120,
		Height:    48,
		UptimeS:   3725,
		IntervalS: 1.0,
	}
}

// historySamples builds the retained history. The last sample is the card
// under the load the README describes; the earlier ones are a smooth wave so
// the braille graph shows a shape rather than a flat line.
func historySamples() []model.GpuSnapshot {
	history := make([]model.GpuSnapshot, 0, historyLen)
	for i := 0; i < historyLen; i++ {
		// t is zero at the newest sample and negative going back in time.
		t := float64(i-(historyLen-1)) / 12.0
		busy := 78 + 12*math.Sin(t/1.7) + 4*math.Sin(t*0.6)
		vram := 60 + 6*math.Sin(t/2.4)
		gtt := 9 + 2*math.Sin(t/3.1)
		gfxClk := int(math.Round(2310 + 90*math.Sin(t/1.7)))

		dgpu := dgpuSample(busy, vram, gtt, gfxClk)
		igpu := igpuSample(31+6*math.Sin(t/1.3+0.8), 24+4*math.Sin(t/2.0))
		history = append(history, model.GpuSnapshot{
			Sequence:    i + 1,
			TimestampNs: model.Nanoseconds(i) * 1_000_000_000,
			Devices:     []model.AmdgpuMetrics{dgpu, igpu},
		})
	}
	return history
}

// dgpuSample is one sample of the RX 6800. Values follow the busy percentage
// the way the real card does: power, temperatures and fan rise with load.
func dgpuSample(busy, vramFill, gttFill float64, gfxClk int) model.AmdgpuMetrics {
	total := 16 * gib
	return model.AmdgpuMetrics{
		Device: model.AmdgpuDevice{
			Index: 0, Card: "card1", Render: strPtr("renderD128"),
			BDF: dgpuBDF, VendorID: 0x1002, DeviceID: 0x73BF,
			DeviceDir: "/sys/class/drm/card1/device",
			Kind:      model.DeviceDGPU, Name: "AMD Radeon RX 6800", BootVGA: true,
		},
		IntervalS:      1.0,
		GpuBusyPercent: percentPtr(busy),
		MemBusyPercent: percentPtr(busy * 0.55),
		VRAM: model.MemoryPool{
			Used: bytePtr(total / 100 * model.Bytes(vramFill)), Total: bytePtr(total),
			Source: "gpu_metrics",
		},
		GTT: model.MemoryPool{
			Used: bytePtr(total / 100 * model.Bytes(gttFill)), Total: bytePtr(total),
			Source: "gpu_metrics",
		},
		SCLK:       model.Clock{Current: mhzPtr(gfxClk), Maximum: mhzPtr(2475), Source: "gpu_metrics"},
		MCLK:       model.Clock{Current: mhzPtr(1000), Maximum: mhzPtr(1000), Source: "gpu_metrics"},
		SocClk:     model.Clock{Current: mhzPtr(900), Maximum: mhzPtr(1200), Source: "gpu_metrics"},
		VoltagesMv: []model.VoltageReading{{Name: "soc", MV: 912}, {Name: "gfx", MV: 851}, {Name: "mem", MV: 900}},
		Temperatures: []model.Temperature{
			{Label: "edge", Celsius: celsiusPtr(48 + busy*0.17)},
			{Label: "junction", Celsius: celsiusPtr(58 + busy*0.20)},
			{Label: "mem", Celsius: celsiusPtr(54 + busy*0.18)},
		},
		Power: model.Power{DrawW: wattsPtr(44 + busy*1.2), CapW: wattsPtr(211), Source: "gpu_metrics"},
		Fan: model.Fan{
			RPM: rpmPtr(int(math.Round(980 + busy*14))), MaxRPM: rpmPtr(3100),
			PWM: percentPtr(busy * 0.85),
		},
		PerformanceLevel: strPtr("auto"),
		PowerProfile:     strPtr("3D_FULL_SCREEN"),
		Throttle:         &model.ThrottleInfo{Raw: 0},
		PCIE:             &model.PcieLink{Width: 16, Speed: 80},
		MetricsABI:       strPtr("v1.3"),
	}
}

// igpuSample is one sample of the Cezanne APU: no fan, no memory clock and a
// small carve-out, which is what makes the second overview row look different
// from the first rather than like a copy.
func igpuSample(busy, vramFill float64) model.AmdgpuMetrics {
	vramTotal := 2 * gib
	return model.AmdgpuMetrics{
		Device: model.AmdgpuDevice{
			Index: 1, Card: "card0", Render: strPtr("renderD129"),
			BDF: igpuBDF, VendorID: 0x1002, DeviceID: 0x1638,
			DeviceDir: "/sys/class/drm/card0/device",
			Kind:      model.DeviceIGPU, Name: model.DescribeDeviceID(0x1638), BootVGA: false,
		},
		IntervalS:      1.0,
		GpuBusyPercent: percentPtr(busy),
		MemBusyPercent: percentPtr(busy * 0.7),
		VRAM: model.MemoryPool{
			Used: bytePtr(vramTotal / 100 * model.Bytes(vramFill)), Total: bytePtr(vramTotal),
			Source: "gpu_metrics",
		},
		GTT: model.MemoryPool{
			Used: bytePtr(2*gib + 180*mib), Total: bytePtr(16 * gib),
			Source: "gpu_metrics",
		},
		SCLK:   model.Clock{Current: mhzPtr(1800), Maximum: mhzPtr(2200), Source: "gpu_metrics"},
		SocClk: model.Clock{Current: mhzPtr(1200), Maximum: mhzPtr(1400), Source: "gpu_metrics"},
		VoltagesMv: []model.VoltageReading{
			{Name: "soc", MV: 890}, {Name: "gfx", MV: 790},
		},
		Temperatures: []model.Temperature{
			{Label: "gfx", Celsius: celsiusPtr(52)},
			{Label: "soc", Celsius: celsiusPtr(48)},
			{Label: "core", Celsius: celsiusPtr(55)},
			{Label: "l3", Celsius: celsiusPtr(49)},
		},
		Power:      model.Power{DrawW: wattsPtr(18), CapW: wattsPtr(45), Source: "gpu_metrics"},
		MetricsABI: strPtr("v2.1"),
	}
}

// decorateDGpu adds the data only the newest sample shows: the radeontop
// blocks and the two SCPP tables behind the optional row.
func decorateDGpu(m *model.AmdgpuMetrics) {
	m.Blocks = dgpuBlocks()
	m.Profiles = model.PowerProfileTable{
		Present: true,
		Profiles: []model.PowerProfileEntry{
			{Index: 0, Name: "BOOTUP_DEFAULT"},
			{Index: 1, Name: "3D_FULL_SCREEN", Active: true},
			{Index: 2, Name: "POWER_SAVING"},
			{Index: 3, Name: "VIDEO"},
			{Index: 4, Name: "COMPUTE"},
			{Index: 5, Name: "CUSTOM"},
			{Index: 6, Name: "VR"},
		},
	}
	m.ODC = model.OdcVoltage{
		Present:        true,
		VddgfxOffsetMv: intPtr(0),
		Domains: []model.OdvDomain{
			{
				Name: "sclk", Minimum: intPtr(500), Maximum: intPtr(2600),
				Unit: model.OdvMegahertz,
				Entries: []model.OdvEntry{
					{State: 0, Value: 500, Unit: model.OdvMegahertz},
					{State: 1, Value: 2600, Unit: model.OdvMegahertz},
				},
			},
			{
				Name: "mclk", Minimum: intPtr(96), Maximum: intPtr(1075),
				Unit: model.OdvMegahertz,
				Entries: []model.OdvEntry{
					{State: 0, Value: 96, Unit: model.OdvMegahertz},
					{State: 1, Value: 1075, Unit: model.OdvMegahertz},
				},
			},
			{
				Name: "socclk", Minimum: intPtr(500), Maximum: intPtr(1200),
				Unit: model.OdvMegahertz,
				Entries: []model.OdvEntry{
					{State: 0, Value: 500, Unit: model.OdvMegahertz},
					{State: 1, Value: 1200, Unit: model.OdvMegahertz},
				},
			},
		},
	}
}

// dgpuBlocks is a radeontop sample shaped like a 3D workload: geometry and
// colour busy, video engines nearly idle.
func dgpuBlocks() *model.GpuBlocks {
	values := map[string]model.Percent{
		"gpu": 78, "ee": 12, "vgt": 64, "ta": 71, "tc": 68, "sx": 55,
		"sh": 43, "spi": 61, "smx": 39, "sc": 48, "pa": 52, "db": 44,
		"cr": 18, "cb": 74,
	}
	blocks := make([]model.BlockReading, 0, len(model.BlockOrder))
	for _, key := range model.BlockOrder {
		blocks = append(blocks, model.BlockReading{Key: key, Label: model.BlockLabel(key), Percent: values[key]})
	}
	return &model.GpuBlocks{
		Blocks: blocks,
		AuxBlocks: []model.BlockReading{
			{Key: "uvd", Label: model.BlockLabel("uvd"), Percent: 4},
			{Key: "vce0", Label: model.BlockLabel("vce0"), Percent: 2},
		},
		Clocks: []model.ClockReading{
			{Key: "mclk", Label: model.BlockLabel("mclk"), Percent: percentPtr(100), Mhz: mhzPtr(1000)},
			{Key: "sclk", Label: model.BlockLabel("sclk"), Percent: percentPtr(93), Mhz: mhzPtr(2310)},
		},
	}
}

// machineProcesses is the DRM client set: a render job with a worker child, a
// browser with its isolated content process, and an inference server on the
// APU. Five rows are enough to show selection, nesting and both devices while
// leaving the table and footer inside a 48-row terminal.
func machineProcesses() []model.GpuProcess {
	return []model.GpuProcess{
		gpuProcess(4211, "blender", "david", 1, dgpuBDF, 3, 512*mib, 64*mib,
			engineUsage("gfx", 88), engineUsage("compute", 12)),
		gpuProcess(4212, "blender", "david", 4211, dgpuBDF, 4, 256*mib, 32*mib,
			engineUsage("gfx", 64)),
		gpuProcess(8842, "firefox", "david", 1, dgpuBDF, 17, 128*mib, 256*mib,
			engineUsage("gfx", 41)),
		gpuProcess(8843, "Isolated Web Co", "david", 8842, dgpuBDF, 18, 48*mib, 64*mib,
			engineUsage("gfx", 6)),
		gpuProcess(12044, "ollama", "david", 1, igpuBDF, 8, 2*gib, 128*mib,
			engineUsage("compute", 63)),
	}
}

// gpuProcess builds one DRM client row.
func gpuProcess(pid int, name, user string, ppid int, bdf model.Bdf, client model.ClientId,
	vram, gtt model.Bytes, engines ...model.EngineUsage) model.GpuProcess {
	return model.GpuProcess{
		PID: pid, Name: name, User: user, BDF: bdf, ClientID: client,
		Cmdline:  []string{name},
		PPID:     intPtr(ppid),
		Engines:  engines,
		VramUsed: bytePtr(vram),
		GttUsed:  bytePtr(gtt),
	}
}

// engineUsage turns a percentage into the cumulative/delta pair the table
// diffs. One second of window keeps the numbers readable.
func engineUsage(name string, pct model.Percent) model.EngineUsage {
	const window = model.Nanoseconds(1_000_000_000)
	delta := model.Nanoseconds(float64(pct) / 100.0 * float64(window))
	return model.EngineUsage{Engine: name, TotalNs: delta + 1, DeltaNs: delta, WindowNs: window}
}

// The pointer helpers keep the synthetic literals readable; model optional
// metrics are pointers because nil means N/A.
func strPtr(v string) *string             { return &v }
func intPtr(v int) *int                   { return &v }
func percentPtr(v float64) *model.Percent { p := model.Percent(v); return &p }
func bytePtr(v model.Bytes) *model.Bytes  { return &v }
func mhzPtr(v int) *model.Mhz             { return &v }
func celsiusPtr(v float64) *model.Celsius { c := model.Celsius(v); return &c }
func wattsPtr(v float64) *model.Watts     { w := model.Watts(v); return &w }
func rpmPtr(v int) *model.Rpm             { return &v }
