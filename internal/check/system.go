package check

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/gputop/gputop-go/internal/model"
)

// AvailabilityDisabled is a metric the run deliberately did not collect.
// It is distinct from missing: nothing is wrong, and saying "missing" would
// read as a broken machine.
const AvailabilityDisabled Availability = "disabled"

// Additional reasons the system findings can carry.
const (
	ReasonDisabled       Reason = "disabled"
	ReasonNotInstalled   Reason = "not installed"
	ReasonNotExercised   Reason = "not exercised"
	ReasonNoClients      Reason = "no GPU clients"
	ReasonHidden         Reason = "hidden by permissions"
	ReasonUnsupported    Reason = "unsupported card"
	ReasonNeedsNewDriver Reason = "needs a newer driver"
)

// SystemOptions are the inputs the system-level findings need. They are
// separate from the device sample because they are about the machine, not one
// GPU, and because --check must never start radeontop to answer them.
type SystemOptions struct {
	// DrmRoot is the sysfs DRM class directory in use, printed by the report.
	DrmRoot string
	// DevRoot overrides where render nodes are listed. Empty derives it from
	// DrmRoot, which is what a normal run does; tests point it at a temp dir.
	DevRoot          string
	ProcRoot         string
	ProcessesScanned bool
	BlocksEnabled    bool
	BlocksBinary     string
	// BlocksStatus is nil when radeontop was never run, which is the normal
	// case for --check: an unstarted pool's status is not evidence.
	BlocksStatus *model.BlocksStatus
	BlocksHint   string
}

// GatherSystem builds the full report: device findings plus the machine-level
// ones, the loaded amdgpu module version and the kernel paths in use.
func GatherSystem(snap model.GpuSnapshot, kernelRelease string, opts SystemOptions) Report {
	report := Gather(snap, kernelRelease)
	report.Amdgpu = AmdgpuVersion()
	report.DrmRoot = opts.DrmRoot
	if report.DrmRoot == "" {
		report.DrmRoot = DefaultDrmRoot
	}
	report.ProcRoot = opts.ProcRoot
	if report.ProcRoot == "" {
		report.ProcRoot = DefaultProcRoot
	}
	devices := make([]model.AmdgpuDevice, 0, len(snap.Devices))
	for _, m := range snap.Devices {
		devices = append(devices, m.Device)
	}
	devRoot := opts.DevRoot
	if devRoot == "" {
		devRoot = devRootFor(report.DrmRoot)
	}
	report.RenderNodes = renderNodes(devices, devRoot)
	report.System = SystemFindings(snap, kernelVersion(kernelRelease), opts)
	return report
}

// devRootFor mirrors the reference: a DRM class directory is swapped for the
// device tree beside it, and anything else falls back to /dev/dri.
func devRootFor(drmRoot string) string {
	if strings.HasPrefix(drmRoot, DefaultDrmRoot) {
		return strings.Replace(drmRoot, DefaultDrmRoot, DefaultDevRoot, 1)
	}
	return DefaultDevRoot
}

// renderNodes lists the device nodes that belong to the discovered cards.
// Entries belonging to another driver are not this report's business, and an
// unreadable directory is an absence rather than an error.
func renderNodes(devices []model.AmdgpuDevice, devRoot string) []string {
	entries, err := os.ReadDir(devRoot)
	if err != nil {
		return nil
	}
	wanted := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		if device.Render != nil {
			wanted[*device.Render] = struct{}{}
		}
	}
	nodes := make([]string, 0, len(wanted))
	for _, entry := range entries {
		if _, ok := wanted[entry.Name()]; ok {
			nodes = append(nodes, entry.Name())
		}
	}
	sort.Strings(nodes)
	return nodes
}

// AmdgpuVersion reads the loaded module version from sysfs. Asking the kernel
// costs no subprocess and works inside a container that has sysfs but not
// modinfo; an unreadable file is simply an unknown version.
func AmdgpuVersion() string {
	data, err := os.ReadFile("/sys/module/amdgpu/version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// SystemFindings evaluates the questions that are about the machine rather
// than one GPU: can the process table be read, and is the optional per-block
// panel available?
func SystemFindings(snap model.GpuSnapshot, kernel [2]int, opts SystemOptions) []Finding {
	return []Finding{processFinding(snap, kernel, opts), blocksFinding(opts)}
}

// processFinding describes the per-process table from a real /proc scan.
//
// The interesting question when the table is empty is "is nothing running, or
// can I not see it?", and those need different answers, so the ownership scan
// decides between them rather than the client count alone.
func processFinding(snap model.GpuSnapshot, kernel [2]int, opts SystemOptions) Finding {
	procRoot := opts.ProcRoot
	if procRoot == "" {
		procRoot = "/proc"
	}
	clients := len(snap.Processes)
	total, hidden := Ownership(procRoot)
	value := "no processes visible"
	if total > 0 {
		value = fmt.Sprintf("%d GPU client%s of %d process%s",
			clients, plural(clients, "", "s"), total, plural(total, "", "es"))
	}
	metric := Metric{
		Key: "processes", Label: "Process table",
		Source: procRoot + "/<pid>/fdinfo", Value: &value,
	}
	if !opts.ProcessesScanned {
		metric.Value = nil
		return Finding{
			Metric: metric, Availability: AvailabilityDisabled, Reason: ReasonDisabled,
			Detail: "the /proc scan was switched off, so this table was not read and says " +
				"nothing about whether the GPU is busy",
		}
	}
	if kernelKnown(kernel) && !atLeast(kernel, minKernelFDINFO) {
		metric.Value = nil
		return Finding{
			Metric: metric, Availability: AvailabilityMissing, Reason: ReasonNeedsNewerKernel,
			Detail: fmt.Sprintf("per-process accounting needs Linux %s or newer, this is %s",
				describeVersion(minKernelFDINFO), describeVersion(kernel)),
		}
	}
	if clients > 0 {
		return Finding{Metric: metric, Availability: AvailabilityOK}
	}
	if total == 0 {
		metric.Value = nil
		return Finding{
			Metric: metric, Availability: AvailabilityMissing, Reason: ReasonNoClients,
			Detail: procRoot + " lists no processes at all",
		}
	}
	if hidden > 0 {
		metric.Value = nil
		return Finding{
			Metric: metric, Availability: AvailabilityMissing, Reason: ReasonHidden,
			Detail: fmt.Sprintf("%d of %d processes belong to another user and cannot be "+
				"inspected; gputop never asks for privileges to read them", hidden, total),
		}
	}
	return Finding{
		Metric: metric, Availability: AvailabilityOK, Reason: ReasonNoClients,
		Detail: fmt.Sprintf("none of the %d visible processes holds a /dev/dri descriptor, "+
			"which is what an idle GPU looks like; ROCm, Vulkan and OpenGL workloads appear "+
			"here as soon as they run", total),
	}
}

// blocksFinding reports the optional panel without ever running it.
func blocksFinding(opts SystemOptions) Finding {
	binary := opts.BlocksBinary
	if binary == "" {
		binary = "radeontop"
	}
	metric := Metric{
		Key: "blocks", Label: "Per-block panel", Source: binary,
		Extra: "add --blocks or blocks.enabled = true to use it",
	}
	if !opts.BlocksEnabled {
		return Finding{Metric: metric, Availability: AvailabilityDisabled, Reason: ReasonDisabled}
	}
	if opts.BlocksStatus == nil {
		resolved := resolveBinary(binary)
		if resolved == "" {
			return Finding{
				Metric: metric, Availability: AvailabilityMissing, Reason: ReasonNotInstalled,
				Detail: "no " + binary + " binary on this system (install it, or set blocks.binary)",
			}
		}
		value := "installed"
		metric.Source, metric.Value = resolved, &value
		metric.Extra = ""
		return Finding{
			Metric: metric, Availability: AvailabilityDisabled, Reason: ReasonNotExercised,
			Detail: "radeontop is installed but --check never runs it, so whether it can " +
				"read this card is untested; enable the blocks panel to find out",
		}
	}
	status := *opts.BlocksStatus
	if status.OK() {
		value := "available"
		metric.Value = &value
		return Finding{Metric: metric, Availability: AvailabilityOK}
	}
	reason := ReasonNotPublished
	switch status {
	case model.BlocksMissing:
		reason = ReasonNotInstalled
	case model.BlocksNotPermitted:
		reason = ReasonPermission
	case model.BlocksUnsupported:
		reason = ReasonUnsupported
	case model.BlocksFailed:
		reason = ReasonNeedsNewDriver
	case model.BlocksDisabled:
		reason = ReasonDisabled
	case model.BlocksOK:
	}
	detail := opts.BlocksHint
	if detail == "" {
		detail = status.Hint()
	}
	return Finding{Metric: metric, Availability: AvailabilityMissing, Reason: reason, Detail: detail}
}

// Ownership counts (processes, hidden) under procRoot. hidden is the number of
// process directories this user cannot inspect, which is the honest version of
// "you may not see every client's utilisation". It costs one stat per process,
// which is why it is asked once here rather than on every sampler tick.
func Ownership(procRoot string) (total, hidden int) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return 0, 0
	}
	uid := os.Geteuid()
	for _, entry := range entries {
		if _, convErr := strconv.Atoi(entry.Name()); convErr != nil {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			if os.IsNotExist(statErr) {
				// The process exited between the scan and the stat, which on a
				// busy machine is routine and says nothing about ownership.
				continue
			}
			total++
			continue
		}
		total++
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != uid {
			hidden++
		}
	}
	return total, hidden
}

// resolveBinary answers "is it installed" with a PATH lookup and a stat. No
// process is started and no privileges are needed.
func resolveBinary(name string) string {
	if strings.ContainsRune(name, os.PathSeparator) {
		if info, err := os.Stat(name); err == nil && !info.IsDir() {
			return name
		}
		return ""
	}
	found, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return found
}

// plural picks the suffix for a count: processes, but one process.
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
