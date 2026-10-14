// Package config loads and validates gputop's YAML configuration.
//
// Precedence, lowest to highest: built-in defaults -> config file ->
// $GPUTOP_CONFIG -> ~/.config/gputop/gputop.yaml -> the state: block gputop
// saved for itself -> CLI flags.
//
// The module's promise is that loading never refuses to start: an unknown
// key, a wrong type, a malformed file or an out-of-range value is reported as
// a warning and replaced with a working default. A monitor that crashes on a
// typo in its config file is worse than one that runs with a default.
//
// The format is YAML (goccy/go-yaml), not the Python build's TOML: this port
// owes functional parity, not file compatibility. The key set matches the
// reference; the layout differs in one deliberate way -- the [general] table
// is flattened, so interval_ms and history_points sit at the top level beside
// the sections, because a two-key section is noise in YAML.
package config

import (
	"path/filepath"

	"github.com/gputop/gputop-go/internal/model"
)

// Interval floors and ceilings in milliseconds. The floor exists because a
// faster tick spends more time in procfs than in the data it collects; the
// ceiling bounds a key press from parking the loop for hours.
const (
	MinIntervalMs = 100
	MaxIntervalMs = 10_000
)

// MaxTempC is the ceiling for an alert temperature threshold: past this point
// every AMD part is throttling hard enough that the number stops being
// informative. It matches the scale the sensor panel draws against.
const MaxTempC = 125.0

// DefaultHistoryPoints is how many samples the history ring retains.
const DefaultHistoryPoints = model.DefaultHistoryLength

// Environment variables that redirect where configuration and session state
// are read from. They exist so a test, a container or a second profile can
// keep its state separate instead of inheriting the real one.
const (
	ConfigEnv = "GPUTOP_CONFIG"
	StateEnv  = "GPUTOP_STATE"
)

// DefaultStatePath is where the session state lives by default. It is the
// same file as the config: gputop writes its state: block into the user's
// config and leaves every other section alone.
var DefaultStatePath = filepath.Join("~", ".config", "gputop", "config.yaml")

// GpuConfig is device selection and classification.
type GpuConfig struct {
	// Devices restricts monitoring to these cards, named by PCI address or
	// by cardN. Empty means every amdgpu device found.
	Devices []string
	// Kind is "auto", "igpu" or "dgpu".
	Kind string
	// Names maps a PCI address to a display name.
	Names map[string]string
}

// ProcessConfig is per-process table behaviour.
type ProcessConfig struct {
	Show              bool
	MaxRows           int
	Sort              string
	MinUsagePercent   float64
	HideKernelThreads bool
	ProcessTree       bool
	FullCommand       bool
	ShowIdle          bool
	AllowKill         bool
}

// StateConfig is what the interface looked like last time, written by gputop
// itself. Every field is a runtime choice; nil (a pointer) means "not
// recorded", which is distinct from a recorded default: a field that was
// never written must not silently override a hand-written value.
type StateConfig struct {
	Theme       *string
	IntervalMs  *int
	Sort        *string
	SortReverse *bool
	Filter      string
	ShowIdle    bool
	ProcessTree bool
	FullCommand bool
	Overview    bool
	GpuIndex    int
}

// BlocksConfig is the optional radeontop-backed per-block panel. Off by
// default: enabling it spawns a child that, unprivileged, answers "are you
// root?", so a default-on panel would put a permanent complaint in every
// status line.
type BlocksConfig struct {
	Enabled   bool
	Binary    string
	Ticks     int
	IntervalS int
	DrmRoot   string
	Restart   bool
}

// AlertsConfig is threshold alerting. Thresholds are absolute because there
// is no honest way to derive "too hot" from what the card reports. A zero
// disables an individual threshold.
type AlertsConfig struct {
	Enabled      bool
	TempC        float64
	PowerPercent float64
	VramPercent  float64
	FlashHz      float64
}

// LogConfig is session recording. Only --log turns recording on; this
// configures how.
type LogConfig struct {
	ZstdLevel int
	IntervalS float64
}

// UiConfig is appearance and layout.
type UiConfig struct {
	Theme       string
	NoColor     bool
	GraphHeight int
}

// Config is the complete, validated configuration.
type Config struct {
	// IntervalMs and HistoryPoints are the flattened [general] table.
	IntervalMs    int
	HistoryPoints int
	Gpu           GpuConfig
	Process       ProcessConfig
	UI            UiConfig
	Blocks        BlocksConfig
	Alerts        AlertsConfig
	Log           LogConfig
	State         StateConfig
	// Warnings collects every unknown key, wrong type and clamped value seen
	// while loading. It is never an error: a malformed file yields defaults
	// plus warnings.
	Warnings []string
}

// IntervalS is the sampling interval in seconds.
func (c Config) IntervalS() float64 { return float64(c.IntervalMs) / 1000.0 }

// HistoryLength is the number of samples retained, floored at one.
func (c Config) HistoryLength() int {
	if c.HistoryPoints < 1 {
		return 1
	}
	return c.HistoryPoints
}

// Thresholds is the model threshold record this config describes.
func (a AlertsConfig) Thresholds() model.Thresholds {
	return model.Thresholds{
		TempC:        a.TempC,
		PowerPercent: a.PowerPercent,
		VramPercent:  a.VramPercent,
	}
}
