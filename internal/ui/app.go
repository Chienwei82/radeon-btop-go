package ui

import (
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/gputop/gputop-go/internal/config"
	"github.com/gputop/gputop-go/internal/control"
	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/sampler"
	"github.com/gputop/gputop-go/internal/sessionlog"
)

// AppOptions is everything the event loop needs from the outside world.
//
// The four function fields are seams, not conveniences: they let the suite
// drive quitting, state persistence, signalling and recording without sending
// a real signal, writing the user's real config, or starting the sampler.
type AppOptions struct {
	Config    config.Config
	Sampler   *sampler.Sampler
	Log       *sessionlog.SessionLog
	SaveState func(config.StateConfig) string
	Terminate func(*model.GpuProcess, string, int) string
	OwnPID    int
}

// overlayKind is which modal, if any, owns the keyboard.
type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlayFilter
	overlayKill
)

// snapshotMsg carries one published sample. ok is false after a wait timeout,
// which is not an error: the loop simply asks again.
type snapshotMsg struct {
	snap model.GpuSnapshot
	ok   bool
}

// flashMsg is the alert flash phase tick.
type flashMsg struct{}

// App is the bubbletea model.
//
// It owns the view state and the snapshot on screen, and nothing else: the
// sampler goroutine owns sampling, the panels are pure functions of a
// snapshot, and every side effect (log write, state save, signal, resample)
// is a request this type sends through one of the seams.
type App struct {
	opts    AppOptions
	view    ViewState
	snap    *model.GpuSnapshot
	history []model.GpuSnapshot

	width, height int
	// tablePageSize is the number of process rows the table can show at the
	// current terminal height, recomputed on resize. PageUp/PageDown move the
	// selection by this many rows; it is the same value the layout gives the
	// table, so a page is what the user can actually see.
	tablePageSize int
	// helpScroll is the first visible line of the help overlay, so a terminal
	// too short to show every binding can still reach the ones at the bottom.
	helpScroll   int
	overlay      overlayKind
	filterBuffer string
	confirm      *model.GpuProcess
	alerts       model.AlertState
	flashOn      bool
	status       string
	start        time.Time
	lastRecorded time.Time
	stateDirty   bool
	shutdown     bool
	// styles caches one resolved lipgloss style per colour key. A repaint
	// touches every cell; rebuilding (and re-parsing) a style per cell was
	// pure repetition over a closed palette (theme keys + gradient stops).
	styles map[string]lipgloss.Style
}

// NewApp builds the model from a configuration and the loaded session state.
func NewApp(opts AppOptions) *App {
	if opts.SaveState == nil {
		opts.SaveState = config.SaveState
	}
	if opts.Terminate == nil {
		opts.Terminate = control.Terminate
	}
	if opts.OwnPID == 0 {
		opts.OwnPID = os.Getpid()
	}
	view := *NewViewState()
	view.ThemeName = opts.Config.UI.Theme
	view.IntervalS = opts.Config.IntervalS()
	view.Sort = ParseSort(opts.Config.Process.Sort, nil)
	view.ShowProcesses = opts.Config.Process.Show
	view.Tree = opts.Config.Process.ProcessTree
	view.FullCommand = opts.Config.Process.FullCommand
	view.Filter.ShowIdle = opts.Config.Process.ShowIdle
	view.Selected = nil

	state := opts.Config.State
	if state.Sort != nil {
		view.Sort = ParseSort(*state.Sort, state.SortReverse)
	}
	if state.Theme != nil {
		view.ThemeName = *state.Theme
	}
	if state.IntervalMs != nil {
		view.IntervalS = float64(*state.IntervalMs) / 1000.0
	}
	view.Filter.Text = state.Filter
	view.Filter.ShowIdle = state.ShowIdle
	view.Tree = state.ProcessTree
	view.FullCommand = state.FullCommand
	view.Overview = state.Overview
	view.DeviceIndex = state.GpuIndex

	return &App{opts: opts, view: view, start: time.Now()}
}

// Init starts the two clocks the loop runs on: the snapshot wait and the
// alert flash phase.
func (a *App) Init() tea.Cmd {
	return tea.Batch(a.waitCmd(), flashCmd(a.flashInterval()))
}

// waitCmd blocks in its own goroutine for the next published snapshot. It
// never blocks the event loop: bubbletea runs commands off the render path.
func (a *App) waitCmd() tea.Cmd {
	s := a.opts.Sampler
	if s == nil {
		return nil
	}
	timeout := time.Duration(max(1.0, a.view.IntervalS*3) * float64(time.Second))
	return func() tea.Msg {
		if snap := s.Wait(timeout); snap != nil {
			return snapshotMsg{snap: *snap, ok: true}
		}
		return snapshotMsg{}
	}
}

// flashInterval is one phase of the alert flash. A zero or negative
// frequency disables the timer rather than spinning it.
func (a *App) flashInterval() time.Duration {
	hz := a.opts.Config.Alerts.FlashHz
	if hz <= 0 {
		return 0
	}
	return max(100*time.Millisecond, time.Duration(float64(time.Second)/hz))
}

// flashCmd schedules the next phase tick, or nothing when flashing is off.
func flashCmd(interval time.Duration) tea.Cmd {
	if interval <= 0 {
		return nil
	}
	return tea.Tick(interval, func(time.Time) tea.Msg { return flashMsg{} })
}

// Update is the whole event loop: window size, samples, flash ticks, keys.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.tablePageSize = estimatePageSize(a.width, a.height, a.deviceCount())
		return a, nil
	case snapshotMsg:
		if !m.ok {
			return a, a.waitCmd()
		}
		a.applySnapshot(m.snap)
		return a, a.waitCmd()
	case flashMsg:
		a.flashOn = !a.flashOn
		return a, flashCmd(a.flashInterval())
	case tea.KeyPressMsg:
		return a, a.handleKey(m)
	case tea.MouseClickMsg:
		return a, a.handleMouseClick(m)
	case tea.MouseWheelMsg:
		return a, a.handleMouseWheel(m)
	}
	return a, nil
}

// handleMouseClick routes a click: a device tab switches the focused GPU, a
// click on the help overlay closes it (a modal is dismissed by clicking
// outside its box). Clicks on the process table are handled by row in a later
// pass; for now they are ignored rather than guessed at.
func (a *App) handleMouseClick(m tea.MouseClickMsg) tea.Cmd {
	if a.snap != nil && len(a.snap.Devices) > 1 && m.Y == a.tabBarRow() {
		if pos := DeviceAt(a.snap.Devices, m.X); pos >= 0 {
			a.view.DeviceIndex = a.snap.Devices[pos].Device.Index
			a.stateDirty = true
		}
	}
	return nil
}

// tabBarRow is the Y row the device tab bar sits on: after the header, which
// is three lines when a device is present. Returns -1 when there is no header.
func (a *App) tabBarRow() int {
	if a.snap == nil || len(a.snap.Devices) == 0 {
		return -1
	}
	return 3
}

// handleMouseWheel scrolls the process selection, the same way PageUp/PageDown
// does: the wheel is a page at a time, not a line at a time, because the table
// is already windowed to what fits.
func (a *App) handleMouseWheel(m tea.MouseWheelMsg) tea.Cmd {
	switch m.Mouse().Button {
	case tea.MouseWheelUp:
		a.moveSelection(-max(1, a.tablePageSize))
	case tea.MouseWheelDown:
		a.moveSelection(max(1, a.tablePageSize))
	}
	return nil
}

// applySnapshot adopts one sample: on screen, in the graphs, in the alerts,
// and in the recording.
func (a *App) applySnapshot(snap model.GpuSnapshot) {
	a.snap = &snap
	if a.opts.Sampler != nil {
		a.history = a.opts.Sampler.History()
	}
	a.alerts = EvaluateAlerts(snap, a.opts.Config.Alerts.Thresholds(), a.opts.Config.Alerts.Enabled)
	a.record(snap)
	a.dropDeadSelection()
}

// record appends the newest snapshot to the recording.
//
// Only the newest snapshot of a tick is recorded, for the same reason the UI
// renders only that one: during a slow repaint the queue can hold several, and
// writing them all would fill a recording with samples nobody ever saw.
// log.interval_s thins the stream further for a session that will run for
// hours; the check lives here rather than in the writer so the recorder stays a
// pure sink with no clock of its own.
func (a *App) record(snap model.GpuSnapshot) {
	if a.opts.Log == nil {
		return
	}
	if minimum := a.opts.Config.Log.IntervalS; minimum > 0 && !a.lastRecorded.IsZero() {
		if time.Since(a.lastRecorded).Seconds() < minimum {
			return
		}
	}
	a.lastRecorded = time.Now()
	if !a.opts.Log.Write(snap) {
		a.status = "recording stopped: " + string(a.opts.Log.Problem)
		a.opts.Log = nil
	}
}

// dropDeadSelection forgets a selection whose process has exited, rather than
// letting the highlight jump to whatever row took its place.
func (a *App) dropDeadSelection() {
	if a.view.Selected == nil || a.snap == nil {
		return
	}
	for _, p := range a.snap.Processes {
		if idOf(p) == *a.view.Selected {
			return
		}
	}
	a.view.Selected = nil
}

// handleKey routes one key press to whichever modal owns the keyboard, or to
// the view state when none does.
func (a *App) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	switch a.overlay {
	case overlayFilter:
		a.handleFilterKey(msg)
		return nil
	case overlayKill:
		return a.handleKillKey(msg)
	case overlayHelp:
		switch msg.String() {
		case "esc", "q", "h", "?":
			a.overlay = overlayNone
		case "pgdown":
			a.scrollHelp(1)
		case "pgup":
			a.scrollHelp(-1)
		case "end":
			a.scrollHelp(1 << 30)
		case "home":
			a.scrollHelp(-1 << 30)
		}
		return nil
	}
	return a.applyAction(a.view.HandleKey(msg.String()))
}

// scrollHelp moves the help overlay by delta lines, clamped to the content so
// a short reference never scrolls past its own end.
func (a *App) scrollHelp(delta int) {
	entries := DefaultHelp(control.HelpText(a.opts.Config.Process.AllowKill), a.blocksNote())
	total := len(entries) + 1 // the "esc close" line
	visible := a.height - boxOverheadY
	if visible < 1 {
		visible = 1
	}
	maxScroll := max(0, total-visible)
	a.helpScroll = min(max(a.helpScroll+delta, 0), maxScroll)
}

// applyAction performs the side effect a key asked for. Pure view-state
// transitions already happened in ViewState.HandleKey; this is the boundary
// where the app talks to the sampler, the filesystem and signals.
func (a *App) applyAction(action ViewAction) tea.Cmd {
	switch action {
	case ActionQuit:
		return a.quit()
	case ActionNextDevice:
		a.focusRelative(1)
	case ActionPrevDevice:
		a.focusRelative(-1)
	case ActionFocusDevice:
		a.focusRequestedDevice()
	case ActionEditFilter:
		a.overlay = overlayFilter
		a.filterBuffer = a.view.Filter.Text
	case ActionHelp:
		a.overlay = overlayHelp
	case ActionResample:
		if a.opts.Sampler != nil {
			a.opts.Sampler.RequestTick()
		}
	case ActionFaster, ActionSlower:
		if a.opts.Sampler != nil {
			a.opts.Sampler.SetInterval(a.view.IntervalS)
		}
		a.stateDirty = true
	case ActionCycleTheme:
		a.stateDirty = true
		a.status = "theme: " + a.view.ThemeName
	case ActionToggleProcesses, ActionToggleIdle, ActionToggleTree, ActionToggleFullCommand,
		ActionClearFilter, ActionNextSort, ActionPrevSort, ActionReverseSort, ActionToggleOverview:
		a.stateDirty = true
	case ActionKill:
		a.requestKill()
	case ActionSelectNext:
		a.moveSelection(1)
	case ActionSelectPrev:
		a.moveSelection(-1)
	case ActionSelectPageDown:
		a.moveSelection(max(1, a.tablePageSize))
	case ActionSelectPageUp:
		a.moveSelection(-max(1, a.tablePageSize))
	case ActionSelectFirst:
		a.moveSelectionTo(0)
	case ActionSelectLast:
		a.moveSelectionTo(1 << 30)
	case ActionUnknown:
	}
	return nil
}

// quit saves the session state, finishes the recording, stops the sampler and
// leaves the alternate screen.
func (a *App) quit() tea.Cmd {
	a.Shutdown()
	return tea.Quit
}

// Shutdown is the one exit path: it persists the session state, finishes the
// recording, and stops the sampler. It is idempotent, so it is safe to call
// from the key handler and again after the program returns -- which is what
// covers a SIGTERM that never produced a key press.
func (a *App) Shutdown() {
	if a.shutdown {
		return
	}
	a.shutdown = true
	if msg := a.opts.SaveState(a.state()); msg != "" {
		// Nothing to show: the screen is about to go away. The message is
		// kept on the struct so an embedding caller can read it.
		a.status = msg
	}
	if a.opts.Log != nil {
		a.opts.Log.Close()
	}
	if a.opts.Sampler != nil {
		a.opts.Sampler.Stop()
	}
}

// state projects the view state onto the state: block.
func (a *App) state() config.StateConfig {
	theme := a.view.ThemeName
	interval := model.RoundHalfEven(a.view.IntervalS * 1000)
	sort := string(a.view.Sort.Column)
	reverse := a.view.Sort.Reverse
	return config.StateConfig{
		Theme:       &theme,
		IntervalMs:  &interval,
		Sort:        &sort,
		SortReverse: &reverse,
		Filter:      a.view.Filter.Text,
		ShowIdle:    a.view.Filter.ShowIdle,
		ProcessTree: a.view.Tree,
		FullCommand: a.view.FullCommand,
		Overview:    a.view.Overview,
		GpuIndex:    a.view.DeviceIndex,
	}
}

// requestKill decides whether the confirmation may even open.
func (a *App) requestKill() {
	if !a.opts.Config.Process.AllowKill {
		a.status = "signalling disabled: set process.allow_kill = true"
		return
	}
	process := a.selectedProcess()
	if process == nil {
		a.status = "no process selected"
		return
	}
	if reason := control.Refusal(process, a.opts.OwnPID); reason != "" {
		a.status = reason
		return
	}
	a.confirm = process
	a.overlay = overlayKill
}

// handleKillKey resolves the confirmation. The target is the process captured
// when the dialog opened, not the row under the cursor now: a list that
// reorders under a modal must never signal a different process.
func (a *App) handleKillKey(msg tea.KeyPressMsg) tea.Cmd {
	process := a.confirm
	switch msg.String() {
	case "t":
		a.sendSignal(process, control.SignalTerm)
	case "k":
		a.sendSignal(process, control.SignalKill)
	case "esc":
		a.overlay = overlayNone
		a.confirm = nil
	}
	return nil
}

// sendSignal delivers the signal and closes the dialog.
func (a *App) sendSignal(process *model.GpuProcess, signalName string) {
	a.overlay = overlayNone
	a.confirm = nil
	if process == nil {
		return
	}
	if outcome := a.opts.Terminate(process, signalName, a.opts.OwnPID); outcome != "" {
		a.status = outcome
		return
	}
	a.status = signalName + " → " + strconv.Itoa(process.PID) + " " + process.Name
}

// handleFilterKey edits the filter text. Enter applies it, Esc clears it (the
// reference's own binding), Backspace removes a character, and any printable
// text is appended.
func (a *App) handleFilterKey(msg tea.KeyPressMsg) {
	switch {
	case msg.Code == tea.KeyEnter || msg.String() == "enter":
		a.view.Filter.Text = a.filterBuffer
		a.overlay = overlayNone
		a.stateDirty = true
	case msg.Code == tea.KeyEscape || msg.String() == "esc":
		a.filterBuffer = ""
		a.view.Filter.Text = ""
		a.overlay = overlayNone
		a.stateDirty = true
	case msg.Code == tea.KeyBackspace || msg.String() == "backspace":
		a.filterBuffer = dropLastRune(a.filterBuffer)
	default:
		if msg.Text != "" {
			a.filterBuffer += msg.Text
		}
	}
}

// dropLastRune removes one character, not one byte: a filter may contain
// non-ASCII text from a command line.
func dropLastRune(s string) string {
	runes := []rune(s)
	if len(runes) == 0 {
		return s
	}
	return string(runes[:len(runes)-1])
}

// selectedProcess is the client the cursor is on, or nil.
func (a *App) selectedProcess() *model.GpuProcess {
	if a.view.Selected == nil || a.snap == nil {
		return nil
	}
	for i := range a.snap.Processes {
		if idOf(a.snap.Processes[i]) == *a.view.Selected {
			return &a.snap.Processes[i]
		}
	}
	return nil
}

// moveSelection steps the cursor through the rows the table is showing. With
// no selection, down starts at the top and up starts at the bottom.
func (a *App) moveSelection(step int) {
	if a.snap == nil {
		return
	}
	rows := BuildRows(a.snap.Processes, a.view.Sort, a.view.Filter, a.view.Tree)
	if len(rows) == 0 {
		a.view.Selected = nil
		return
	}
	current := -1
	if a.view.Selected != nil {
		for i, row := range rows {
			if idOf(row.Process) == *a.view.Selected {
				current = i
				break
			}
		}
	}
	next := 0
	switch {
	case current >= 0:
		next = min(max(current+step, 0), len(rows)-1)
	case step < 0:
		next = len(rows) - 1
	}
	id := idOf(rows[next].Process)
	a.view.Selected = &id
}

// moveSelectionTo jumps the selection to an absolute row index (clamped).
func (a *App) moveSelectionTo(index int) {
	if a.snap == nil {
		return
	}
	rows := BuildRows(a.snap.Processes, a.view.Sort, a.view.Filter, a.view.Tree)
	if len(rows) == 0 {
		a.view.Selected = nil
		return
	}
	next := min(max(index, 0), len(rows)-1)
	id := idOf(rows[next].Process)
	a.view.Selected = &id
}

// deviceCount is how many devices the current snapshot reports.
func (a *App) deviceCount() int {
	if a.snap == nil {
		return 0
	}
	return len(a.snap.Devices)
}

// estimatePageSize mirrors the layout's table budget (layout.go: the table
// gets a third of what the header, tabs and footer leave). It is only used to
// turn PageUp/PageDown into a visible-page jump; the renderer stays the source
// of truth for what is actually drawn.
func estimatePageSize(width, height, devices int) int {
	const headerLines = 3
	rest := height - headerLines - footerLines
	if devices > 1 {
		rest-- // the tab bar row
	}
	if rest < 4 {
		rest = 4
	}
	if height < ShortHeight || rest < 8 {
		return 0
	}
	return rest / 3
}

// focusRelative cycles the focused GPU, wrapping at both ends.
func (a *App) focusRelative(step int) {
	if a.snap == nil || len(a.snap.Devices) == 0 {
		return
	}
	position := 0
	for i, device := range a.snap.Devices {
		if device.Device.Index == a.view.DeviceIndex {
			position = i
			break
		}
	}
	next := (position + step + len(a.snap.Devices)) % len(a.snap.Devices)
	a.view.DeviceIndex = a.snap.Devices[next].Device.Index
}

// focusRequestedDevice applies a 1-9 key, ignoring an index this machine does
// not have: a stale tab that blanks the screen would be worse than no move.
func (a *App) focusRequestedDevice() {
	if a.snap == nil || a.view.FocusDeviceArg < 0 {
		return
	}
	for _, device := range a.snap.Devices {
		if device.Device.Index == a.view.FocusDeviceArg {
			a.view.DeviceIndex = a.view.FocusDeviceArg
			return
		}
	}
}

// blocksNote is the help-overlay sentence about a panel that is not showing,
// so the reason it is absent is visible rather than inferred.
func (a *App) blocksNote() string {
	if a.snap == nil || a.snap.BlocksStatus.OK() {
		return ""
	}
	hint := a.snap.BlocksHint
	if hint == "" {
		hint = a.snap.BlocksStatus.Hint()
	}
	if hint == "" {
		return ""
	}
	return "blocks: " + hint
}

// View composes one frame: the pure screen for the current size, the modal if
// one is open, and the cells resolved to terminal styles.
func (a *App) View() tea.View {
	width, height := a.width, a.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	th := GetTheme(a.view.ThemeName)
	if a.opts.Config.UI.NoColor {
		th = MonochromeTheme
	}

	snapshot := model.GpuSnapshot{}
	if a.snap != nil {
		snapshot = *a.snap
	}
	rows := RenderScreen(LayoutInput{
		Snapshot:  snapshot,
		History:   a.history,
		View:      a.view,
		Theme:     th,
		Width:     width,
		Height:    height,
		UptimeS:   time.Since(a.start).Seconds(),
		IntervalS: a.view.IntervalS,
		Alerts:    a.alerts,
		FlashOn:   a.flashOn,
		Notice:    a.status,
	})
	if a.snap == nil {
		rows = []Row{{Cells: []Cell{{Text: "gputop: waiting for the first sample…", Color: th.Muted}}}}
	}
	rows = a.withOverlay(rows, width, height, th)

	view := tea.NewView(a.renderRows(rows, width, height))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

// scrollRows returns rows[start:] when start is a valid offset, else rows
// unchanged. It keeps the top border (row 0) pinned so the frame still reads
// as a frame while the body scrolls underneath it.
func scrollRows(rows []Row, start int) []Row {
	if start <= 0 || start >= len(rows) {
		return rows
	}
	// Keep the top border, skip `start` body rows, keep the bottom border.
	if len(rows) < 2 {
		return rows
	}
	out := make([]Row, 0, len(rows)-start)
	out = append(out, rows[0])
	out = append(out, rows[1+start:]...)
	return out
}

// withOverlay centres the open modal over the screen. It replaces the lines it
// covers rather than drawing a background: a modal is a screen of its own.
func (a *App) withOverlay(rows []Row, width, height int, th Theme) []Row {
	var box []Row
	switch a.overlay {
	case overlayHelp:
		box = scrollRows(HelpOverlay(DefaultHelp(control.HelpText(a.opts.Config.Process.AllowKill), a.blocksNote()), th), a.helpScroll)
	case overlayFilter:
		box = FilterPrompt(a.filterBuffer, th)
	case overlayKill:
		if a.confirm == nil {
			return rows
		}
		box = KillConfirm(*a.confirm, th)
	default:
		return rows
	}
	boxWidth := 0
	for _, row := range box {
		boxWidth = max(boxWidth, DisplayWidth(Plain(row)))
	}
	boxWidth = min(boxWidth, width)

	out := append([]Row(nil), rows...)
	for len(out) < height {
		out = append(out, Row{})
	}
	startY := max(0, (height-len(box))/2)
	left := max(0, (width-boxWidth)/2)
	for i, row := range box {
		y := startY + i
		if y >= height {
			break
		}
		cells := []Cell{{Text: strings.Repeat(" ", left)}}
		cells = append(cells, fitRow(row, boxWidth).Cells...)
		out[y] = Row{Cells: cells}
	}
	return out[:height]
}

// renderRows resolves theme colours to escape sequences exactly once per
// frame. --no-color returns the plain text, which is also what the golden
// tests read.
func (a *App) renderRows(rows []Row, width, height int) string {
	if len(rows) > height {
		rows = rows[:height]
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		var b strings.Builder
		for _, cell := range row.Cells {
			b.WriteString(a.styleCell(cell))
		}
		if plainWidth := DisplayWidth(Plain(row)); plainWidth < width {
			b.WriteString(strings.Repeat(" ", width-plainWidth))
		}
		lines = append(lines, b.String())
	}
	return strings.Join(lines, "\n")
}

// styleCell paints one cell. The colour key is a hex value with optional
// "|bold" / "|reverse" suffixes, which is the same key the pure panels carry.
func (a *App) styleCell(cell Cell) string {
	if a.opts.Config.UI.NoColor || cell.Color == "" {
		return cell.Text
	}
	if style, ok := a.styles[cell.Color]; ok {
		return style.Render(cell.Text)
	}
	parsed := ParseCellStyle(cell.Color)
	style := lipgloss.NewStyle()
	if parsed.Colour != "" {
		style = style.Foreground(lipgloss.Color(parsed.Colour))
	}
	if parsed.Bold {
		style = style.Bold(true)
	}
	if parsed.Reverse {
		style = style.Reverse(true)
	}
	if a.styles == nil {
		a.styles = make(map[string]lipgloss.Style)
	}
	a.styles[cell.Color] = style
	return style.Render(cell.Text)
}

// Status is the last notice the app produced: a refused signal, a save
// failure, the theme it just switched to. Exposed for tests and embedders.
func (a *App) Status() string { return a.status }
