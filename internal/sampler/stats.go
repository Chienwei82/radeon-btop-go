package sampler

import (
	"github.com/gputop/gputop-go/internal/model"
)

// runTick wraps a tick with a last-resort recover. The per-device and procfs
// guards already contain the faults the readers can raise; this covers
// anything outside them (for example a blocks source), so a tick that dies
// still counts as a tick error and the loop keeps its phase instead of
// unwinding the goroutine.
func (s *Sampler) runTick() (snap model.GpuSnapshot, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			s.stats = withTickError(s.stats)
			publishStats(s)
			ok = false
		}
	}()
	return s.tick(), true
}

// advance folds this tick's timings into the rolling counters. durations holds
// one entry per sub-system read; an empty slice means the whole tick is one
// sample.
func advance(stats model.SamplerStats, durations []float64, elapsedMs float64) model.SamplerStats {
	samples := durations
	if len(samples) == 0 {
		samples = []float64{elapsedMs}
	}
	sum := 0.0
	for _, d := range samples {
		sum += d
	}
	avg := stats.AvgDurationMs
	if stats.Ticks > 0 || sum > 0 {
		avg = (stats.AvgDurationMs*float64(stats.Ticks) + sum) / float64(stats.Ticks+1)
	}
	return model.SamplerStats{
		Ticks:          stats.Ticks + 1,
		TickErrors:     stats.TickErrors,
		ReadErrors:     stats.ReadErrors,
		LastDurationMs: elapsedMs,
		AvgDurationMs:  avg,
	}
}

// withTickError records a failed tick while leaving the timing history intact.
func withTickError(stats model.SamplerStats) model.SamplerStats {
	stats.TickErrors++
	return stats
}

// withReadError records a failed per-device or per-subsystem read.
func withReadError(stats model.SamplerStats) model.SamplerStats {
	stats.ReadErrors++
	return stats
}

// recordWarning records a warning once, so a permanent fault does not flood
// the snapshot, and is the only producer of SamplerStats.ReadErrors: the count
// a user looks at to decide whether a metric that says N/A means "absent" or
// "unreadable". Leaving it at zero would make the read path look like it had
// never failed.
func (s *Sampler) recordWarning(message string, current *[]string) {
	if _, seen := s.seenWarnings[message]; seen {
		return
	}
	s.seenWarnings[message] = struct{}{}
	if len(s.seenWarnings) > MaxSeenWarnings {
		// Bounded rather than capped per snapshot: a message that embeds a
		// changing value (a PID, a path) would otherwise add a fresh entry
		// every few seconds for as long as the monitor ran.
		s.seenWarnings = map[string]struct{}{}
	}
	s.stats = withReadError(s.stats)
	if len(*current) < MaxWarnings {
		*current = append(*current, message)
	}
}

// dominantABI returns the ABI used by the first device that has one, for the
// footer.
func dominantABI(samples []model.AmdgpuMetrics) string {
	for i := range samples {
		if samples[i].MetricsABI != nil {
			return *samples[i].MetricsABI
		}
	}
	return ""
}

// blocksHint returns the explanation shown where the blocks panel would be.
// It is attached to every snapshot rather than raised as a warning, because a
// disabled panel is a permanent condition rather than a fault: it would hold
// one of the warning slots for the whole session and push out a real error.
func (s *Sampler) blocksHint(status model.BlocksStatus) string {
	if status.OK() {
		return ""
	}
	if d := s.blocks.Diagnostic(); d != "" {
		return d
	}
	return status.Hint()
}
