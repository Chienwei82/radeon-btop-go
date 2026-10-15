// Command gputop is a btop-style terminal monitor for AMD Radeon GPUs
// driven by the amdgpu kernel driver.
//
// With no arguments it starts the terminal interface; see --help for the
// headless diagnostic modes (--dump, --devices, --check).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gputop/gputop-go/internal/cli"
	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/readers"
	"github.com/gputop/gputop-go/internal/sampler"
)

const version = "0.1.0"

func main() {
	status, err := runWith(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gputop: %v\n", err)
		os.Exit(1)
	}
	os.Exit(status)
}

// run keeps the single-error signature the tests use; the process exit status
// is handled by main so a command can return "no device" without an error.
func run(args []string) error {
	_, err := runWith(args, os.Stdout)
	return err
}

// runWith parses the command line and dispatches to the headless modes. It
// returns the process exit status plus a real error (a status of 0 or
// cli.ExitNoDevice is not an error).
func runWith(args []string, stdout io.Writer) (int, error) {
	fs := flag.NewFlagSet("gputop", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print version and exit")
	dump := fs.Bool("dump", false, "write one JSON snapshot to stdout and exit")
	devices := fs.Bool("devices", false, "list discovered GPUs as JSON and exit")
	pretty := fs.Bool("pretty", false, "indent --dump output for human reading")
	configPath := fs.String("config", "", "configuration file to load instead of the default locations")
	interval := fs.Float64("interval", 0, "sampling interval in seconds, overriding general.interval_ms")
	drmRoot := fs.String("drm-root", "", "sysfs DRM class directory (default /sys/class/drm)")
	procRoot := fs.String("proc-root", "", "procfs mount point (default /proc)")
	noProcesses := fs.Bool("no-processes", false, "skip the /proc scan, the most expensive part of a sample")
	kind := fs.String("kind", "", "force every device to be classified as igpu or dgpu")
	blocks := fs.Bool("blocks", false, "force the per-block panel on, overriding blocks.enabled")
	noBlocks := fs.Bool("no-blocks", false, "never start radeontop, even when it is installed")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	if *blocks && *noBlocks {
		return 1, fmt.Errorf("--blocks and --no-blocks are mutually exclusive")
	}
	if *showVersion {
		if _, err := fmt.Fprintf(stdout, "gputop %s\n", version); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if !*dump && !*devices {
		return 1, fmt.Errorf("TUI not implemented yet (Phase 2); see --dump, --devices or --version")
	}

	cfg := config.Load(*configPath)
	// The flag wins over the config; --no-blocks is the only way to override a
	// config that turned the panel on.
	blocksEnabled := cfg.Blocks.Enabled
	if *blocks {
		blocksEnabled = true
	}
	if *noBlocks {
		blocksEnabled = false
	}
	in := cli.OneShot{
		Config: &cfg,
		Pretty: *pretty,
		Blocks: readers.NewRadeontopPool(readers.RadeontopOptions{
			Enabled:   blocksEnabled,
			Binary:    cfg.Blocks.Binary,
			Ticks:     cfg.Blocks.Ticks,
			IntervalS: cfg.Blocks.IntervalS,
			DrmRoot:   cfg.Blocks.DrmRoot,
			Restart:   cfg.Blocks.Restart,
		}),
		Sampler: sampler.SamplerOptions{
			IntervalS:        cfg.IntervalS(),
			HistoryLength:    cfg.HistoryLength(),
			CollectProcesses: cfg.Process.Show && !*noProcesses,
			DrmRoot:          *drmRoot,
			ProcRoot:         *procRoot,
			OnlyDevices:      cfg.Gpu.Devices,
			NameOverrides:    cfg.Gpu.Names,
		},
	}
	if *interval > 0 {
		in.Sampler.IntervalS = *interval
	}
	if k := strings.TrimSpace(*kind); k != "" && k != "auto" {
		deviceKind := model.DeviceKind(k)
		in.Sampler.KindOverride = &deviceKind
	}

	if *devices {
		return cli.Devices(stdout, in)
	}
	return cli.Dump(stdout, in)
}
