package ui

import "github.com/gputop/gputop-go/internal/model"

// EvaluateAlerts judges every device in the snapshot against the thresholds,
// not just the focused one: a card the user is not looking at is exactly the
// one that can burn.
func EvaluateAlerts(snap model.GpuSnapshot, thresholds model.Thresholds, enabled bool) model.AlertState {
	if !enabled {
		return model.ClearAlert()
	}
	states := make([]model.AlertState, 0, len(snap.Devices))
	for _, device := range snap.Devices {
		states = append(states, model.Evaluate(device, thresholds))
	}
	return model.WorstAlert(states)
}

// AlertBorderColor is the panel border colour for the current flash phase.
//
// Nothing crossed leaves the theme accent. Something crossed alternates
// between the alert colour and the track colour, and a full cycle returns to
// where it started -- a border that sits on one colour is not a flash.
func AlertBorderColor(state model.AlertState, flashOn bool, th Theme) string {
	if !state.Active() {
		return th.Accent
	}
	if flashOn {
		return th.Alert
	}
	return th.Track
}

// AlertNotice is the status-line text for a crossed threshold, or "".
func AlertNotice(state model.AlertState) string { return state.Summary() }
