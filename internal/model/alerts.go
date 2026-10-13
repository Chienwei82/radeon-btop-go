package model

import "fmt"

// AlertLevel says how bad it is; ordered so max() picks the panel level.
type AlertLevel int

const (
	AlertOK    AlertLevel = 0
	AlertAlert AlertLevel = 1
)

// Label returns the short display name.
func (l AlertLevel) Label() string {
	if l == AlertOK {
		return "ok"
	}
	return "alert"
}

// Thresholds holds the configured alert thresholds.
type Thresholds struct {
	TempC        float64
	PowerPercent float64
	VramPercent  float64
}

// IsExceeded reports whether any threshold is at its ceiling.
func (t Thresholds) IsExceeded() bool {
	return t.TempC > 0 || t.PowerPercent > 0 || t.VramPercent > 0
}

// AlertBreach is one metric that crossed its threshold.
type AlertBreach struct {
	Metric  string
	Reading string
	Limit   string
	Level   AlertLevel
}

// Describe returns a one-line description, e.g. "temp junction 94C > 90C".
func (b AlertBreach) Describe() string {
	return fmt.Sprintf("%s %s > %s", b.Metric, b.Reading, b.Limit)
}

// AlertState holds every threshold currently crossed for one device.
type AlertState struct {
	Level    AlertLevel
	Breaches []AlertBreach
}

// ClearAlert returns the state of a device with nothing to report.
func ClearAlert() AlertState { return AlertState{Level: AlertOK} }

// Active reports whether anything is over its limit.
func (s AlertState) Active() bool { return s.Level != AlertOK }

// Summary returns a one-line description of every breach, or "".
func (s AlertState) Summary() string {
	out := ""
	for i, b := range s.Breaches {
		if i > 0 {
			out += "; "
		}
		out += b.Describe()
	}
	return out
}

// HottestTemperature returns (label, celsius) for the hottest sensor.
func HottestTemperature(m AmdgpuMetrics) (string, float64, bool) {
	best := false
	var bestLabel string
	var bestTemp float64
	for _, t := range m.Temperatures {
		if t.Celsius == nil {
			continue
		}
		v := *t.Celsius
		if !best || v > bestTemp || (v == bestTemp && t.Label > bestLabel) {
			best, bestLabel, bestTemp = true, t.Label, v
		}
	}
	return bestLabel, bestTemp, best
}

// Evaluate compares one sample against the thresholds.
func Evaluate(m AmdgpuMetrics, thresholds Thresholds) AlertState {
	var breaches []AlertBreach
	if label, temp, ok := HottestTemperature(m); ok && thresholds.TempC > 0 && temp >= thresholds.TempC {
		breaches = append(breaches, AlertBreach{
			Metric:  "temp " + label,
			Reading: fmt.Sprintf("%.0fC", temp),
			Limit:   fmt.Sprintf("%.0fC", thresholds.TempC),
			Level:   AlertAlert,
		})
	}
	if pct := m.Power.Percent(); pct != nil && thresholds.PowerPercent > 0 && *pct >= thresholds.PowerPercent {
		breaches = append(breaches, AlertBreach{
			Metric:  "power",
			Reading: fmt.Sprintf("%.0f%%", *pct),
			Limit:   fmt.Sprintf("%.0f%%", thresholds.PowerPercent),
			Level:   AlertAlert,
		})
	}
	if pct := m.VRAM.Percent(); pct != nil && thresholds.VramPercent > 0 && *pct >= thresholds.VramPercent {
		breaches = append(breaches, AlertBreach{
			Metric:  "vram",
			Reading: fmt.Sprintf("%.0f%%", *pct),
			Limit:   fmt.Sprintf("%.0f%%", thresholds.VramPercent),
			Level:   AlertAlert,
		})
	}
	if len(breaches) == 0 {
		return ClearAlert()
	}
	level := AlertOK
	for _, b := range breaches {
		if b.Level > level {
			level = b.Level
		}
	}
	kept := breaches[:0]
	for _, b := range breaches {
		if b.Level == level {
			kept = append(kept, b)
		}
	}
	return AlertState{Level: level, Breaches: kept}
}

// WorstAlert combines several devices' states into the border signal.
func WorstAlert(states []AlertState) AlertState {
	var collected []AlertState
	for _, s := range states {
		if s.Active() {
			collected = append(collected, s)
		}
	}
	if len(collected) == 0 {
		return ClearAlert()
	}
	level := AlertOK
	var breaches []AlertBreach
	for _, s := range collected {
		if s.Level > level {
			level = s.Level
		}
		breaches = append(breaches, s.Breaches...)
	}
	return AlertState{Level: level, Breaches: breaches}
}
