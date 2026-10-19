package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/readers"
	"github.com/gputop/gputop-go/internal/sampler"
)

// TestDumpBlocksPanel pins the --dump wiring for the optional radeontop panel:
// a disabled pool must report "disabled" without spawning anything, and an
// enabled one must hand its parsed blocks to the snapshot -- because --dump has
// to reproduce everything the interface shows, panel included.
func TestDumpBlocksPanel(t *testing.T) {
	cfg := config.Load(filepath.Join(parityDir, "gputop.yaml"))
	base := sampler.SamplerOptions{
		IntervalS:        sampler.MinIntervalS,
		CollectProcesses: true,
		DrmRoot:          filepath.Join(parityDir, "tree", "sys", "class", "drm"),
		ProcRoot:         filepath.Join(parityDir, "tree", "proc"),
	}

	disabled := readers.NewRadeontopPool(readers.RadeontopOptions{})
	var disabledOut bytes.Buffer
	if status, err := Dump(&disabledOut, OneShot{Config: &cfg, Sampler: base, Blocks: disabled}); err != nil || status != 0 {
		t.Fatalf("disabled blocks: status %d, err %v", status, err)
	}
	if !strings.Contains(disabledOut.String(), `"status":"disabled"`) {
		t.Errorf("disabled dump did not report the panel as disabled:\n%s", disabledOut.String())
	}

	// A stand-in radeontop, so the happy path is exercised without the real
	// binary and without waiting out the start-up grace period.
	script := filepath.Join(t.TempDir(), "radeontop-stand-in")
	body := "#!/bin/sh\nwhile true; do\n" +
		"  echo '100.0: bus ff, gpu 42.00%, cb 7.00%, sclk 25.00% 2.100ghz'\n" +
		"  sleep 0.05\ndone\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stand-in: %v", err)
	}
	enabled := readers.NewRadeontopPool(readers.RadeontopOptions{Enabled: true, Binary: script})
	var enabledOut bytes.Buffer
	if status, err := Dump(&enabledOut, OneShot{Config: &cfg, Sampler: base, Blocks: enabled}); err != nil || status != 0 {
		t.Fatalf("enabled blocks: status %d, err %v", status, err)
	}
	if !strings.Contains(enabledOut.String(), `"status":"ok"`) {
		t.Errorf("enabled dump did not report the panel as ok:\n%s", enabledOut.String())
	}
	if !strings.Contains(enabledOut.String(), `"gpu":42`) {
		t.Errorf("enabled dump did not carry the parsed block:\n%s", enabledOut.String())
	}
}
