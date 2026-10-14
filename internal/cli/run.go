package cli

import (
	"io"
	"time"

	"github.com/gputop/gputop-go/internal/config"
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
