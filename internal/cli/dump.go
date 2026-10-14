// Package cli implements gputop's headless command line modes.
//
// The one-shot commands (--dump, --devices) exist so the data layer can be
// inspected, diffed and scripted without a terminal: --dump is the machine
// readable form of everything the interface draws, and it is what the port's
// parity gate compares field-for-field against the Python reference.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/model"
)

// This file serialises a snapshot to the same JSON shape the Python reference
// --dump emits. Optional readings are emitted as null rather than omitted, so
// a consumer can tell "not available on this hardware" apart from "not
// collected". Field order matters: Go's encoding/json writes struct fields in
// declaration order and the reference writes keys in dict insertion order
// (sort_keys=False), so the two are kept aligned deliberately -- that is what
// lets a plain textual diff of --dump mean something. Float formatting still
// differs between the two (Go writes 25, Python 25.0), so the parity gate
// compares parsed structures field-for-field rather than raw bytes. The few
// fields the reference builds from a dict (voltages_mv, blocks.percent,
// blocks.clocks) go through orderedJSON, because encoding/json sorts map keys
// and would silently reorder them.

// orderedJSON is a JSON object that keeps its keys in insertion order. It
// exists only for the handful of dict-valued fields above; every fixed-shape
// object is a struct so its field order is visible at the declaration.
type orderedJSON struct {
	keys   []string
	values []any
}

// newOrdered returns an empty ordered object.
func newOrdered() *orderedJSON { return &orderedJSON{} }

// set appends a key/value pair, preserving order.
func (o *orderedJSON) set(key string, value any) *orderedJSON {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
	return o
}

// MarshalJSON writes the pairs in the order they were set.
func (o *orderedJSON) MarshalJSON() ([]byte, error) {
	buf := []byte{'{'}
	for i, k := range o.keys {
		if i > 0 {
			buf = append(buf, ',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(o.values[i])
		if err != nil {
			return nil, err
		}
		buf = append(buf, key...)
		buf = append(buf, ':')
		buf = append(buf, value...)
	}
	return append(buf, '}'), nil
}

// memoryJSON is one memory pool: used/total in bytes plus the derived percent.
type memoryJSON struct {
	Used    *int64   `json:"used"`
	Total   *int64   `json:"total"`
	Percent *float64 `json:"percent"`
}

func toMemory(m model.MemoryPool) memoryJSON {
	return memoryJSON{Used: m.Used, Total: m.Total, Percent: m.Percent()}
}

// clockJSON is one clock with its current value, ceiling and source.
type clockJSON struct {
	Current *int   `json:"current"`
	Maximum *int   `json:"maximum"`
	Source  string `json:"source"`
}

func toClock(c model.Clock) clockJSON {
	return clockJSON{Current: c.Current, Maximum: c.Maximum, Source: c.Source}
}

// temperatureJSON is one temperature reading.
type temperatureJSON struct {
	Label   string   `json:"label"`
	Celsius *float64 `json:"celsius"`
	Source  string   `json:"source"`
}

func toTemperatures(ts []model.Temperature) []temperatureJSON {
	out := make([]temperatureJSON, 0, len(ts))
	for _, t := range ts {
		out = append(out, temperatureJSON{Label: t.Label, Celsius: t.Celsius, Source: t.Source})
	}
	return out
}

// toVoltages renders the named voltage rails as a JSON object, keyed by name,
// exactly as the reference's dict(voltages_mv) does.
func toVoltages(vs []model.VoltageReading) *orderedJSON {
	out := newOrdered()
	for _, v := range vs {
		out.set(v.Name, v.MV)
	}
	return out
}

// powerJSON is board power draw against the enforced cap.
type powerJSON struct {
	DrawW   *float64 `json:"draw_w"`
	CapW    *float64 `json:"cap_w"`
	EnergyM *int64   `json:"energy_mj"`
	Source  string   `json:"source"`
}

func toPower(p model.Power) powerJSON {
	return powerJSON{DrawW: p.DrawW, CapW: p.CapW, EnergyM: p.EnergyM, Source: p.Source}
}

// fanJSON is fan state; APUs and passively cooled cards report null throughout.
type fanJSON struct {
	RPM     *int     `json:"rpm"`
	MaxRPM  *int     `json:"max_rpm"`
	PWM     *float64 `json:"pwm_percent"`
	Stopped bool     `json:"stopped"`
}

func toFan(f model.Fan) fanJSON {
	return fanJSON{RPM: f.RPM, MaxRPM: f.MaxRPM, PWM: f.Percent(), Stopped: f.Stopped()}
}

// throttleJSON is decoded throttle status bits.
type throttleJSON struct {
	Raw          int      `json:"raw"`
	Active       []string `json:"active"`
	IsThrottling bool     `json:"is_throttling"`
}

func toThrottle(t *model.ThrottleInfo) *throttleJSON {
	if t == nil {
		return nil
	}
	// The reference stores reasons in a frozenset and sorts before emitting,
	// because a set has no order of its own; match it so the dump is stable.
	active := make([]string, 0, len(t.Active))
	active = append(active, t.Active...)
	sort.Strings(active)
	return &throttleJSON{Raw: t.Raw, Active: active, IsThrottling: t.IsThrottling()}
}

// pcieJSON is a negotiated PCIe link. Speed is the kernel's own unit, tenths
// of a GT/s, so the decoded rate is carried alongside it.
type pcieJSON struct {
	Width       int     `json:"width"`
	Speed       int     `json:"speed"`
	GtPerSecond float64 `json:"gt_per_second"`
	Generation  int     `json:"generation"`
	Describe    string  `json:"describe"`
}

func toPcie(p *model.PcieLink) *pcieJSON {
	if p == nil {
		return nil
	}
	return &pcieJSON{
		Width:       p.Width,
		Speed:       p.Speed,
		GtPerSecond: p.GtPerSecond(),
		Generation:  p.Generation(),
		Describe:    p.Describe(),
	}
}

// odvDomainJSON is one clock domain's overdrive table. Each entry is a
// three-element array [state, value, unit], matching the reference's nested
// list rather than an object.
type odvDomainJSON struct {
	Name    string   `json:"name"`
	Minimum *int     `json:"minimum"`
	Maximum *int     `json:"maximum"`
	Ceiling *int     `json:"ceiling"`
	Unit    string   `json:"unit"`
	Percent *float64 `json:"percent"`
	Entries [][3]any `json:"entries"`
}

// odcJSON is a parsed pp_od_clk_voltage.
type odcJSON struct {
	Present        bool            `json:"present"`
	VddgfxOffsetMv *int            `json:"vddgfx_offset_mv"`
	Domains        []odvDomainJSON `json:"domains"`
}

func toODC(o model.OdcVoltage) odcJSON {
	domains := make([]odvDomainJSON, 0, len(o.Domains))
	for _, d := range o.Domains {
		entries := make([][3]any, 0, len(d.Entries))
		for _, e := range d.Entries {
			entries = append(entries, [3]any{e.State, e.Value, string(e.Unit)})
		}
		domains = append(domains, odvDomainJSON{
			Name:    d.Name,
			Minimum: d.Minimum,
			Maximum: d.Maximum,
			Ceiling: d.Ceiling(),
			Unit:    string(d.Unit),
			Percent: d.Percent(),
			Entries: entries,
		})
	}
	return odcJSON{Present: o.Present, VddgfxOffsetMv: o.VddgfxOffsetMv, Domains: domains}
}

// powerProfilesJSON is the SCPP profile table.
type powerProfilesJSON struct {
	Present     bool     `json:"present"`
	Active      string   `json:"active"`
	ActiveIndex *int     `json:"active_index"`
	Available   []string `json:"available"`
}

func toProfiles(p model.PowerProfileTable) powerProfilesJSON {
	return powerProfilesJSON{
		Present:     p.Present,
		Active:      p.ActiveName(),
		ActiveIndex: p.ActiveIndex(),
		Available:   p.Names(),
	}
}

// clockMhzJSON is one clock's utilisation and its absolute frequency.
type clockMhzJSON struct {
	Percent *float64 `json:"percent"`
	Mhz     *int     `json:"mhz"`
}

// blocksJSON is a complete per-block sample, or null when there is none. The
// percent and clocks maps keep radeontop's own ordering (pipeline, then video).
type blocksJSON struct {
	Percent     *orderedJSON `json:"percent"`
	Busiest     *string      `json:"busiest"`
	Clocks      *orderedJSON `json:"clocks"`
	TimestampNs int64        `json:"timestamp_ns"`
}

func toBlocks(b *model.GpuBlocks) *blocksJSON {
	if b == nil {
		return nil
	}
	percent := newOrdered()
	index := make(map[string]int, len(b.Blocks)+len(b.AuxBlocks))
	// by_key() is a dict comprehension over the pipeline blocks then update()d
	// with the aux blocks: a repeated key keeps its first position and takes the
	// later value. Mirror that so the two orders cannot drift.
	put := func(key string, value float64) {
		if i, ok := index[key]; ok {
			percent.values[i] = value
			return
		}
		index[key] = len(percent.keys)
		percent.set(key, value)
	}
	for _, r := range b.Blocks {
		put(r.Key, float64(r.Percent))
	}
	for _, r := range b.AuxBlocks {
		put(r.Key, float64(r.Percent))
	}
	clocks := newOrdered()
	for _, c := range b.Clocks {
		clocks.set(c.Key, clockMhzJSON{Percent: c.Percent, Mhz: c.Mhz})
	}
	var busiest *string
	if key := b.BusiestKey(); key != "" {
		busiest = &key
	}
	return &blocksJSON{
		Percent:     percent,
		Busiest:     busiest,
		Clocks:      clocks,
		TimestampNs: b.TimestampNs,
	}
}

// deviceJSON serialises one device's metrics. Field order matches the
// reference _device_to_dict.
type deviceJSON struct {
	Index            int               `json:"index"`
	Card             string            `json:"card"`
	Render           *string           `json:"render"`
	BDF              string            `json:"bdf"`
	Name             string            `json:"name"`
	Kind             string            `json:"kind"`
	Integrated       bool              `json:"integrated"`
	DeviceID         string            `json:"device_id"`
	BootVGA          bool              `json:"boot_vga"`
	MetricsABI       *string           `json:"metrics_abi"`
	GpuBusyPercent   *float64          `json:"gpu_busy_percent"`
	MemBusyPercent   *float64          `json:"mem_busy_percent"`
	VRAM             memoryJSON        `json:"vram"`
	GTT              memoryJSON        `json:"gtt"`
	SCLK             clockJSON         `json:"sclk"`
	MCLK             clockJSON         `json:"mclk"`
	SocClk           clockJSON         `json:"socclk"`
	Temperatures     []temperatureJSON `json:"temperatures"`
	VoltagesMv       *orderedJSON      `json:"voltages_mv"`
	Power            powerJSON         `json:"power"`
	Fan              fanJSON           `json:"fan"`
	PerformanceLevel *string           `json:"performance_level"`
	PowerProfile     *string           `json:"power_profile"`
	Throttle         *throttleJSON     `json:"throttle"`
	PCIE             *pcieJSON         `json:"pcie"`
	Blocks           *blocksJSON       `json:"blocks"`
	ODC              odcJSON           `json:"odc"`
	PowerProfiles    powerProfilesJSON `json:"power_profiles"`
}

func toDevice(m model.AmdgpuMetrics) deviceJSON {
	return deviceJSON{
		Index:            m.Device.Index,
		Card:             m.Device.Card,
		Render:           m.Device.Render,
		BDF:              m.Device.BDF,
		Name:             m.Device.Name,
		Kind:             string(m.Device.Kind),
		Integrated:       m.Device.IsIntegrated(),
		DeviceID:         hexDeviceID(m.Device.DeviceID),
		BootVGA:          m.Device.BootVGA,
		MetricsABI:       m.MetricsABI,
		GpuBusyPercent:   m.GpuBusyPercent,
		MemBusyPercent:   m.MemBusyPercent,
		VRAM:             toMemory(m.VRAM),
		GTT:              toMemory(m.GTT),
		SCLK:             toClock(m.SCLK),
		MCLK:             toClock(m.MCLK),
		SocClk:           toClock(m.SocClk),
		Temperatures:     toTemperatures(m.Temperatures),
		VoltagesMv:       toVoltages(m.VoltagesMv),
		Power:            toPower(m.Power),
		Fan:              toFan(m.Fan),
		PerformanceLevel: m.PerformanceLevel,
		PowerProfile:     m.PowerProfile,
		Throttle:         toThrottle(m.Throttle),
		PCIE:             toPcie(m.PCIE),
		Blocks:           toBlocks(m.Blocks),
		ODC:              toODC(m.ODC),
		PowerProfiles:    toProfiles(m.Profiles),
	}
}

// hexDeviceID renders a PCI device id the way the reference does: 0x plus four
// lowercase hex digits.
func hexDeviceID(deviceID int) string { return fmt.Sprintf("0x%04x", deviceID) }

// engineJSON is one engine's usage over the last window.
type engineJSON struct {
	Engine   string   `json:"engine"`
	TotalNs  int64    `json:"total_ns"`
	DeltaNs  int64    `json:"delta_ns"`
	WindowNs int64    `json:"window_ns"`
	Percent  *float64 `json:"percent"`
}

// processJSON serialises one DRM client. Field order matches the reference.
type processJSON struct {
	PID           int          `json:"pid"`
	Name          string       `json:"name"`
	User          string       `json:"user"`
	BDF           string       `json:"bdf"`
	ClientID      int          `json:"client_id"`
	EnginePercent float64      `json:"engine_percent"`
	Engines       []engineJSON `json:"engines"`
	VramUsed      *int64       `json:"vram_used"`
	GttUsed       *int64       `json:"gtt_used"`
	CpuUsed       *int64       `json:"cpu_used"`
	MemoryUsed    int64        `json:"memory_used"`
}

func toProcess(p model.GpuProcess) processJSON {
	engines := make([]engineJSON, 0, len(p.Engines))
	for _, u := range p.Engines {
		// model.Nanoseconds is an alias for int64, so these need no conversion.
		engines = append(engines, engineJSON{
			Engine:   u.Engine,
			TotalNs:  u.TotalNs,
			DeltaNs:  u.DeltaNs,
			WindowNs: u.WindowNs,
			Percent:  u.Percent(),
		})
	}
	return processJSON{
		PID:           p.PID,
		Name:          p.Name,
		User:          p.User,
		BDF:           p.BDF,
		ClientID:      p.ClientID,
		EnginePercent: p.EnginePercent(),
		Engines:       engines,
		VramUsed:      p.VramUsed,
		GttUsed:       p.GttUsed,
		CpuUsed:       p.CpuUsed,
		MemoryUsed:    p.MemoryUsed(),
	}
}

// processVisibilityJSON says how much of /proc the user could see.
type processVisibilityJSON struct {
	Visible int  `json:"visible"`
	Total   int  `json:"total"`
	Partial bool `json:"partial"`
}

// statsJSON is the sampler's rolling health. The reference does not emit
// read_errors here even though the model carries it.
type statsJSON struct {
	Ticks          int     `json:"ticks"`
	TickErrors     int     `json:"tick_errors"`
	LastDurationMs float64 `json:"last_duration_ms"`
	AvgDurationMs  float64 `json:"avg_duration_ms"`
}

// blocksStatusJSON says whether the radeontop panel is producing data.
type blocksStatusJSON struct {
	Status string `json:"status"`
	Hint   string `json:"hint"`
}

// breachJSON is one metric that crossed its threshold.
type breachJSON struct {
	Metric  string `json:"metric"`
	Reading string `json:"reading"`
	Limit   string `json:"limit"`
}

// thresholdsJSON are the configured alert limits, echoed so a script can see
// what the breaches were measured against.
type thresholdsJSON struct {
	TempC        float64 `json:"temp_c"`
	PowerPercent float64 `json:"power_percent"`
	VramPercent  float64 `json:"vram_percent"`
}

// alertsJSON is the threshold alert state. Thresholds are present only when
// alerting is enabled, matching the reference.
type alertsJSON struct {
	Enabled    bool            `json:"enabled"`
	Level      string          `json:"level"`
	Breaches   []breachJSON    `json:"breaches"`
	Thresholds *thresholdsJSON `json:"thresholds,omitempty"`
}

// snapshotJSON serialises a whole snapshot. Field order matches the reference
// snapshot_to_dict.
type snapshotJSON struct {
	Sequence           int                   `json:"sequence"`
	TimestampNs        int64                 `json:"timestamp_ns"`
	MetricsABI         *string               `json:"metrics_abi"`
	UsingBinaryMetrics bool                  `json:"using_binary_metrics"`
	Devices            []deviceJSON          `json:"devices"`
	Processes          []processJSON         `json:"processes"`
	ProcessVisibility  processVisibilityJSON `json:"process_visibility"`
	Stats              statsJSON             `json:"stats"`
	Warnings           []string              `json:"warnings"`
	Blocks             blocksStatusJSON      `json:"blocks"`
	Alerts             *alertsJSON           `json:"alerts"`
}

// Snapshot serialises a snapshot. A nil config drops the alert section rather
// than inventing thresholds, so a caller without a config gets no alert claim
// rather than a wrong one.
func Snapshot(snap model.GpuSnapshot, cfg *config.Config) *snapshotJSON {
	devices := make([]deviceJSON, 0, len(snap.Devices))
	for _, m := range snap.Devices {
		devices = append(devices, toDevice(m))
	}
	processes := make([]processJSON, 0, len(snap.Processes))
	for _, p := range snap.Processes {
		processes = append(processes, toProcess(p))
	}
	warnings := snap.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	var abi *string
	if snap.Source.MetricsABI != "" {
		v := snap.Source.MetricsABI
		abi = &v
	}
	return &snapshotJSON{
		Sequence:           snap.Sequence,
		TimestampNs:        snap.TimestampNs,
		MetricsABI:         abi,
		UsingBinaryMetrics: snap.Source.UsingBinaryMetrics(),
		Devices:            devices,
		Processes:          processes,
		ProcessVisibility: processVisibilityJSON{
			Visible: snap.VisibleProcessCount,
			Total:   snap.TotalProcessCount,
			Partial: snap.IsPartialProcessView(),
		},
		Stats: statsJSON{
			Ticks:          snap.Stats.Ticks,
			TickErrors:     snap.Stats.TickErrors,
			LastDurationMs: snap.Stats.LastDurationMs,
			AvgDurationMs:  snap.Stats.AvgDurationMs,
		},
		Warnings: warnings,
		Blocks:   blocksStatusJSON{Status: string(snap.BlocksStatus), Hint: snap.BlocksHint},
		Alerts:   alertsFor(snap, cfg),
	}
}

// alertsFor evaluates every device against the configured thresholds. It is the
// same evaluate/worst the interface uses, so --dump and the panel can never
// disagree about whether something is over its limit.
func alertsFor(snap model.GpuSnapshot, cfg *config.Config) *alertsJSON {
	if cfg == nil {
		return nil
	}
	if !cfg.Alerts.Enabled {
		return &alertsJSON{Enabled: false, Level: model.AlertOK.Label(), Breaches: []breachJSON{}}
	}
	thresholds := cfg.Alerts.Thresholds()
	states := make([]model.AlertState, 0, len(snap.Devices))
	for _, m := range snap.Devices {
		states = append(states, model.Evaluate(m, thresholds))
	}
	state := model.WorstAlert(states)
	breaches := make([]breachJSON, 0, len(state.Breaches))
	for _, b := range state.Breaches {
		breaches = append(breaches, breachJSON{Metric: b.Metric, Reading: b.Reading, Limit: b.Limit})
	}
	return &alertsJSON{
		Enabled:  true,
		Level:    state.Level.Label(),
		Breaches: breaches,
		Thresholds: &thresholdsJSON{
			TempC:        thresholds.TempC,
			PowerPercent: thresholds.PowerPercent,
			VramPercent:  thresholds.VramPercent,
		},
	}
}

// deviceSummaryJSON is one row of the --devices listing.
type deviceSummaryJSON struct {
	Index    int     `json:"index"`
	Card     string  `json:"card"`
	Render   *string `json:"render"`
	BDF      string  `json:"bdf"`
	Name     string  `json:"name"`
	Kind     string  `json:"kind"`
	DeviceID string  `json:"device_id"`
	BootVGA  bool    `json:"boot_vga"`
}

func toDeviceSummary(d model.AmdgpuDevice) deviceSummaryJSON {
	return deviceSummaryJSON{
		Index:    d.Index,
		Card:     d.Card,
		Render:   d.Render,
		BDF:      d.BDF,
		Name:     d.Name,
		Kind:     string(d.Kind),
		DeviceID: hexDeviceID(d.DeviceID),
		BootVGA:  d.BootVGA,
	}
}

// deviceListJSON is the --devices payload.
type deviceListJSON struct {
	Count    int                 `json:"count"`
	Devices  []deviceSummaryJSON `json:"devices"`
	Warnings []string            `json:"warnings"`
}

// noDevicesJSON is the --dump payload when nothing was discovered. The devices
// and processes lists stay present but empty, and the reason travels in the
// warnings rather than as an error.
type noDevicesJSON struct {
	Devices   []any    `json:"devices"`
	Processes []any    `json:"processes"`
	Warnings  []string `json:"warnings"`
}

// StartupWarnings is every warning a one-shot command should carry: the config
// loader's warnings plus the device filter's, which is a start-up condition
// rather than a per-sample fault.
func StartupWarnings(cfg *config.Config, filterWarning string) []string {
	warnings := []string{}
	if cfg != nil {
		warnings = append(warnings, cfg.Warnings...)
	}
	if filterWarning != "" {
		warnings = append(warnings, filterWarning)
	}
	return warnings
}

// Encode writes payload as JSON, appending the newline the reference does so
// shell pipelines stay tidy. HTML escaping is off because json.dump does not
// escape it and the two outputs are compared byte-for-byte.
func Encode(w io.Writer, payload any, pretty bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}
