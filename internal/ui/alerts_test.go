package ui

import (
	"strings"
	"testing"

	"github.com/gputop/gputop-go/internal/model"
)

func hotMetrics() model.AmdgpuMetrics {
	m := testMetrics()
	hot := model.Celsius(94)
	m.Temperatures = []model.Temperature{{Label: "junction", Celsius: &hot}}
	return m
}

func TestEvaluateAlertsJudgesEveryDeviceNotJustTheFocusedOne(t *testing.T) {
	quiet := testMetrics()
	hot := hotMetrics()
	hot.Device.Index = 1
	snap := model.GpuSnapshot{Devices: []model.AmdgpuMetrics{quiet, hot}}
	state := EvaluateAlerts(snap, model.Thresholds{TempC: 90}, true)
	if !state.Active() {
		t.Fatal("a card the user is not looking at is the one that can burn")
	}
	if !strings.Contains(state.Summary(), "temp junction") {
		t.Fatalf("summary = %q, want the breached sensor named", state.Summary())
	}
}

func TestEvaluateAlertsIsSilentWhenDisabled(t *testing.T) {
	snap := model.GpuSnapshot{Devices: []model.AmdgpuMetrics{hotMetrics()}}
	if EvaluateAlerts(snap, model.Thresholds{TempC: 90}, false).Active() {
		t.Fatal("an alert that cannot be turned off is not a setting")
	}
}

func TestEvaluateAlertsLeavesAnOrdinaryCardAlone(t *testing.T) {
	cool := model.Celsius(56)
	m := testMetrics()
	m.Temperatures = []model.Temperature{{Label: "junction", Celsius: &cool}}
	draw, cap := model.Watts(45), model.Watts(211)
	m.Power = model.Power{DrawW: &draw, CapW: &cap}
	snap := model.GpuSnapshot{Devices: []model.AmdgpuMetrics{m}}
	if EvaluateAlerts(snap, model.Thresholds{TempC: 90, PowerPercent: 95, VramPercent: 92}, true).Active() {
		t.Fatal("56C and 45/211 W are comfortably inside the defaults")
	}
}

func TestAlertBorderColorIsTheAccentWhenNothingIsCrossed(t *testing.T) {
	th := DefaultTheme
	if got := AlertBorderColor(model.ClearAlert(), true, th); got != th.Accent {
		t.Fatalf("border = %q, want the theme accent %q", got, th.Accent)
	}
}

func TestAlertBorderColorAlternatesWhileActive(t *testing.T) {
	th := DefaultTheme
	active := model.AlertState{Level: model.AlertAlert}
	lit := AlertBorderColor(active, true, th)
	unlit := AlertBorderColor(active, false, th)
	if lit == unlit {
		t.Fatalf("border did not alternate: %q both ways", lit)
	}
	if lit != th.Alert || unlit != th.Track {
		t.Fatalf("lit/unlit = %q/%q, want the alert and track colours", lit, unlit)
	}
	if again := AlertBorderColor(active, true, th); again != lit {
		t.Fatalf("a full cycle returned to %q, want %q", again, lit)
	}
}

func TestAlertBorderColorReturnsToTheAccentWhenTheConditionClears(t *testing.T) {
	th := DefaultTheme
	active := model.AlertState{Level: model.AlertAlert}
	if got := AlertBorderColor(active, true, th); got == th.Accent {
		t.Fatal("a crossed threshold must leave the accent colour")
	}
	if got := AlertBorderColor(model.ClearAlert(), true, th); got != th.Accent {
		t.Fatalf("border = %q, want the accent once the condition clears", got)
	}
}

// The flash has to reach the screen: a state machine that alternates a flag
// nobody paints is not an alert. Every frame changes colour with the phase.
func TestActiveAlertFlashesThePanelFrames(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	in.Alerts = model.AlertState{Level: model.AlertAlert}
	for _, flash := range []bool{false, true} {
		in.FlashOn = flash
		want := AlertBorderColor(in.Alerts, flash, DefaultTheme)
		frames := 0
		for _, row := range RenderScreen(in) {
			plain := Plain(row)
			if !strings.HasPrefix(plain, boxTopLeft) && !strings.HasPrefix(plain, boxBottomLeft) {
				continue
			}
			frames++
			if got := row.Cells[0].Color; got != want {
				t.Fatalf("flash=%v frame colour = %q, want %q", flash, got, want)
			}
		}
		if frames == 0 {
			t.Fatalf("flash=%v drew no frames", flash)
		}
	}
}

func TestNoAlertLeavesTheFramesOnTheAccent(t *testing.T) {
	in := screenInput()
	in.Width, in.Height = 120, 40
	for _, row := range RenderScreen(in) {
		if strings.HasPrefix(Plain(row), boxTopLeft) && row.Cells[0].Color != DefaultTheme.Accent {
			t.Fatalf("frame colour = %q with nothing crossed, want the accent", row.Cells[0].Color)
		}
	}
}

func TestAlertNoticeNamesTheBreach(t *testing.T) {
	snap := model.GpuSnapshot{Devices: []model.AmdgpuMetrics{hotMetrics()}}
	state := EvaluateAlerts(snap, model.Thresholds{TempC: 90}, true)
	notice := AlertNotice(state)
	for _, want := range []string{"temp junction", "94", "90"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice = %q, want %q", notice, want)
		}
	}
	if got := AlertNotice(model.ClearAlert()); got != "" {
		t.Fatalf("AlertNotice(clear) = %q, want nothing to say", got)
	}
}
