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
	"github.com/gputop/gputop-go/internal/sessionlog"
)

// version is a variable, not a constant, so the release stage can stamp it
// with -ldflags "-X main.version=...". The default is the version this tree
// documents; scripts/check_packaging.sh keeps the man page and PKGBUILD in
// step with it.
var version = "1.2.0"

func main() {
	status, err := runWith(os.Args[1:], os.Stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gputop: %v\n", err)
		os.Exit(1)
	}
	os.Exit(status)
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
	configPath := ""
	fs.StringVar(&configPath, "config", "", "configuration file to load instead of the default locations")
	fs.StringVar(&configPath, "c", "", "shorthand for --config")
	interval := 0.0
	fs.Float64Var(&interval, "interval", 0, "sampling interval in seconds, overriding interval_ms")
	fs.Float64Var(&interval, "i", 0, "shorthand for --interval")
	drmRoot := fs.String("drm-root", "", "sysfs DRM class directory (default /sys/class/drm)")
	procRoot := fs.String("proc-root", "", "procfs mount point (default /proc)")
	noProcesses := fs.Bool("no-processes", false, "skip the /proc scan, the most expensive part of a sample")
	kind := fs.String("kind", "", "force every device to be classified as igpu or dgpu")
	theme := fs.String("theme", "", "colour theme: default, dracula, gruvbox, monochrome or radeon")
	blocks := fs.Bool("blocks", false, "force the per-block panel on, overriding blocks.enabled")
	noBlocks := fs.Bool("no-blocks", false, "never start radeontop, even when it is installed")
	checkMode := fs.Bool("check", false, "report what this machine can show and why anything is missing")
	asJSON := fs.Bool("json", false, "emit --check as JSON instead of text (ignored without --check)")
	logPath := fs.String("log", "", "record the session to this file (.csv, .json[.zst])")
	noColor := fs.Bool("no-color", false, "never emit colour, even on a colour terminal")
	if err := fs.Parse(args); err != nil {
		return 1, err
	}
	if *blocks && *noBlocks {
		return 1, fmt.Errorf("--blocks and --no-blocks are mutually exclusive")
	}
	if *theme != "" && !model.ValidTheme(*theme) {
		return 1, fmt.Errorf("--theme %q is not one of: %s",
			*theme, strings.Join(model.ThemeNames, ", "))
	}
	if *kind != "" && *kind != "auto" && *kind != "igpu" && *kind != "dgpu" {
		return 1, fmt.Errorf("--kind %q is not auto, igpu or dgpu", *kind)
	}
	if *logPath != "" && (*checkMode || *dump || *devices) {
		return 1, fmt.Errorf("--log records the interface; it cannot be combined with --check, --dump or --devices")
	}
	if *showVersion {
		if _, err := fmt.Fprintf(stdout, "gputop %s\n", version); err != nil {
			return 1, err
		}
		return 0, nil
	}

	cfg := config.Load(configPath)
	if *noColor {
		cfg.UI.NoColor = true
	}
	if *theme != "" {
		cfg.UI.Theme = *theme
	}
	// The flag wins over the config; --no-blocks is the only way to override a
	// config that turned the panel on.
	blocksEnabled := cfg.Blocks.Enabled
	if *blocks {
		blocksEnabled = true
	}
	if *noBlocks {
		blocksEnabled = false
	}
	pool := readers.NewRadeontopPool(readers.RadeontopOptions{
		Enabled:   blocksEnabled,
		Binary:    cfg.Blocks.Binary,
		Ticks:     cfg.Blocks.Ticks,
		IntervalS: cfg.Blocks.IntervalS,
		DrmRoot:   cfg.Blocks.DrmRoot,
		Restart:   cfg.Blocks.Restart,
	})
	in := cli.OneShot{
		Config: &cfg,
		Pretty: *pretty,
		Blocks: pool,
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
	if interval > 0 {
		in.Sampler.IntervalS = interval
	}
	if k := strings.TrimSpace(*kind); k != "" && k != "auto" {
		deviceKind := model.DeviceKind(k)
		in.Sampler.KindOverride = &deviceKind
	}

	switch {
	case *checkMode:
		return cli.Check(stdout, in, cli.CheckOptions{
			JSON:             *asJSON,
			ProcRoot:         *procRoot,
			ProcessesScanned: cfg.Process.Show && !*noProcesses,
			BlocksEnabled:    blocksEnabled,
			BlocksBinary:     cfg.Blocks.Binary,
		})
	case *devices:
		return cli.Devices(stdout, in)
	case *dump:
		return cli.Dump(stdout, in)
	}
	return 0, runInterface(in, *logPath)
}

// runInterface opens the recording, if one was asked for, and hands the
// terminal to the event loop. A log that cannot be opened is a warning, not a
// refusal to start: the monitor is still the reason the user ran it.
func runInterface(in cli.OneShot, logPath string) error {
	var log *sessionlog.SessionLog
	if logPath != "" {
		log = sessionlog.OpenLog(logPath, in.Config.Log.ZstdLevel)
		if !log.IsOpen() {
			warning := "recording disabled: " + string(log.Problem) + " (" + log.Detail + ")"
			in.Config.Warnings = append(in.Config.Warnings, warning)
			fmt.Fprintln(os.Stderr, "gputop: "+warning)
			log = nil
		}
	}
	return cli.RunTUI(cli.TUI{
		Config:  *in.Config,
		Sampler: in.Sampler,
		Blocks:  in.Blocks,
		Log:     log,
	})
}
