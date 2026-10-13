package sampler

import (
	"context"
	"fmt"
	"time"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/readers"
)

// monoStart anchors monotonic timestamps, the Go counterpart of Python's
// time.monotonic_ns().
var monoStart = time.Now()

// monotonicNs returns nanoseconds from a fixed monotonic origin.
func monotonicNs() model.Nanoseconds {
	return model.Nanoseconds(time.Since(monoStart))
}

// msSince converts an elapsed duration to fractional milliseconds.
func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000.0
}

// run is the tick goroutine body: tick until the context is cancelled,
// sleeping on a fixed-rate deadline grid.
//
// The period is re-read from s.interval at the top of every iteration rather
// than captured once: a cached copy would keep sampling at the rate the thread
// started with, so set-interval would report the new value back to the caller
// while the loop carried on at the old one. The sleep waits on the wake
// channel as well as the deadline, so set-interval, request-tick and stop cut
// it short instead of waiting out the remainder of the old period.
func (s *Sampler) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	period := s.currentInterval()
	nextDeadline := time.Now()
	for {
		// Drain the wake channel (arm it) before the tick, so a change made
		// during the tick is not lost between here and the wait.
		drain(s.wake)
		current := s.currentInterval()
		if current != period {
			period = current
			nextDeadline = time.Now()
		}
		snap, ok := s.runTick()
		if ok {
			s.publish(snap)
		}
		nextDeadline = nextDeadline.Add(period)
		now := time.Now()
		if nextDeadline.Before(now) {
			// The tick overran its slot. The missed slots are dropped, not
			// replayed: waiting out a negative delay would run the next ticks
			// back to back until the grid caught up, a full /proc scan per
			// missed period. The grid's phase is kept, so the rate does not
			// drift.
			skipped := int64(now.Sub(nextDeadline)/period) + 1
			nextDeadline = nextDeadline.Add(time.Duration(skipped) * period)
			now = time.Now()
		}
		delay := nextDeadline.Sub(now)
		if delay < 0 {
			delay = 0
		}
		if delay > period {
			delay = period
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// drain empties a channel without blocking.
func drain(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// currentInterval reads the live sampling period.
func (s *Sampler) currentInterval() time.Duration {
	return time.Duration(s.interval.Load().(float64) * float64(time.Second))
}

// tick performs one sample and returns the immutable result.
//
// It is the single owner of every mutable field it touches -- sequence,
// history, stats, seenWarnings and the collector's delta baseline -- so it is
// only ever called from the tick goroutine or from SampleOnce, never both at
// once (SampleOnce's guard enforces that).
//
// The readers are total, but a device or the procfs scan is still guarded: a
// panic from one card degrades to a bounded warning plus a read-error count
// rather than taking the whole tick down. A monitor that stops because one card
// misbehaved is worse than one that reports the rest -- this is the Go
// counterpart of the reference's per-device try/except.
func (s *Sampler) tick() model.GpuSnapshot {
	started := time.Now()
	s.sequence++
	opts := *s.opts.Load()
	warnings := make([]string, 0, MaxWarnings)
	var durations []float64
	devices := *s.devices.Load()

	// Polled before the devices are read so a block sample already waiting is
	// attached to the tick that will display it, not one tick late.
	blocksStatus := s.blocks.Poll()

	samples := make([]model.AmdgpuMetrics, 0, len(devices))
	for i := range devices {
		device := devices[i]
		deviceStart := time.Now()
		m, err := s.safeSampleDevice(device, opts)
		if err != nil {
			s.recordWarning(device.Card+": "+err.Error(), &warnings)
		}
		samples = append(samples, m)
		durations = append(durations, msSince(deviceStart))
	}

	var processes []model.GpuProcess
	visible, total := 0, 0
	if opts.CollectProcesses {
		procStart := time.Now()
		collected, vis, tot, err := s.safeCollect()
		if err != nil {
			s.recordWarning("procfs: "+err.Error(), &warnings)
		}
		processes, visible, total = collected, vis, tot
		durations = append(durations, msSince(procStart))
	}

	elapsedMs := msSince(started)
	s.stats = advance(s.stats, durations, elapsedMs)
	publishStats(s)

	snap := model.GpuSnapshot{
		Sequence:            s.sequence,
		TimestampNs:         monotonicNs(),
		Devices:             samples,
		Processes:           processes,
		VisibleProcessCount: visible,
		TotalProcessCount:   total,
		Stats:               s.stats,
		Source: model.SourceReport{
			MetricsABI: dominantABI(samples),
			PerMetric:  map[string]string{},
		},
		Warnings:     warnings,
		BlocksStatus: blocksStatus,
		BlocksHint:   s.blocksHint(blocksStatus),
		WallClockNs:  time.Now().UnixNano(),
	}
	// Recording here rather than at the publish site means SampleOnce
	// populates history too.
	s.history.Append(snap)
	items := s.history.Items()
	s.historyView.Store(&items)
	return snap
}

// safeSampleDevice reads every source for one device through the readers merge
// layer and attaches the latest block sample, converting a panic into an error
// so a single misbehaving card cannot stop the tick. On error it returns the
// device with an otherwise-empty (all-nil) sample so the row still appears.
func (s *Sampler) safeSampleDevice(
	device model.AmdgpuDevice, opts SamplerOptions,
) (m model.AmdgpuMetrics, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v: %v", panicType(r), r)
			m = model.AmdgpuMetrics{
				Device:    device,
				Sequence:  s.sequence,
				IntervalS: opts.IntervalS,
				Power:     model.Power{Source: "hwmon"},
				SCLK:      model.Clock{Source: "none"},
				MCLK:      model.Clock{Source: "none"},
				SocClk:    model.Clock{Source: "none"},
			}
		}
	}()
	m = readers.SampleDevice(device, s.sequence, monotonicNs(), opts.IntervalS)
	m.Blocks = s.blocks.BlocksFor(device.Index)
	return m, nil
}

// safeCollect runs the procfs scan, converting a panic into an error so a
// wedged /proc read cannot stop the tick.
func (s *Sampler) safeCollect() (
	processes []model.GpuProcess, visible, total int, err error,
) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v: %v", panicType(r), r)
		}
	}()
	processes, visible, total = s.collector.Collect(monotonicNs())
	return processes, visible, total, nil
}

// panicType names the concrete type of a recovered value, mirroring Python's
// type(error).__name__ in the reference's warning text. %T reports the dynamic
// type whether the recovered value is an error or a bare value.
func panicType(r any) string {
	return fmt.Sprintf("%T", r)
}

// publish offers a snapshot to the channel, dropping the oldest when full so
// the consumer always has the newest available and the tick never blocks.
func (s *Sampler) publish(snap model.GpuSnapshot) {
	for {
		select {
		case s.out <- snap:
			return
		default:
		}
		select {
		case <-s.out:
		default:
		}
	}
}

// publishStats rebinds the immutable stats view for cross-goroutine readers.
func publishStats(s *Sampler) {
	stats := s.stats
	s.statsView.Store(&stats)
}
