package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/sessionlog"
)

func appConfig(allowKill bool) config.Config {
	cfg := config.Default()
	cfg.IntervalMs = 1000
	cfg.Process.AllowKill = allowKill
	cfg.Alerts.FlashHz = 1
	cfg.UI.Theme = "default"
	return cfg
}

func appProcess(pid int, name string) model.GpuProcess {
	// Resident memory, because a client with none is "demonstrably idle" and
	// the table hides those by default: a zero-memory fixture would describe
	// a process gputop deliberately does not show.
	vram, gtt := model.Bytes(256*1024*1024), model.Bytes(64*1024*1024)
	return model.GpuProcess{
		PID: pid, Name: name, User: "david",
		BDF: "0000:0c:00.0", ClientID: pid,
		Cmdline:  []string{name, "--scene", "42"},
		VramUsed: &vram, GttUsed: &gtt,
	}
}

func appSnapshot(m model.AmdgpuMetrics, procs ...model.GpuProcess) model.GpuSnapshot {
	return model.GpuSnapshot{
		Sequence: 1, TimestampNs: 1_000_000, WallClockNs: 1_700_000_000_000_000_000,
		Devices: []model.AmdgpuMetrics{m}, Processes: procs,
		VisibleProcessCount: len(procs), TotalProcessCount: len(procs),
	}
}

// press delivers one key the way the event loop would. The text field is set
// for printable keys because that is what bubbletea v2 reports.
func press(a *App, key string) tea.Cmd {
	switch key {
	case "down":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	case "up":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	case "tab":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	case "esc":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	case "enter":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "backspace":
		return a.handleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	default:
		r := []rune(key)[0]
		return a.handleKey(tea.KeyPressMsg{Code: r, Text: key})
	}
}

func viewText(a *App) string { return a.View().Content }

func TestAppStartsFromTheSavedState(t *testing.T) {
	cfg := appConfig(false)
	theme, interval, sort := "dracula", 250, "vram"
	reverse := true
	cfg.State = config.StateConfig{
		Theme: &theme, IntervalMs: &interval, Sort: &sort, SortReverse: &reverse,
		Filter: "render", ShowIdle: true, ProcessTree: true,
		FullCommand: true, Overview: true, GpuIndex: 1,
	}
	app := NewApp(AppOptions{Config: cfg})
	if app.view.ThemeName != "dracula" || app.view.IntervalS != 0.25 {
		t.Errorf("theme/interval = %q/%v, want dracula/0.25", app.view.ThemeName, app.view.IntervalS)
	}
	if string(app.view.Sort.Column) != "vram" || !app.view.Sort.Reverse {
		t.Errorf("sort = %+v, want vram reversed", app.view.Sort)
	}
	if app.view.Filter.Text != "render" || !app.view.Filter.ShowIdle ||
		!app.view.Tree || !app.view.FullCommand || !app.view.Overview || app.view.DeviceIndex != 1 {
		t.Errorf("view did not start from the saved state: %+v", app.view)
	}
}

func TestAppAdoptsASnapshotAndWritesTheRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.csv")
	log := sessionlog.OpenLog(path, 0)
	t.Cleanup(log.Close)
	app := NewApp(AppOptions{Config: appConfig(false), Log: log})

	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})

	if got := viewText(app); !strings.Contains(got, "AMD Radeon Test") {
		t.Fatalf("screen does not show the device: %q", got)
	}
	if log.Records() != 1 {
		t.Fatalf("records = %d, want 1", log.Records())
	}
	lines, err := sessionlog.ReadLog(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("log lines = %d, want a header and one row", len(lines))
	}
}

func TestAppFilterModalEditsAndApplies(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})

	press(app, "/")
	if app.overlay != overlayFilter {
		t.Fatalf("overlay = %v, want the filter prompt", app.overlay)
	}
	for _, ch := range []string{"r", "e", "n"} {
		press(app, ch)
	}
	if app.filterBuffer != "ren" {
		t.Fatalf("buffer = %q, want ren", app.filterBuffer)
	}
	press(app, "backspace")
	if app.filterBuffer != "re" {
		t.Fatalf("buffer after backspace = %q, want re", app.filterBuffer)
	}
	press(app, "enter")
	if app.overlay != overlayNone || app.view.Filter.Text != "re" {
		t.Fatalf("overlay/filter = %v/%q, want closed and re", app.overlay, app.view.Filter.Text)
	}
}

func TestAppFilterEscapeClears(t *testing.T) {
	cfg := appConfig(false)
	cfg.State.Filter = "stale"
	app := NewApp(AppOptions{Config: cfg})
	press(app, "/")
	press(app, "esc")
	if app.overlay != overlayNone || app.view.Filter.Text != "" || app.filterBuffer != "" {
		t.Fatalf("esc left overlay=%v filter=%q buffer=%q", app.overlay, app.view.Filter.Text, app.filterBuffer)
	}
}

func TestAppKillIsRefusedWhenDisabled(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	press(app, "down")
	press(app, "k")
	if app.overlay != overlayNone {
		t.Fatalf("overlay = %v, want none when signalling is disabled", app.overlay)
	}
	if !strings.Contains(app.Status(), "allow_kill") {
		t.Fatalf("status = %q, want the configuration key named", app.Status())
	}
}

func TestAppKillAsksForConfirmationAndSendsTheChosenSignal(t *testing.T) {
	var got struct {
		pid    int
		signal string
	}
	calls := 0
	cfg := appConfig(true)
	app := NewApp(AppOptions{
		Config: cfg,
		Terminate: func(p *model.GpuProcess, signal string, _ int) string {
			calls++
			got.pid, got.signal = p.PID, signal
			return ""
		},
	})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	press(app, "down")
	press(app, "k")
	if app.overlay != overlayKill || app.confirm == nil || app.confirm.PID != 4242 {
		t.Fatalf("confirmation did not open on the selected process: %+v", app.confirm)
	}
	press(app, "t")
	if calls != 1 || got.pid != 4242 || got.signal != "SIGTERM" {
		t.Fatalf("terminate = %+v after %d calls, want pid 4242 SIGTERM", got, calls)
	}
	if app.overlay != overlayNone {
		t.Fatalf("overlay = %v, want closed after the signal", app.overlay)
	}
	if !strings.Contains(app.Status(), "SIGTERM") {
		t.Fatalf("status = %q, want the delivered signal", app.Status())
	}
}

func TestAppKillConfirmEscSendsNothing(t *testing.T) {
	calls := 0
	app := NewApp(AppOptions{
		Config: appConfig(true),
		Terminate: func(*model.GpuProcess, string, int) string {
			calls++
			return ""
		},
	})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	press(app, "down")
	press(app, "k")
	press(app, "esc")
	if calls != 0 || app.overlay != overlayNone || app.confirm != nil {
		t.Fatalf("esc sent %d signals (overlay %v)", calls, app.overlay)
	}
}

func TestAppDropsADeadSelection(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	press(app, "down")
	if app.view.Selected == nil {
		t.Fatal("no selection after down")
	}
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(7777, "other")), ok: true})
	if app.view.Selected != nil {
		t.Fatal("a selection whose process exited survived the next sample")
	}
}

func TestAppQuitSavesStateAndClosesTheRecording(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.json")
	log := sessionlog.OpenLog(path, 0)
	saved := 0
	var savedState config.StateConfig
	app := NewApp(AppOptions{
		Config: appConfig(false),
		Log:    log,
		SaveState: func(state config.StateConfig) string {
			saved++
			savedState = state
			return ""
		},
	})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	press(app, "m") // theme cycle marks the state dirty
	cmd := press(app, "q")
	if cmd == nil {
		t.Fatal("q returned no command; the program would not quit")
	}
	if saved != 1 {
		t.Fatalf("state saved %d times, want 1", saved)
	}
	if savedState.Theme == nil || *savedState.Theme != app.view.ThemeName {
		t.Fatalf("saved theme = %v, want %q", savedState.Theme, app.view.ThemeName)
	}
	if log.IsOpen() {
		t.Fatal("the recording was not closed on quit")
	}
	// A second shutdown (the post-Run call) must not save again.
	app.Shutdown()
	if saved != 1 {
		t.Fatalf("Shutdown saved again: %d", saved)
	}
	lines, err := sessionlog.ReadLog(path)
	if err != nil || len(lines) != 1 {
		t.Fatalf("recording = %v (err %v), want one JSON line", lines, err)
	}
}

func TestAppHelpOverlayExplainsWhyKillIsDisabled(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	press(app, "h")
	if app.overlay != overlayHelp {
		t.Fatalf("overlay = %v, want help", app.overlay)
	}
	if text := viewText(app); !strings.Contains(text, "allow_kill") {
		t.Fatalf("help does not explain the disabled binding: %q", text)
	}
	press(app, "esc")
	if app.overlay != overlayNone {
		t.Fatalf("esc did not close help: %v", app.overlay)
	}
}

func TestAppNoColorEmitsNoEscapeSequences(t *testing.T) {
	cfg := appConfig(false)
	cfg.UI.NoColor = true
	app := NewApp(AppOptions{Config: cfg})
	app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics(), appProcess(4242, "renderfarm")), ok: true})
	if text := viewText(app); strings.Contains(text, "\x1b[") {
		t.Fatalf("--no-color emitted an escape sequence: %q", text)
	}
}

func TestAppTabCyclesDevicesAndWraps(t *testing.T) {
	second := fullMetrics()
	second.Device.Index = 1
	second.Device.Name = "AMD Radeon Second"
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(snapshotMsg{snap: model.GpuSnapshot{
		Sequence: 1, Devices: []model.AmdgpuMetrics{fullMetrics(), second},
		VisibleProcessCount: 0, TotalProcessCount: 0,
	}, ok: true})

	press(app, "tab")
	if app.view.DeviceIndex != 1 {
		t.Fatalf("device index = %d, want 1", app.view.DeviceIndex)
	}
	press(app, "tab")
	if app.view.DeviceIndex != 0 {
		t.Fatalf("device index = %d, want a wrap back to 0", app.view.DeviceIndex)
	}
}

func TestAppFlashTickAlternatesTheAlertPhase(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	if app.flashOn {
		t.Fatal("flash starts on")
	}
	app.Update(flashMsg{})
	if !app.flashOn {
		t.Fatal("flash did not turn on")
	}
	app.Update(flashMsg{})
	if app.flashOn {
		t.Fatal("flash did not turn off")
	}
}

func TestAppThinsTheRecordingAtTheConfiguredInterval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.csv")
	log := sessionlog.OpenLog(path, 0)
	t.Cleanup(log.Close)
	cfg := appConfig(false)
	cfg.Log.IntervalS = 3600 // far longer than the test

	app := NewApp(AppOptions{Config: cfg, Log: log})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics()), ok: true})
	app.Update(snapshotMsg{snap: appSnapshot(fullMetrics()), ok: true})
	if log.Records() != 1 {
		t.Fatalf("records = %d, want 1: log.interval_s was not applied", log.Records())
	}
}

func TestAppWaitingFrameIsNotAnEmptyScreen(t *testing.T) {
	app := NewApp(AppOptions{Config: appConfig(false)})
	app.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if text := viewText(app); !strings.Contains(text, "waiting for the first sample") {
		t.Fatalf("first frame = %q, want an explanation", text)
	}
}

// Reverse video is part of the cell contract (the active GPU badge uses it),
// so a style key that carries |reverse must reach lipgloss. Before this test
// the suffix was passed through as if it were part of the colour and the
// badge lost its highlight.
func TestStyleCellAppliesReverseVideo(t *testing.T) {
	app := &App{}
	got := app.styleCell(Cell{Text: " 1 ", Color: "#e6e6e6|reverse"})
	if !strings.Contains(got, "\x1b[7;") && !strings.Contains(got, "\x1b[7m") {
		t.Fatalf("styleCell(reverse) = %q, want a reverse-video escape", got)
	}
}

func TestStyleCellAppliesBoldAndReverseTogether(t *testing.T) {
	app := &App{}
	got := app.styleCell(Cell{Text: "Q", Color: "#e6e6e6|bold|reverse"})
	if !strings.Contains(got, "\x1b[") || !strings.Contains(got, ";") {
		t.Fatalf("styleCell(bold|reverse) = %q, want a combined style escape", got)
	}
	if !strings.Contains(got, "1") || !strings.Contains(got, "7") {
		t.Fatalf("styleCell(bold|reverse) = %q, want both bold and reverse enabled", got)
	}
}
