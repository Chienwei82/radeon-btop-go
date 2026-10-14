// Package sampler is the sampling engine.
//
// One tick goroutine owns every piece of mutable sampling state -- the
// sequence counter, the history ring, the rolling statistics and the
// per-client delta baselines inside the process collector -- and publishes
// immutable model.GpuSnapshot values through a bounded channel. The consumer
// (the TUI, or the --dump CLI path) drains the newest snapshot and never does
// I/O of its own.
//
// The design mirrors the Python reference sampler (gputop/sampler/engine.py):
//   - a fixed-rate deadline grid, so a slow tick drops its missed slots
//     rather than replaying them and thrashing the disk, while keeping phase;
//   - an interruptible sleep, so set-interval and stop take effect at once;
//   - a tick that never dies: a bad read degrades to a bounded warning.
//
// The optional radeontop "blocks" panel is a supervised child process and
// belongs to a later phase; the sampler depends on the BlocksSource seam, not
// on that child, so it ships here with the disabled implementation.
package sampler
