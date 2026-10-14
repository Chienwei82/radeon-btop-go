package sampler

import "github.com/gputop/gputop-go/internal/model"

// BlocksSource is the seam for the optional radeontop "blocks" panel.
//
// The sampler polls it once per tick and asks it for a per-device sample and
// an overall status, without knowing that a supervised child process stands
// behind it. Phase 1 ships the disabled implementation; Phase 3 replaces it
// with the radeontop pool behind this same interface, leaving the tick loop
// untouched.
type BlocksSource interface {
	// Poll refreshes every source and returns the overall status. It is
	// called once per tick, before the devices are read, so a block sample
	// already waiting is attached to the tick that will display it.
	Poll() model.BlocksStatus
	// Status returns the current overall status without polling.
	Status() model.BlocksStatus
	// Diagnostic returns a one-line explanation of a non-OK status, or ""
	// when healthy.
	Diagnostic() string
	// BlocksFor returns the latest block sample for a device index, or nil
	// when there is none.
	BlocksFor(index int) *model.GpuBlocks
	// Configure points the source at the currently discovered devices. It is
	// called after discovery, before Start.
	Configure(devices []model.AmdgpuDevice)
	// Start begins sampling (no-op when disabled).
	Start()
	// Stop tears the source down. It is idempotent.
	Stop()
}

// disabledBlocks is the Phase 1 BlocksSource: the panel is off, so every
// query reports BlocksDisabled and no device ever carries block data. This is
// the common, non-error case the UI explains with a hint rather than a
// warning.
type disabledBlocks struct{}

func (disabledBlocks) Poll() model.BlocksStatus       { return model.BlocksDisabled }
func (disabledBlocks) Status() model.BlocksStatus     { return model.BlocksDisabled }
func (disabledBlocks) Diagnostic() string             { return "" }
func (disabledBlocks) BlocksFor(int) *model.GpuBlocks { return nil }
func (disabledBlocks) Configure([]model.AmdgpuDevice) {}
func (disabledBlocks) Start()                         {}
func (disabledBlocks) Stop()                          {}
