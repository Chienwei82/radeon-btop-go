package ui

import (
	"strings"

	"github.com/gputop/gputop-go/internal/model"
	"github.com/gputop/gputop-go/internal/sampler"
)

// NarrowWidth collapses the layout to a single column below this width.
const NarrowWidth = 72

// ShortHeight hides the per-process table below this height.
const ShortHeight = 20

// MinHeightForMemoryGraphs hides the memory graphs below this height; an
// empty bordered box reads as a bug.
const MinHeightForMemoryGraphs = 28

// IntervalStepS is the +/- key step in seconds.
const IntervalStepS = 0.25

// KeyBinding is one documented key: the bubbletea key string plus what it
// does. The help overlay renders from this table so it cannot drift.
type KeyBinding struct {
	Key         string
	Description string
}

// KeyBindings is the canonical 30 bindings.
var KeyBindings = []KeyBinding{
	{"q", "quit"},
	{"tab", "next GPU"},
	{"shift+tab", "previous GPU"},
	{"o", "overview: every GPU at once"},
	{"1", "jump to GPU 1"},
	{"2", "jump to GPU 2"},
	{"3", "jump to GPU 3"},
	{"4", "jump to GPU 4"},
	{"5", "jump to GPU 5"},
	{"6", "jump to GPU 6"},
	{"7", "jump to GPU 7"},
	{"8", "jump to GPU 8"},
	{"9", "jump to GPU 9"},
	{"R", "sample now"},
	{"+", "faster sampling"},
	{"-", "slower sampling"},
	{"m", "cycle colour theme"},
	{"p", "show or hide the process table"},
	{"s", "next sort column"},
	{"S", "previous sort column"},
	{"r", "reverse the sort"},
	{"/", "filter processes"},
	{"esc", "clear the filter"},
	{"a", "show or hide idle processes"},
	{"t", "process tree on / off"},
	{"c", "full command line"},
	{"up", "move the selection"},
	{"down", "move the selection"},
	{"k", "signal the selected process"},
	{"h", "this help"},
}

// ViewAction is what a key asks the app to do. Pure state transitions live
// on ViewState; side effects (resample, kill, overlays) are decided by the
// bubbletea Update from the returned action.
type ViewAction int

const (
	ActionUnknown ViewAction = iota
	ActionQuit
	ActionNextDevice
	ActionPrevDevice
	ActionFocusDevice
	ActionToggleOverview
	ActionEditFilter
	ActionClearFilter
	ActionHelp
	ActionResample
	ActionFaster
	ActionSlower
	ActionCycleTheme
	ActionToggleProcesses
	ActionNextSort
	ActionPrevSort
	ActionReverseSort
	ActionToggleIdle
	ActionToggleTree
	ActionToggleFullCommand
	ActionSelectNext
	ActionSelectPrev
	ActionKill
)

// ViewState is the TUI's view state: sort, filter, theme, interval, device
// focus. Held by the bubbletea model, derived from config at construction,
// written back when it changes. What the table shows is decided by
// procview; this only decides what to ask it for.
type ViewState struct {
	Sort           SortState
	Filter         FilterState
	ThemeName      string
	NoColor        bool
	IntervalS      float64
	DeviceIndex    int
	Overview       bool
	ShowProcesses  bool
	Tree           bool
	FullCommand    bool
	Selected       *procID
	FocusDeviceArg int // -1 when the last key was not 1-9
}

// NewViewState returns the default view state.
func NewViewState() *ViewState {
	return &ViewState{
		Sort:           DefaultSort,
		ThemeName:      model.DefaultThemeName,
		IntervalS:      sampler.DefaultIntervalS,
		ShowProcesses:  true,
		FocusDeviceArg: -1,
	}
}

// HandleKey applies one key press, returning what the app should do next.
// Pure state transition: no I/O, no sampler calls. The case-sensitive keys
// (R/S/r) match before the lowercase fold.
func (v *ViewState) HandleKey(key string) ViewAction {
	switch key {
	case "R":
		return ActionResample
	case "S":
		v.Sort = v.Sort.Prev()
		return ActionPrevSort
	case "r":
		v.Sort = v.Sort.Toggle()
		return ActionReverseSort
	}
	switch strings.ToLower(key) {
	case "q", "ctrl+c":
		return ActionQuit
	case "tab":
		v.Overview = false
		return ActionNextDevice
	case "shift+tab":
		v.Overview = false
		return ActionPrevDevice
	case "o":
		v.Overview = !v.Overview
		return ActionToggleOverview
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		v.FocusDeviceArg = int(key[0] - '1')
		v.Overview = false
		return ActionFocusDevice
	case "+", "=", "plus":
		v.IntervalS = sampler.ClampInterval(v.IntervalS - IntervalStepS)
		return ActionFaster
	case "-", "_", "minus":
		v.IntervalS = sampler.ClampInterval(v.IntervalS + IntervalStepS)
		return ActionSlower
	case "m":
		v.ThemeName = nextTheme(v.ThemeName)
		return ActionCycleTheme
	case "p":
		v.ShowProcesses = !v.ShowProcesses
		return ActionToggleProcesses
	case "s":
		v.Sort = v.Sort.Next()
		return ActionNextSort
	case "/", "slash":
		return ActionEditFilter
	case "esc", "escape":
		if v.Filter.Text != "" {
			v.Filter.Text = ""
			return ActionClearFilter
		}
		return ActionUnknown
	case "a":
		v.Filter.ShowIdle = !v.Filter.ShowIdle
		return ActionToggleIdle
	case "t":
		v.Tree = !v.Tree
		return ActionToggleTree
	case "c":
		v.FullCommand = !v.FullCommand
		return ActionToggleFullCommand
	case "up":
		return ActionSelectPrev
	case "down":
		return ActionSelectNext
	case "k":
		return ActionKill
	case "h", "?":
		return ActionHelp
	}
	return ActionUnknown
}

func nextTheme(name string) string {
	for i, n := range model.ThemeNames {
		if n == name {
			return model.ThemeNames[(i+1)%len(model.ThemeNames)]
		}
	}
	return model.DefaultThemeName
}
