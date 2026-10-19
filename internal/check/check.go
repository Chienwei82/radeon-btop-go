// Package check implements the basic `gputop --check`: what this machine
// can report, and why not.
//
// The report is built from a real sample, not from a list of expected
// filenames, so what it says is what the interface would actually draw on
// this machine right now. The missing metrics are then probed on disk to
// explain themselves.
//
// No reason is invented: every Reason is derived from something observed —
// an errno, a kernel version compared against a documented minimum, or the
// shape of the device. When none of those explains it, the reason is
// "not published" and the text says so rather than guessing. The report is
// plain ASCII: it is meant to be pasted into a bug report.
package check

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// ReportWidth is the widest the text report draws itself.
const ReportWidth = 96

// Availability is whether a metric can be shown.
type Availability string

const (
	// OK is a metric the sample produced.
	AvailabilityOK Availability = "ok"
	// Missing is a metric the sample did not produce.
	AvailabilityMissing Availability = "missing"
)

// Reason is why a metric is not available. Each value carries the
// information a user needs to act: "install radeontop" and "you need root"
// are different problems.
type Reason string

const (
	ReasonNotPublished          Reason = "not published"
	ReasonNeedsNewerKernel      Reason = "needs a newer kernel"
	ReasonUnsupportedGeneration Reason = "not on this GPU generation"
	ReasonPermission            Reason = "permission denied"
	ReasonUnknownABI            Reason = "unknown gpu_metrics version"
)

// minKernelGPUMetrics is the kernel release that introduced the versioned
// gpu_metrics binary table; before it every reading comes from sysfs.
var minKernelGPUMetrics = [2]int{5, 10}

// minKernelFDINFO is the release that introduced DRM fdinfo accounting.
var minKernelFDINFO = [2]int{5, 19}

// attributeMinKernel maps a sysfs attribute to the release that introduced
// it, used only to explain an absence. Where a minimum could not be
// established it is left out rather than guessed.
var attributeMinKernel = map[string][2]int{
	"gpu_metrics":           minKernelGPUMetrics,
	"gpu_busy_percent":      {4, 19},
	"pp_power_profile_mode": {4, 17},
	"pp_od_clk_voltage":     {4, 17},
	"mem_vram_used":         {5, 2},
	"mem_vram_total":        {5, 2},
	"mem_gtt_used":          {5, 2},
	"mem_gtt_total":         {5, 2},
	"drm_engine_gfx":        minKernelFDINFO,
}

// unknownKernel is what kernelVersion returns for a release it could not
// parse; it is not a version and must never be printed as one.
var unknownKernel = [2]int{0, 0}

// Metric is one metric the report can say something about.
type Metric struct {
	Key          string
	Label        string
	Source       string
	Value        *string
	Attributes   []string
	MinKernel    *[2]int
	DiscreteOnly bool
	Remedy       string
	Extra        string
}

// Finding is a metric plus the verdict on it.
type Finding struct {
	Metric       Metric
	Availability Availability
	Reason       Reason
	Detail       string
}

// OK reports whether the metric was produced.
func (f Finding) OK() bool { return f.Availability == AvailabilityOK }

// DeviceReport is everything the report says about one GPU.
type DeviceReport struct {
	Device     model.AmdgpuDevice
	MetricsABI string
	Findings   []Finding
}

// Missing are the findings that are not ok.
func (d DeviceReport) Missing() []Finding {
	var out []Finding
	for _, f := range d.Findings {
		if !f.OK() {
			out = append(out, f)
		}
	}
	return out
}

// Report is the whole --check answer.
type Report struct {
	Kernel  string
	Amdgpu  string
	Devices []DeviceReport
	// System are the findings about the machine rather than one GPU: the
	// process table and the optional per-block panel.
	System   []Finding
	Warnings []string
}

// OK reports whether the run found anything to report on.
func (r Report) OK() bool { return len(r.Devices) > 0 }

// kernelVersion parses a release string to (major, minor). Unparseable is
// (0, 0), which kernelKnown rejects so a garbage release is never compared
// as if it were a version.
func kernelVersion(release string) [2]int {
	fields := strings.FieldsFunc(strings.TrimSpace(release), func(r rune) bool {
		return r == '.' || r == '-' || r == '+'
	})
	if len(fields) == 0 {
		return unknownKernel
	}
	major, ok1 := atoiSafe(fields[0])
	if !ok1 || major < 0 {
		return unknownKernel
	}
	if len(fields) < 2 {
		return unknownKernel
	}
	minor, ok2 := atoiSafe(fields[1])
	if !ok2 || minor < 0 {
		return unknownKernel
	}
	return [2]int{major, minor}
}

func atoiSafe(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, true
}

// kernelKnown reports whether the version is a real kernel version.
func kernelKnown(v [2]int) bool { return v != unknownKernel }

// atLeast reports whether version is >= minimum.
func atLeast(version, minimum [2]int) bool {
	if version[0] != minimum[0] {
		return version[0] > minimum[0]
	}
	return version[1] >= minimum[1]
}

// describeVersion renders a version for a human.
func describeVersion(v [2]int) string { return fmt.Sprintf("%d.%d", v[0], v[1]) }

// probeClass is what probe found at one path.
type probeClass int

const (
	probeMissing probeClass = iota
	probeOK
	probePermission
	probeUnreadable
)

// probe classifies one path without inventing anything: exists and opens,
// exists but denied, exists but unusable, or does not exist.
func probe(path string) probeClass {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsPermission(err) {
			return probePermission
		}
		return probeMissing
	}
	if info.IsDir() {
		// A directory that exists is as informative as a file: the source
		// is published here.
		return probeOK
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsPermission(err) {
			return probePermission
		}
		return probeUnreadable
	}
	_ = f.Close()
	return probeOK
}

// explainOpts carries the device-shaped inputs explain needs.
type explainOpts struct {
	discreteOnly bool
	integrated   bool
	metricsABI   string
}

// explain works out why a metric is missing, from what is on disk.
//
// The order is the order of what a user can act on: hardware that cannot
// have the sensor at all first, then a source present but empty, then a
// source that exists but cannot be opened, and only then the kernel
// version — the one explanation that cannot be wrong but is the least
// likely. Every candidate is probed once: re-opening a file to ask a
// second question is how "permission denied" turns into "not published".
func explain(deviceDir string, attributes []string, kernel [2]int, opts explainOpts) (Reason, string) {
	if len(attributes) == 0 {
		return ReasonNotPublished, "this card exposes no source for it"
	}
	if opts.discreteOnly && opts.integrated {
		return ReasonUnsupportedGeneration, "an integrated GPU has no fan to report"
	}

	states := map[string]probeClass{}
	var readable, denied, unreadable []string
	for _, name := range attributes {
		state := probe(filepath.Join(deviceDir, name))
		states[name] = state
		switch state {
		case probeOK:
			readable = append(readable, name)
		case probePermission:
			denied = append(denied, name)
		case probeUnreadable:
			unreadable = append(unreadable, name)
		}
	}

	if len(readable) > 0 {
		if contains(readable, "gpu_metrics") && opts.metricsABI == "" {
			// The file is there and openable but did not decode: a newer
			// driver rolled out a layout this build has no decoder for.
			return ReasonUnknownABI,
				"this kernel publishes a gpu_metrics layout this build does not decode, " +
					"so the numbers are being read from sysfs one attribute at a time instead"
		}
		return ReasonNotPublished, fmt.Sprintf("%s exists but holds no value this build can use", readable[0])
	}
	if len(denied) > 0 {
		return ReasonPermission, fmt.Sprintf("%s is not readable by this user", denied[0])
	}
	if len(unreadable) > 0 {
		return ReasonNotPublished, fmt.Sprintf("%s could not be opened", unreadable[0])
	}

	var required *[2]int
	if opts.metricsABI != "" || !contains(attributes, "gpu_metrics") {
		for _, name := range attributes {
			if v, ok := attributeMinKernel[name]; ok {
				required = &v
				break
			}
		}
	}
	if required != nil && kernelKnown(kernel) && !atLeast(kernel, *required) {
		return ReasonNeedsNewerKernel, fmt.Sprintf("needs Linux %s or newer, this is %s",
			describeVersion(*required), describeVersion(kernel))
	}
	return ReasonNotPublished, fmt.Sprintf("the driver does not create %s on this card", attributes[0])
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// spec is the declarative description of one metric in the report.
type spec struct {
	Key          string
	Label        string
	Read         func(model.AmdgpuMetrics) *string
	Attributes   []string
	DiscreteOnly bool
	Remedy       string
	Extra        string
}

// specs is the basic metric set: one entry per family the panels draw.
// gpu_metrics is listed first for every metric so that a machine with the
// binary table reports its source rather than per-attribute sysfs.
var specs = []spec{
	{
		Key: "gpu_busy", Label: "GPU busy",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.GpuBusyPercent == nil {
				return nil
			}
			s := fmt.Sprintf("%.0f%%", *m.GpuBusyPercent)
			return &s
		},
		Attributes: []string{"gpu_metrics", "gpu_busy_percent"},
	},
	{
		Key: "vram", Label: "VRAM used",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.VRAM.Used == nil {
				return nil
			}
			s := fmt.Sprintf("%d MiB", *m.VRAM.Used/(1024*1024))
			return &s
		},
		Attributes: []string{"gpu_metrics", "mem_vram_used"},
	},
	{
		Key: "gtt", Label: "GTT used",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.GTT.Used == nil {
				return nil
			}
			s := fmt.Sprintf("%d MiB", *m.GTT.Used/(1024*1024))
			return &s
		},
		Attributes: []string{"gpu_metrics", "mem_gtt_used"},
	},
	{
		Key: "sclk", Label: "core clock",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.SCLK.Current == nil {
				return nil
			}
			s := fmt.Sprintf("%d MHz", *m.SCLK.Current)
			return &s
		},
		Attributes: []string{"gpu_metrics", "pp_dpm_sclk"},
	},
	{
		Key: "temp", Label: "temperature",
		Read: func(m model.AmdgpuMetrics) *string {
			for _, t := range m.Temperatures {
				if t.Celsius != nil {
					s := fmt.Sprintf("%.0f C", *t.Celsius)
					return &s
				}
			}
			return nil
		},
		Attributes: []string{"gpu_metrics", "temp1_input"},
	},
	{
		Key: "power", Label: "power",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.Power.DrawW == nil {
				return nil
			}
			s := fmt.Sprintf("%.1f W", *m.Power.DrawW)
			return &s
		},
		Attributes: []string{"gpu_metrics", "power1_average"},
	},
	{
		Key: "fan", Label: "fan",
		Read: func(m model.AmdgpuMetrics) *string {
			if m.Fan.RPM == nil {
				return nil
			}
			s := fmt.Sprintf("%d rpm", *m.Fan.RPM)
			return &s
		},
		Attributes:   []string{"fan1_input"},
		DiscreteOnly: true,
		Extra:        "APUs have no fan; this metric is not expected on an integrated GPU",
	},
}

// Gather builds the report from a real sample. The kernel release is a
// parameter so a test can pin it; the caller passes the host's.
func Gather(snap model.GpuSnapshot, kernelRelease string) Report {
	report := Report{Kernel: kernelRelease}
	kernel := kernelVersion(kernelRelease)
	for _, m := range snap.Devices {
		dev := DeviceReport{Device: m.Device, MetricsABI: metricABI(m)}
		for _, sp := range specs {
			value := sp.Read(m)
			finding := Finding{
				Metric: Metric{
					Key: sp.Key, Label: sp.Label, Source: sourceOf(m), Value: value,
					Attributes: sp.Attributes, DiscreteOnly: sp.DiscreteOnly,
					Remedy: sp.Remedy, Extra: sp.Extra,
				},
				Availability: AvailabilityMissing,
			}
			if value != nil {
				finding.Availability = AvailabilityOK
			} else {
				reason, detail := explain(m.Device.DeviceDir, sp.Attributes, kernel, explainOpts{
					discreteOnly: sp.DiscreteOnly,
					integrated:   m.Device.IsIntegrated(),
					metricsABI:   dev.MetricsABI,
				})
				finding.Reason, finding.Detail = reason, detail
			}
			dev.Findings = append(dev.Findings, finding)
		}
		report.Devices = append(report.Devices, dev)
	}
	return report
}

func metricABI(m model.AmdgpuMetrics) string {
	if m.MetricsABI != nil {
		return *m.MetricsABI
	}
	return ""
}

func sourceOf(m model.AmdgpuMetrics) string {
	if abi := metricABI(m); abi != "" {
		return "gpu_metrics " + abi
	}
	return "sysfs"
}

// Render is the human report: plain ASCII, safe to paste into a bug report.
func (r Report) Render() string {
	lines := []string{"gputop check", strings.Repeat("=", ReportWidth), ""}
	lines = append(lines, "kernel           "+orNotLoaded(r.Kernel))
	lines = append(lines, "amdgpu           "+orNotLoaded(r.Amdgpu))
	lines = append(lines, "go               "+runtime.Version())
	lines = append(lines, "")
	if len(r.System) > 0 {
		lines = append(lines, renderTable(r.System)...)
		lines = append(lines, "")
	}

	if len(r.Devices) == 0 {
		lines = append(lines,
			"No amdgpu device was found under the sysfs DRM class directory.",
			"",
			"Things that produce exactly this:",
			"  - the kernel has no CONFIG_DRM_AMDGPU, or no module built",
			"  - the card is bound to the older 'radeon' driver, not 'amdgpu'",
			"  - the container this runs in does not pass /sys through",
			"")
	}

	for _, d := range r.Devices {
		dev := d.Device
		lines = append(lines, fmt.Sprintf("%s  %s  (%s, 0x%04x)",
			dev.Card, dev.Name, dev.BDF, dev.DeviceID))
		abi := d.MetricsABI
		if abi == "" {
			abi = "none (sysfs only)"
		}
		lines = append(lines, fmt.Sprintf("%-18s%s", "metrics ABI", abi))
		lines = append(lines, "")
		lines = append(lines, renderTable(d.Findings)...)
		lines = append(lines, "")
	}

	missing, total := 0, 0
	for _, d := range r.Devices {
		total += len(d.Findings)
		missing += len(d.Missing())
	}
	summary := fmt.Sprintf("%d/%d metrics available", total-missing, total)
	if missing > 0 {
		summary += fmt.Sprintf("  (%d not available, each explained above)", missing)
	}
	lines = append(lines, summary)
	if len(r.Warnings) > 0 {
		lines = append(lines, "")
		lines = append(lines, "warnings:")
		for _, warning := range r.Warnings {
			lines = append(lines, "  - "+warning)
		}
	}
	return strings.Join(lines, "\n")
}

func orNotLoaded(s string) string {
	if s == "" {
		return "not loaded"
	}
	return s
}

// renderTable renders findings as an aligned table, with an explanation
// under anything missing. A note is only attached to a metric that is not
// available: qualifying a present reading would read as doubt about a
// number the machine just produced.
func renderTable(findings []Finding) []string {
	const stateWidth, sourceWidth = 10, 26
	keyWidth := 8
	for _, f := range findings {
		if n := len(f.Metric.Label); n > keyWidth {
			keyWidth = n
		}
	}
	if keyWidth > 26 {
		keyWidth = 26
	}
	lines := []string{
		fmt.Sprintf("%-*s  %-*s  %-*s  VALUE", keyWidth, "METRIC", stateWidth, "STATUS", sourceWidth, "SOURCE"),
		fmt.Sprintf("%s  %s  %s  -----", strings.Repeat("-", keyWidth), strings.Repeat("-", stateWidth), strings.Repeat("-", sourceWidth)),
	}
	for _, f := range findings {
		value := "-"
		if f.OK() {
			value = *f.Metric.Value
		}
		lines = append(lines, fmt.Sprintf("%-*s  %-*s  %-*s  %s",
			keyWidth, clip(f.Metric.Label, keyWidth),
			stateWidth, string(f.Availability),
			sourceWidth, clip(f.Metric.Source, sourceWidth),
			value))
		if f.OK() {
			continue
		}
		pad := strings.Repeat(" ", keyWidth+stateWidth+4)
		note := string(f.Reason)
		if f.Detail != "" {
			if note != "" {
				note += ": " + f.Detail
			} else {
				note = f.Detail
			}
		}
		if note != "" {
			lines = append(lines, pad+"-> "+note)
		}
		if f.Metric.Extra != "" {
			lines = append(lines, pad+"-> "+f.Metric.Extra)
		}
		if f.Metric.Remedy != "" {
			lines = append(lines, pad+"fix: "+f.Metric.Remedy)
		}
	}
	return lines
}

func clip(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	return string(runes[:width])
}
