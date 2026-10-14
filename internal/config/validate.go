package config

import "fmt"

// validate coerces, clamps and normalises the enum-ish fields after the
// document has been applied.
//
// The as* coercions already turned wrong types into defaults; this pass clamps
// values that are the right type but out of range, and drops enum values a
// future release might have written. It always leaves a working Config.
func validate(c *Config, warnings *[]string) {
	if c.IntervalMs < MinIntervalMs || c.IntervalMs > MaxIntervalMs {
		clamped := clampInt(c.IntervalMs, MinIntervalMs, MaxIntervalMs)
		*warnings = append(*warnings, fmt.Sprintf("interval_ms=%d out of range, using %d", c.IntervalMs, clamped))
		c.IntervalMs = clamped
	}
	if c.HistoryPoints < 1 {
		*warnings = append(*warnings, fmt.Sprintf("history_points=%d below 1, using 1", c.HistoryPoints))
		c.HistoryPoints = 1
	}

	if c.Process.MaxRows < 1 {
		*warnings = append(*warnings, fmt.Sprintf("process.max_rows=%d below 1, using 1", c.Process.MaxRows))
		c.Process.MaxRows = 1
	}
	if !validSortColumn(c.Process.Sort) {
		*warnings = append(*warnings, fmt.Sprintf("process.sort=%q unknown, using total", c.Process.Sort))
		c.Process.Sort = "total"
	}

	if c.Gpu.Kind != "auto" && c.Gpu.Kind != "igpu" && c.Gpu.Kind != "dgpu" {
		*warnings = append(*warnings, fmt.Sprintf("gpu.kind=%q invalid, using 'auto'", c.Gpu.Kind))
		c.Gpu.Kind = "auto"
	}

	if !validThemeName(c.UI.Theme) {
		*warnings = append(*warnings, fmt.Sprintf("ui.theme=%q unknown, using 'default'", c.UI.Theme))
		c.UI.Theme = "default"
	}
	if c.UI.GraphHeight < 0 {
		*warnings = append(*warnings, fmt.Sprintf("ui.graph_height=%d below 0, using 0 (fill available)", c.UI.GraphHeight))
		c.UI.GraphHeight = 0
	}

	validateAlerts(&c.Alerts, warnings)
	validateLog(&c.Log, warnings)
	validateState(&c.State, warnings)
}

// validateAlerts clamps the thresholds. A temperature limit above the point
// where the part throttles hard can never usefully fire, so it is clamped to
// MaxTempC; percentages above 100 are clamped for the same reason.
func validateAlerts(a *AlertsConfig, warnings *[]string) {
	if a.TempC < 0 || a.TempC > MaxTempC {
		clamped := clampFloat(a.TempC, 0, MaxTempC)
		*warnings = append(*warnings, fmt.Sprintf("alerts.temp_c=%g out of range, using %g", a.TempC, clamped))
		a.TempC = clamped
	}
	if a.PowerPercent < 0 || a.PowerPercent > 100 {
		clamped := clampFloat(a.PowerPercent, 0, 100)
		*warnings = append(*warnings, fmt.Sprintf("alerts.power_percent=%g out of range, using %g", a.PowerPercent, clamped))
		a.PowerPercent = clamped
	}
	if a.VramPercent < 0 || a.VramPercent > 100 {
		clamped := clampFloat(a.VramPercent, 0, 100)
		*warnings = append(*warnings, fmt.Sprintf("alerts.vram_percent=%g out of range, using %g", a.VramPercent, clamped))
		a.VramPercent = clamped
	}
	if a.FlashHz < 0 || a.FlashHz > 20 {
		clamped := clampFloat(a.FlashHz, 0, 20)
		*warnings = append(*warnings, fmt.Sprintf("alerts.flash_hz=%g out of range, using %g", a.FlashHz, clamped))
		a.FlashHz = clamped
	}
}

// validateLog clamps the recording options. The zstd range is 0-19 with 0
// meaning "library default".
func validateLog(l *LogConfig, warnings *[]string) {
	if l.ZstdLevel < 0 {
		*warnings = append(*warnings, fmt.Sprintf("log.zstd_level=%d out of range, using 0", l.ZstdLevel))
		l.ZstdLevel = 0
	} else if l.ZstdLevel > 19 {
		*warnings = append(*warnings, fmt.Sprintf("log.zstd_level=%d out of range, using 19", l.ZstdLevel))
		l.ZstdLevel = 19
	}
	if l.IntervalS < 0 {
		*warnings = append(*warnings, fmt.Sprintf("log.interval_s=%g below 0, using 0 (every sample)", l.IntervalS))
		l.IntervalS = 0
	}
}

// validateState rejects a saved state value this build does not understand. A
// state file outlives the release that wrote it, so a theme or a sort column
// from a future version is dropped with a warning rather than allowed to break
// start-up.
func validateState(s *StateConfig, warnings *[]string) {
	if s.Theme != nil && !validThemeName(*s.Theme) {
		*warnings = append(*warnings, fmt.Sprintf("state.theme=%q unknown, ignoring the saved value", *s.Theme))
		s.Theme = nil
	}
	if s.Sort != nil && !validSortColumn(*s.Sort) {
		*warnings = append(*warnings, fmt.Sprintf("state.sort=%q unknown, ignoring the saved value", *s.Sort))
		s.Sort = nil
	}
	if s.IntervalMs != nil {
		if *s.IntervalMs < MinIntervalMs || *s.IntervalMs > MaxIntervalMs {
			*warnings = append(*warnings, fmt.Sprintf("state.interval_ms=%d out of range, ignoring the saved value", *s.IntervalMs))
			s.IntervalMs = nil
		}
	}
	if s.GpuIndex < 0 {
		*warnings = append(*warnings, fmt.Sprintf("state.gpu_index=%d below 0, using 0", s.GpuIndex))
		s.GpuIndex = 0
	}
}
