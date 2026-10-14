package sampler

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/readers"
)

// errSampleOnceRunning guards the single-owner invariant of the tick body.
var errSampleOnceRunning = errors.New(
	"sample_once() runs a whole tick and must not race the sampling thread; " +
		"use RequestTick() to ask a running sampler for a sample",
)

// Sampler is the background sampling engine. It owns all mutable sampling
// state on its tick goroutine and publishes immutable model.GpuSnapshot
// values through a bounded channel.
//
// A single Sampler may be shared between goroutines provided only one calls
// Start/Stop, which is how both the CLI and (later) the TUI use it.
type Sampler struct {
	opts   atomic.Pointer[SamplerOptions]
	blocks BlocksSource

	// out is the bounded snapshot channel (capacity snapshotQueueSize). The
	// tick loop never blocks on it: a full channel means a slow consumer, and
	// the newest snapshot is what matters, so a full send drops the oldest.
	out chan model.GpuSnapshot

	// history ring, owned by the tick goroutine; historyView is the immutable
	// slice rebound after every tick for cross-goroutine readers.
	history     *model.RingBuffer[model.GpuSnapshot]
	historyView atomic.Pointer[[]model.GpuSnapshot]

	// collector owns the per-client delta baselines; only the tick touches it.
	collector *readers.ProcessCollector

	// interval is read by the tick loop every iteration and written by
	// SetInterval, so it is atomic.
	interval atomic.Value // float64

	// sequence and stats are mutated only by the tick; stats is published for
	// cross-goroutine reads via statsView.
	sequence  int
	stats     model.SamplerStats
	statsView atomic.Pointer[model.SamplerStats]

	// seenWarnings de-duplicates warnings so a permanent fault floods neither
	// the snapshot nor the read-error counter. Tick-owned.
	seenWarnings map[string]struct{}

	// devices and filterWarning are published for readers after discovery.
	devices       atomic.Pointer[[]model.AmdgpuDevice]
	filterWarning atomic.Pointer[string]

	// Lifecycle. runDone is closed when the tick goroutine exits; wake pokes
	// the loop's interruptible sleep (set-interval / request-tick / stop).
	cancel  context.CancelFunc
	runDone chan struct{}
	wake    chan struct{}
	mu      sync.Mutex
	running bool
}

// New returns a sampler with validated options and a disabled blocks source.
func New(opts SamplerOptions) *Sampler {
	// Apply the dataclass defaults the reference carries on SamplerOptions: an
	// unset (zero) field means "not specified" and takes the default, which
	// validated() then floors. This is what distinguishes a default 300-sample
	// history and a 1 s interval from an explicit request for the minimum.
	if opts.HistoryLength == 0 {
		opts.HistoryLength = model.DefaultHistoryLength
	}
	if opts.IntervalS == 0 {
		opts.IntervalS = DefaultIntervalS
	}
	v := opts.validated()
	s := &Sampler{
		blocks:       disabledBlocks{},
		out:          make(chan model.GpuSnapshot, snapshotQueueSize),
		history:      model.NewRingBuffer[model.GpuSnapshot](v.HistoryLength),
		collector:    readers.NewProcessCollector(v.ProcRoot),
		wake:         make(chan struct{}, 1),
		seenWarnings: map[string]struct{}{},
	}
	s.opts.Store(&v)
	s.interval.Store(v.IntervalS)
	empty := []model.GpuSnapshot{}
	s.historyView.Store(&empty)
	stats := model.SamplerStats{}
	s.statsView.Store(&stats)
	noDevices := []model.AmdgpuDevice{}
	s.devices.Store(&noDevices)
	noWarning := ""
	s.filterWarning.Store(&noWarning)
	return s
}

// WithBlocks replaces the blocks source. It must be called before Start; the
// Phase 3 radeontop pool is installed here.
func (s *Sampler) WithBlocks(source BlocksSource) *Sampler {
	if source != nil {
		s.blocks = source
	}
	return s
}

// Options returns the validated configuration in effect.
func (s *Sampler) Options() SamplerOptions { return *s.opts.Load() }

// Stats returns the rolling health counters. The stored value is a single
// struct, so a cross-goroutine read never observes a half-updated state.
func (s *Sampler) Stats() model.SamplerStats { return *s.statsView.Load() }

// Devices returns the devices discovered by Discover or Start.
func (s *Sampler) Devices() []model.AmdgpuDevice { return *s.devices.Load() }

// FilterWarning returns why a gpu.devices entry matched no card, or "" when
// all of them did. It is meaningful only once Discover or Start has run.
func (s *Sampler) FilterWarning() string { return *s.filterWarning.Load() }

// History returns the retained history, oldest first. Safe from any goroutine:
// the tick rebinds historyView to a fresh slice after every tick, and readers
// only ever touch that slice.
func (s *Sampler) History() []model.GpuSnapshot { return *s.historyView.Load() }

// -- lifecycle ----------------------------------------------------------

// Start discovers devices and starts the sampling goroutine. Discovery runs on
// the caller's goroutine so a machine with no AMD GPU reports that immediately
// rather than after the first tick.
func (s *Sampler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	devices := s.discover()
	s.blocks.Configure(devices)
	s.blocks.Start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel = cancel
	s.runDone = done
	s.running = true
	s.mu.Unlock()

	go s.run(ctx, done)
}

// Stop signals the tick goroutine to finish and waits for it. Teardown is
// bounded: a wedged tick is no reason to hang shutdown, and the blocks source
// is torn down here too, for callers that never start the goroutine.
func (s *Sampler) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	done := s.runDone
	s.running = false
	s.mu.Unlock()

	s.poke()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	s.blocks.Stop()
}

// SetInterval changes the sampling period of an already running sampler and
// returns the value in effect. A negative or absurd value is clamped rather
// than rejected, because the only caller is a key press and the clamp is
// exactly the behaviour the key advertises.
func (s *Sampler) SetInterval(intervalS float64) float64 {
	v := clampInterval(intervalS)
	s.interval.Store(v)
	// Reflect it in the published options so a reader sees the same value the
	// loop will use next.
	opts := *s.opts.Load()
	opts.IntervalS = v
	s.opts.Store(&opts)
	// Cut the loop's current sleep short: re-reading the period only helps
	// once the loop is awake.
	s.poke()
	return v
}

// RequestTick asks a running sampler for a sample now, without waiting for the
// period. It pokes the loop's wake-up rather than running a tick on the
// caller's goroutine, so the sampler thread stays the single owner.
func (s *Sampler) RequestTick() { s.poke() }

// poke nudges the interruptible sleep without blocking.
func (s *Sampler) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// -- consuming ----------------------------------------------------------

// Latest returns the newest published snapshot without blocking, draining any
// that queued behind it so a slow consumer is never handed a stale sample.
func (s *Sampler) Latest() *model.GpuSnapshot {
	var newest *model.GpuSnapshot
	for {
		select {
		case snap := <-s.out:
			n := snap
			newest = &n
		default:
			return newest
		}
	}
}

// Wait blocks until a snapshot arrives or the timeout elapses, returning nil
// on timeout.
func (s *Sampler) Wait(timeout time.Duration) *model.GpuSnapshot {
	select {
	case snap := <-s.out:
		n := snap
		return &n
	case <-time.After(timeout):
		return nil
	}
}

// -- one-shot sampling --------------------------------------------------

// SampleOnce runs exactly one tick synchronously and publishes the result.
//
// It must not be called while the sampling goroutine is running: a tick
// mutates the sequence counter, the history ring, the statistics and the
// collector's delta baseline, all of which the design gives to a single owner.
// The guard is enforced, not merely documented.
func (s *Sampler) SampleOnce() (model.GpuSnapshot, error) {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		return model.GpuSnapshot{}, errSampleOnceRunning
	}
	snap := s.tick()
	s.publish(snap)
	return snap, nil
}

// Discover enumerates devices without starting the goroutine, for one-shot
// use, and points the blocks source at them (configured, not started).
func (s *Sampler) Discover() []model.AmdgpuDevice {
	devices := s.discover()
	s.blocks.Configure(devices)
	return devices
}

// discover applies the gpu.devices filter and records why an entry missed.
func (s *Sampler) discover() []model.AmdgpuDevice {
	opts := *s.opts.Load()
	found := readers.DiscoverDevices(opts.DrmRoot, opts.KindOverride, opts.NameOverrides, opts.PciRoot)
	selected, unmatched := readers.SelectDevices(found, opts.OnlyDevices)
	if len(unmatched) > 0 {
		available := make([]string, 0, len(found))
		for _, d := range found {
			name := d.BDF
			if name == "" {
				name = d.Card
			}
			available = append(available, name)
		}
		sort.Strings(available)
		suffix := " (no devices found)"
		if len(available) > 0 {
			suffix = " (found: " + strings.Join(available, ", ") + ")"
		}
		warning := "gpu.devices: no such device: " + strings.Join(unmatched, ", ") + suffix
		s.filterWarning.Store(&warning)
	} else {
		empty := ""
		s.filterWarning.Store(&empty)
	}
	selectedCopy := append([]model.AmdgpuDevice(nil), selected...)
	s.devices.Store(&selectedCopy)
	return selectedCopy
}
