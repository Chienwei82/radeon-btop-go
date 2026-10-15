package cli

import (
	"io"
	"time"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/readers"
	"github.com/gputop/gputop-go/internal/sampler"
)

// ExitNoDevice is the status used when no AMD GPU is present, so a script can
// tell "no hardware" apart from "gputop is broken".
const ExitNoDevice = 3

// OneShot carries the inputs a headless command needs: the sampling options,
// the loaded configuration (nil when there is none) and the output preference.
type OneShot struct {
	Sampler sampler.SamplerOptions
	Config  *config.Config
	Pretty  bool
	// Blocks, when non-nil, is the supervised radeontop pool. Only --dump
	// starts it, and only when the panel was asked for: an unconditional wait
	// would add seconds to every dump on a machine that has radeontop
	// installed and does not use this feature.
	Blocks *readers.RadeontopPool
}

// Devices writes the discovered-GPU list as JSON and returns an exit status.
//
// It runs discovery through a Sampler rather than a bare call because the
// gpu.devices filter lives there: listing every card while the interface would
// watch one of them answers a different question than the one that was asked.
func Devices(w io.Writer, in OneShot) (int, error) {
	s := sampler.New(in.Sampler)
	devices := s.Discover()
	payload := deviceListJSON{
		Count:    len(devices),
		Devices:  make([]deviceSummaryJSON, 0, len(devices)),
		Warnings: StartupWarnings(in.Config, s.FilterWarning()),
	}
	for _, d := range devices {
		payload.Devices = append(payload.Devices, toDeviceSummary(d))
	}
	if err := Encode(w, payload, in.Pretty); err != nil {
		return 1, err
	}
	if len(devices) == 0 {
		return ExitNoDevice, nil
	}
	return 0, nil
}

// Dump writes one JSON snapshot and returns an exit status.
//
// Engine utilisation comes from a delta between two samples, so a dump needs a
// real window: sample once to establish the baseline, wait one interval (capped
// at two seconds so a dump never stalls a pipeline), then sample again. Without
// the wait every engine would read 0%.
func Dump(w io.Writer, in OneShot) (int, error) {
	s := sampler.New(in.Sampler)
	if in.Blocks != nil {
		s = s.WithBlocks(in.Blocks)
	}
	devices := s.Discover()
	warnings := StartupWarnings(in.Config, s.FilterWarning())
	if len(devices) == 0 {
		payload := noDevicesJSON{
			Devices:   []any{},
			Processes: []any{},
			Warnings:  append(warnings, "no amdgpu device found under "+s.Options().DrmRoot),
		}
		if err := Encode(w, payload, in.Pretty); err != nil {
			return 1, err
		}
		return ExitNoDevice, nil
	}
	if in.Blocks != nil && in.Blocks.Enabled() {
		// The panel must have real numbers the first time it renders, and a
		// one-shot dump has no sampling goroutine to accumulate the wait for
		// it. A timeout is not an error: the snapshot still goes out, with the
		// status saying why the blocks are missing.
		in.Blocks.Start()
		defer in.Blocks.Stop()
		readers.WaitForFirstSample(in.Blocks,
			time.Duration(readers.StartupGraceS*float64(time.Second)))
	}
	if _, err := s.SampleOnce(); err != nil {
		return 1, err
	}
	wait := s.Options().IntervalS
	if wait > 2.0 {
		wait = 2.0
	}
	time.Sleep(time.Duration(wait * float64(time.Second)))
	snap, err := s.SampleOnce()
	if err != nil {
		return 1, err
	}
	if err := Encode(w, Snapshot(snap, in.Config), in.Pretty); err != nil {
		return 1, err
	}
	return 0, nil
}
