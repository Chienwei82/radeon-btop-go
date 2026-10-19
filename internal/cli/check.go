package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gputop/gputop-go/internal/check"
	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/sampler"
)

// CheckOptions are the inputs --check needs beyond the sampling options.
type CheckOptions struct {
	// JSON emits the machine-readable report instead of the text one.
	JSON bool
	// ProcRoot is the procfs mount the ownership scan reads.
	ProcRoot string
	// ProcessesScanned is false when --no-processes turned the scan off. The
	// report then says "nobody looked" instead of "the GPU is idle".
	ProcessesScanned bool
	// BlocksEnabled and BlocksBinary describe the optional panel as the
	// configuration stands. --check never starts radeontop.
	BlocksEnabled bool
	BlocksBinary  string
	// KernelRelease pins the release for a test; empty reads the host's.
	KernelRelease string
}

// Check writes the "what can this machine show me, and what is missing?"
// report and returns an exit status.
//
// The report is built from a real two-sample run, so it describes what the
// interface would draw right now rather than what a list of filenames says
// should be there. The wait between the samples is what per-engine
// utilisation is derived from, and is kept as short as the sampler allows.
func Check(w io.Writer, in OneShot, opts CheckOptions) (int, error) {
	s := sampler.New(in.Sampler)
	devices := s.Discover()
	warnings := StartupWarnings(in.Config, s.FilterWarning())

	procRoot := opts.ProcRoot
	if procRoot == "" {
		procRoot = s.Options().ProcRoot
	}
	if procRoot == "" {
		procRoot = "/proc"
	}
	system := check.SystemOptions{
		ProcRoot:         procRoot,
		ProcessesScanned: opts.ProcessesScanned,
		BlocksEnabled:    opts.BlocksEnabled,
		BlocksBinary:     opts.BlocksBinary,
	}
	kernel := opts.KernelRelease
	if kernel == "" {
		kernel = kernelRelease(procRoot)
	}

	if len(devices) == 0 {
		report := check.GatherSystem(model.GpuSnapshot{}, kernel, system)
		report.Warnings = append(warnings, "no amdgpu device found under "+s.Options().DrmRoot)
		return ExitNoDevice, emitCheck(w, report, opts.JSON)
	}

	if _, err := s.SampleOnce(); err != nil {
		return 1, err
	}
	wait := s.Options().IntervalS
	if wait > 0.5 {
		wait = 0.5
	}
	time.Sleep(time.Duration(wait * float64(time.Second)))
	snap, err := s.SampleOnce()
	if err != nil {
		return 1, err
	}
	report := check.GatherSystem(snap, kernel, system)
	report.Warnings = append(report.Warnings, warnings...)
	return 0, emitCheck(w, report, opts.JSON)
}

// emitCheck writes the report in whichever shape was asked for.
func emitCheck(w io.Writer, report check.Report, asJSON bool) error {
	if asJSON {
		return Encode(w, report.ToMap(), true)
	}
	_, err := io.WriteString(w, report.Render()+"\n")
	return err
}

// kernelRelease reads the host's release, falling back to an unreadable value
// rather than inventing a version: the report prints "unknown" only by
// omitting the comparison, never by naming a kernel that is not running.
func kernelRelease(procRoot string) string {
	data, err := os.ReadFile(filepath.Join(procRoot, "sys", "kernel", "osrelease"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
