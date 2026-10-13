package model

import "testing"

func ptr[T any](v T) *T { return &v }

func TestMemoryPoolPercent(t *testing.T) {
	half := MemoryPool{Used: ptr[Bytes](50), Total: ptr[Bytes](100)}
	if got := half.Percent(); got == nil || *got != 50 {
		t.Fatalf("half pool Percent() = %v, want 50", got)
	}
	if got := (MemoryPool{Total: ptr[Bytes](100)}).Percent(); got != nil {
		t.Fatalf("nil used Percent() = %v, want nil", *got)
	}
	if got := (MemoryPool{Used: ptr[Bytes](50)}).Percent(); got != nil {
		t.Fatalf("nil total Percent() = %v, want nil", *got)
	}
	if got := (MemoryPool{Used: ptr[Bytes](50), Total: ptr[Bytes](0)}).Percent(); got != nil {
		t.Fatalf("zero total Percent() = %v, want nil", *got)
	}
	over := MemoryPool{Used: ptr[Bytes](150), Total: ptr[Bytes](100)}
	if got := over.Percent(); got == nil || *got != 100 {
		t.Fatalf("over pool Percent() = %v, want clamp 100", got)
	}
}

func TestClockPowerFan(t *testing.T) {
	if got := (Clock{Current: ptr[Mhz](500), Maximum: ptr[Mhz](1000)}).Percent(); got == nil || *got != 50 {
		t.Fatalf("Clock.Percent() = %v, want 50", got)
	}
	if got := (Clock{Current: ptr[Mhz](500)}).Percent(); got != nil {
		t.Fatalf("Clock without max = %v, want nil", *got)
	}
	if got := (Power{DrawW: ptr[Watts](33), CapW: ptr[Watts](100)}).Percent(); got == nil || *got != 33 {
		t.Fatalf("Power.Percent() = %v, want 33", got)
	}
	if got := (Power{DrawW: ptr[Watts](33)}).Percent(); got != nil {
		t.Fatalf("Power without cap = %v, want nil", *got)
	}
	f := Fan{RPM: ptr[Rpm](1000), MaxRPM: ptr[Rpm](2000), PWM: ptr[Percent](10)}
	if got := f.Percent(); got == nil || *got != 50 {
		t.Fatalf("Fan.Percent() = %v, want 50", got)
	}
	f2 := Fan{PWM: ptr[Percent](25)}
	if got := f2.Percent(); got == nil || *got != 25 {
		t.Fatalf("Fan fallback = %v, want 25", got)
	}
	if (Fan{}).Percent() != nil {
		t.Fatal("empty fan must be nil, never 0")
	}
	if !(Fan{RPM: ptr[Rpm](0)}).Stopped() {
		t.Fatal("rpm 0 must report stopped")
	}
	if (Fan{}).Stopped() {
		t.Fatal("absent fan must not report stopped")
	}
}

func TestPcieAndDevice(t *testing.T) {
	if got := (PcieLink{Width: 16, Speed: 80}).Describe(); got != "Gen3 x16" {
		t.Fatalf("Describe() = %q, want Gen3 x16", got)
	}
	if got := (PcieLink{Width: 16, Speed: 80}).Generation(); got != 3 {
		t.Fatalf("Generation() = %d, want 3", got)
	}
	if got := (PcieLink{Width: 8, Speed: 999}).Generation(); got != 0 {
		t.Fatalf("odd rate Generation() = %d, want 0", got)
	}
	if got := DescribeDeviceID(0x73BF); got != "AMD Radeon RX 6800" {
		t.Fatalf("known id = %q", got)
	}
	if got := DescribeDeviceID(0x9999); got != "AMD Radeon [0x9999]" {
		t.Fatalf("unknown id = %q", got)
	}
}

func TestEngineUsageAndProcess(t *testing.T) {
	u := EngineUsage{Engine: "gfx", TotalNs: 100, DeltaNs: 50, WindowNs: 0}
	if u.Percent() != nil {
		t.Fatal("zero window must be nil, never 0")
	}
	u2 := EngineUsage{Engine: "gfx", TotalNs: 0, DeltaNs: 0, WindowNs: 1_000_000_000}
	if u2.Percent() != nil {
		t.Fatal("zero total must be nil (no baseline yet)")
	}
	u3 := EngineUsage{Engine: "gfx", TotalNs: 2_000_000_000, DeltaNs: 2_000_000_000, WindowNs: 1_000_000_000}
	if got := u3.Percent(); got == nil || *got != 100 {
		t.Fatal("over-window delta must clamp at 100")
	}
	p := GpuProcess{PID: 1, Name: "foo", BDF: "0000:0c:00.0", ClientID: 7}
	if bdf, id := p.Identity(); bdf != "0000:0c:00.0" || id != 7 {
		t.Fatalf("Identity() = (%q,%d)", bdf, id)
	}
	if p.Command() != "foo" {
		t.Fatalf("Command() fallback = %q", p.Command())
	}
	if !p.IsKernelThread() || !p.IsIdle() {
		t.Fatal("fresh process must be kernel-thread-looking and idle")
	}
	p2 := GpuProcess{
		PID: 2, Name: "x", BDF: "0000:0c:00.0", ClientID: 8,
		Cmdline:  []string{"app", "--flag"},
		Engines:  []EngineUsage{{Engine: "gfx", TotalNs: 500_000_000, DeltaNs: 500_000_000, WindowNs: 1_000_000_000}},
		VramUsed: ptr[Bytes](100),
	}
	if p2.Command() != "app --flag" {
		t.Fatalf("Command() = %q", p2.Command())
	}
	if p2.EnginePercent() != 50 {
		t.Fatalf("EnginePercent() = %v, want 50", p2.EnginePercent())
	}
	if p2.IsIdle() {
		t.Fatal("busy process must not be idle")
	}
}

func TestPowerProfileTable(t *testing.T) {
	var empty PowerProfileTable
	if empty.Active() != nil || empty.Describe() != "N/A" {
		t.Fatal("empty table must report N/A")
	}
	tab := PowerProfileTable{Profiles: []PowerProfileEntry{
		{Index: 0, Name: "BOOTUP_DEFAULT"},
		{Index: 2, Name: "3D_FULL_SCREEN", Active: true},
	}, Present: true}
	if tab.ActiveName() != "3D_FULL_SCREEN" {
		t.Fatalf("ActiveName() = %q", tab.ActiveName())
	}
	if tab.Describe() != "3D_FULL_SCREEN (2 of 2)" {
		t.Fatalf("Describe() = %q", tab.Describe())
	}
}

func TestOdvDomain(t *testing.T) {
	lo, hi := 500, 2600
	d := OdvDomain{
		Name: "sclk",
		Entries: []OdvEntry{
			{State: 0, Value: 500, Unit: OdvMegahertz},
			{State: 1, Value: 2104, Unit: OdvMegahertz},
		},
		Minimum: &lo, Maximum: &hi, Unit: OdvMegahertz,
	}
	if *d.Ceiling() != 2104 {
		t.Fatalf("Ceiling() = %v", *d.Ceiling())
	}
	if d.Describe() != "2104/2600 MHz" {
		t.Fatalf("Describe() = %q", d.Describe())
	}
	if (OdvDomain{Name: "sclk"}).Ceiling() != nil {
		t.Fatal("empty domain ceiling must be nil")
	}
}

func TestAlertEvaluate(t *testing.T) {
	m := AmdgpuMetrics{Temperatures: []Temperature{{Label: "junction", Celsius: ptr[Celsius](94)}}}
	if s := Evaluate(m, Thresholds{TempC: 90}); !s.Active() {
		t.Fatal("94C over 90C must alert")
	}
	if Evaluate(AmdgpuMetrics{}, Thresholds{TempC: 90}).Active() {
		t.Fatal("missing reading must never alert")
	}
	if Evaluate(m, Thresholds{}).Active() {
		t.Fatal("zero thresholds disable alerting")
	}
}

func TestRingBufferAndSnapshot(t *testing.T) {
	r := NewRingBuffer[int](3)
	r.Append(1)
	r.Append(2)
	r.Append(3)
	r.Append(4)
	got := r.Items()
	if len(got) != 3 || got[0] != 2 || got[2] != 4 {
		t.Fatalf("Items() = %v, want [2 3 4]", got)
	}
	last, ok := r.Last()
	if !ok || last != 4 {
		t.Fatalf("Last() = %v,%v", last, ok)
	}
	r.Clear()
	if r.Len() != 0 {
		t.Fatal("Clear() must empty the buffer")
	}
	s := GpuSnapshot{TotalProcessCount: 5, VisibleProcessCount: 3}
	if !s.IsPartialProcessView() {
		t.Fatal("5 total > 3 visible must be partial")
	}
	if (GpuSnapshot{}).IsPartialProcessView() {
		t.Fatal("empty snapshot must not be partial")
	}
	if !BlocksOK.OK() || BlocksDisabled.OK() {
		t.Fatal("only OK must report ok")
	}
	if BlocksDisabled.Hint() == "" || BlocksOK.Hint() != "" {
		t.Fatal("hint shape wrong")
	}
}
