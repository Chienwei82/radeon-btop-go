package ui

import (
	"fmt"
	stdsort "sort"
	"strings"

	"github.com/gputop/gputop-go/internal/model"
)

// ColumnOrder is the displayed column order. total is sortable but not
// shown.
var ColumnOrder = []model.SortColumn{
	model.SortPID, model.SortUser, model.SortCommand, model.SortGfx,
	model.SortCompute, model.SortDMA, model.SortEncDec, model.SortVRAM, model.SortGTT,
}

// ColumnTitles are the headings per column.
var ColumnTitles = map[model.SortColumn]string{
	model.SortTotal: "Σ", model.SortPID: "PID", model.SortUser: "USER",
	model.SortCommand: "COMMAND", model.SortGfx: "GFX %", model.SortCompute: "COMPUTE %",
	model.SortDMA: "DMA %", model.SortEncDec: "ENC/DEC %",
	model.SortVRAM: "VRAM", model.SortGTT: "GTT",
}

// ColumnWidths are the column widths in cells. command takes the slack:
// it is the only unbounded column, and a process name is what a user
// scans for.
var ColumnWidths = map[model.SortColumn]int{
	model.SortPID: 7, model.SortUser: 10, model.SortCommand: 26,
	model.SortGfx: 6, model.SortCompute: 9, model.SortDMA: 6,
	model.SortEncDec: 9, model.SortVRAM: 9, model.SortGTT: 9,
}

// Tree drawing glyphs, matching the block style of the bars.
const (
	TreeBranch = "├─ "
	TreeLast   = "└─ "
	TreeBar    = "│  "
	TreeGap    = "   "
)

// SortState is which column the table is ordered by, and in which direction.
type SortState struct {
	Column  model.SortColumn
	Reverse bool
}

// DefaultSort is total engine load, busiest first.
var DefaultSort = SortState{Column: model.SortTotal, Reverse: true}

// numericColumns rank by measurement, so a new selection starts
// descending (largest first, which is what every "top" view reads as).
var numericColumns = map[model.SortColumn]bool{
	model.SortTotal: true, model.SortGfx: true, model.SortCompute: true,
	model.SortDMA: true, model.SortEncDec: true, model.SortVRAM: true, model.SortGTT: true,
	model.SortPID: true,
}

// NumericSortColumns rank by how hard a client is working (pid excluded:
// it is an identifier, so ordering by it makes the highest-numbered
// process "top" while it may be the least busy one on screen).
var NumericSortColumns = map[model.SortColumn]bool{
	model.SortTotal: true, model.SortGfx: true, model.SortCompute: true,
	model.SortDMA: true, model.SortEncDec: true, model.SortVRAM: true, model.SortGTT: true,
}

// ParseSort builds a state from a config string, falling back to default.
func ParseSort(value string, reverse *bool) SortState {
	v := strings.ToLower(strings.TrimSpace(value))
	if !model.ValidSortColumn(v) {
		return DefaultSort
	}
	col := model.SortColumn(v)
	rev := numericColumns[col]
	if reverse != nil {
		rev = *reverse
	}
	return SortState{Column: col, Reverse: rev}
}

// Describe names the config key plus direction marker.
func (s SortState) Describe() string {
	marker := "▲"
	if s.Reverse {
		marker = "▼"
	}
	return string(s.Column) + " " + marker
}

// Next moves to the next sortable column, wrapping around.
func (s SortState) Next() SortState { return s.step(1) }

// Prev moves to the previous sortable column.
func (s SortState) Prev() SortState { return s.step(-1) }

func (s SortState) step(dir int) SortState {
	i := 0
	for j, c := range model.SortColumns {
		if c == s.Column {
			i = j
		}
	}
	next := model.SortColumns[(i+dir+len(model.SortColumns))%len(model.SortColumns)]
	return SortState{Column: next, Reverse: numericColumns[next]}
}

// Toggle flips only the direction.
func (s SortState) Toggle() SortState { return SortState{Column: s.Column, Reverse: !s.Reverse} }

// Heading returns the heading text with the marker on the active column.
func (s SortState) Heading(col model.SortColumn) string {
	title := ColumnTitles[col]
	if col == s.Column {
		if s.Reverse {
			return title + " ▼"
		}
		return title + " ▲"
	}
	return title
}

// FilterState is what the table hides.
type FilterState struct {
	Text              string
	MinUsagePercent   float64
	HideKernelThreads bool
	ShowIdle          bool
	DeviceScope       string
}

// Describe says what is hidden.
func (f FilterState) Describe() string {
	parts := []string{}
	if f.Text != "" {
		parts = append(parts, "filter "+f.Text)
	}
	if !f.ShowIdle {
		parts = append(parts, "idle hidden")
	}
	if f.DeviceScope != "" {
		parts = append(parts, "gpu "+f.DeviceScope)
	}
	if len(parts) == 0 {
		return "all"
	}
	return strings.Join(parts, ", ")
}

// ColumnValue is the value a column sorts by, or nil when unknown. nil is
// meaningful and is not zero: a client seen for the first time has no
// engine baseline, and 0% would claim it is idle when the truth is "not
// measured yet".
func ColumnValue(p model.GpuProcess, column model.SortColumn) any {
	switch column {
	case model.SortTotal:
		return p.EnginePercent()
	case model.SortPID:
		return p.PID
	case model.SortUser:
		return strings.ToLower(p.User)
	case model.SortCommand:
		return strings.ToLower(p.Command())
	case model.SortGfx:
		return optFloat(p.EnginePercentFor("gfx"))
	case model.SortCompute:
		return optFloat(p.EnginePercentFor("compute"))
	case model.SortDMA:
		return optFloat(p.EnginePercentFor("dma"))
	case model.SortEncDec:
		return optFloat(p.EncDecPercent())
	case model.SortVRAM:
		return optBytes(p.VramUsed)
	case model.SortGTT:
		return optBytes(p.GttUsed)
	}
	return nil
}

func optFloat(v *model.Percent) any {
	if v == nil {
		return nil
	}
	return float64(*v)
}

func optBytes(v *model.Bytes) any {
	if v == nil {
		return nil
	}
	return *v
}

// SortProcesses orders clients. Unknown values sort last in both
// directions; ties break on (pid, bdf, client_id) so rows do not swap
// places between one-second samples.
func SortProcesses(procs []model.GpuProcess, state SortState) []model.GpuProcess {
	measured := make([]model.GpuProcess, 0, len(procs))
	var unmeasured []model.GpuProcess
	for _, p := range procs {
		if ColumnValue(p, state.Column) == nil {
			unmeasured = append(unmeasured, p)
		} else {
			measured = append(measured, p)
		}
	}
	// One stable pass with a full comparator: value first (direction
	// aware), then the identity tie-break pinned ascending so equal rows
	// never swap places when the direction flips.
	stdsort.SliceStable(measured, func(i, j int) bool {
		vi, vj := ColumnValue(measured[i], state.Column), ColumnValue(measured[j], state.Column)
		if !equalValue(vi, vj) {
			if state.Reverse {
				return greaterValue(vi, vj)
			}
			return lessValue(vi, vj)
		}
		return tieLess(measured[i], measured[j])
	})
	stdsort.SliceStable(unmeasured, func(i, j int) bool {
		return tieLess(unmeasured[i], unmeasured[j])
	})
	return append(measured, unmeasured...)
}

func tieLess(a, b model.GpuProcess) bool {
	if a.PID != b.PID {
		return a.PID < b.PID
	}
	if a.BDF != b.BDF {
		return a.BDF < b.BDF
	}
	return a.ClientID < b.ClientID
}

func lessValue(a, b any) bool {
	switch av := a.(type) {
	case float64:
		bv, ok := b.(float64)
		return ok && av < bv
	case int:
		switch bv := b.(type) {
		case int:
			return av < bv
		case int64:
			return int64(av) < bv
		}
	case int64:
		switch bv := b.(type) {
		case int64:
			return av < bv
		case int:
			return av < int64(bv)
		}
	case string:
		bv, ok := b.(string)
		return ok && av < bv
	}
	return false
}

func greaterValue(a, b any) bool { return lessValue(b, a) }

func equalValue(a, b any) bool { return !lessValue(a, b) && !lessValue(b, a) }

// FilterProcesses keeps the rows that survive the filter state.
func FilterProcesses(procs []model.GpuProcess, f FilterState) []model.GpuProcess {
	needle := strings.ToLower(strings.TrimSpace(f.Text))
	out := make([]model.GpuProcess, 0, len(procs))
	for _, p := range procs {
		if f.DeviceScope != "" && p.BDF != f.DeviceScope {
			continue
		}
		if f.HideKernelThreads && p.IsKernelThread() {
			continue
		}
		if !f.ShowIdle && p.IsIdle() {
			continue
		}
		if f.MinUsagePercent > 0 && p.EnginePercent() <= f.MinUsagePercent {
			continue
		}
		if needle != "" && !matchesText(p, needle) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func matchesText(p model.GpuProcess, needle string) bool {
	if strings.Contains(strings.ToLower(p.Name), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(p.Command()), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(p.User), needle) {
		return true
	}
	if strings.Contains(fmt.Sprint(p.PID), needle) {
		return true
	}
	return false
}

// procID is the kernel-reported dedup pair: the table's stable identity.
type procID struct {
	bdf      model.Bdf
	clientID model.ClientId
}

func idOf(p model.GpuProcess) procID { return procID{bdf: p.BDF, clientID: p.ClientID} }

// ProcessRow is one rendered table line.
type ProcessRow struct {
	Process     model.GpuProcess
	Depth       int
	HasChildren bool
	IsLast      bool
	Guides      []bool
}

// Prefix returns the tree guides for this row, empty at depth 0.
func (r ProcessRow) Prefix() string {
	if r.Depth == 0 {
		return ""
	}
	var sb strings.Builder
	for _, g := range r.Guides {
		if g {
			sb.WriteString(TreeBar)
		} else {
			sb.WriteString(TreeGap)
		}
	}
	if r.IsLast {
		sb.WriteString(TreeLast)
	} else {
		sb.WriteString(TreeBranch)
	}
	return sb.String()
}

// FlatRows wraps sorted clients with no depth.
func FlatRows(procs []model.GpuProcess) []ProcessRow {
	rows := make([]ProcessRow, 0, len(procs))
	for _, p := range procs {
		rows = append(rows, ProcessRow{Process: p, IsLast: true})
	}
	return rows
}

// TreeRows nests children under parents by PPID. A parent that is not a
// client (filtered out, unreadable, or a self-parent/cycle) makes its
// child a root; cycles terminate and every client appears exactly once.
func TreeRows(procs []model.GpuProcess, sort SortState) []ProcessRow {
	ordered := SortProcesses(procs, sort)
	byPID := map[int][]model.GpuProcess{}
	for _, p := range ordered {
		byPID[p.PID] = append(byPID[p.PID], p)
	}
	inTable := map[procID]bool{}
	for _, p := range ordered {
		inTable[idOf(p)] = true
	}
	children := map[procID][]model.GpuProcess{}
	var roots []model.GpuProcess
	for _, p := range ordered {
		parent := findParent(p, byPID, inTable)
		if parent == nil {
			roots = append(roots, p)
		} else {
			children[idOf(*parent)] = append(children[idOf(*parent)], p)
		}
	}
	order := map[procID]int{}
	for i, p := range ordered {
		order[idOf(p)] = i
	}
	for parent, kids := range children {
		stdsort.SliceStable(kids, func(i, j int) bool { return order[idOf(kids[i])] < order[idOf(kids[j])] })
		children[parent] = kids
	}
	var rows []ProcessRow
	emitted := map[procID]bool{}
	var emit func(p model.GpuProcess, depth int, guides []bool, last bool, chain map[procID]bool)
	emit = func(p model.GpuProcess, depth int, guides []bool, last bool, chain map[procID]bool) {
		rows = append(rows, ProcessRow{
			Process: p, Depth: depth, HasChildren: len(children[idOf(p)]) > 0,
			IsLast: last, Guides: append([]bool(nil), guides...),
		})
		emitted[idOf(p)] = true
		kids := children[idOf(p)]
		for i, k := range kids {
			if chain[idOf(k)] {
				continue
			}
			next := map[procID]bool{}
			for c := range chain {
				next[c] = true
			}
			next[idOf(p)] = true
			ng := append(append([]bool(nil), guides...), !last)
			emit(k, depth+1, ng, i == len(kids)-1, next)
		}
	}
	for i, r := range roots {
		emit(r, 0, nil, i == len(roots)-1, map[procID]bool{})
	}
	for _, p := range ordered {
		if !emitted[idOf(p)] {
			emit(p, 0, nil, true, map[procID]bool{})
		}
	}
	return rows
}

func findParent(p model.GpuProcess, byPID map[int][]model.GpuProcess, inTable map[procID]bool) *model.GpuProcess {
	if p.PPID == nil || *p.PPID == p.PID {
		return nil
	}
	for _, c := range byPID[*p.PPID] {
		if c.PID == p.PID {
			continue // same PID + different client = sibling, never nested
		}
		if inTable[idOf(c)] {
			out := c
			return &out
		}
	}
	return nil
}

// BuildRows filters, orders and shapes the rows in one call: the single
// entry point the table renderer uses.
func BuildRows(procs []model.GpuProcess, sort SortState, filters FilterState, tree bool) []ProcessRow {
	visible := FilterProcesses(procs, filters)
	if tree {
		return TreeRows(visible, sort)
	}
	return FlatRows(SortProcesses(visible, sort))
}

// BusiestProcesses returns the clients tied for the most GPU work, or
// nothing when nothing is busy. Ranked by magnitude, not table position:
// the sort direction is ignored, and a non-measurement column yields nothing.
func BusiestProcesses(procs []model.GpuProcess, sort SortState) []model.GpuProcess {
	if len(procs) == 0 || !NumericSortColumns[sort.Column] {
		return nil
	}
	best := 0.0
	found := false
	for _, p := range procs {
		v, ok := numericColumnValue(p, sort.Column)
		if !ok {
			continue
		}
		found = true
		if v > best {
			best = v
		}
	}
	if !found || best <= 0 {
		return nil
	}
	var out []model.GpuProcess
	for _, p := range procs {
		if v, ok := numericColumnValue(p, sort.Column); ok && v == best {
			out = append(out, p)
		}
	}
	return out
}

func numericColumnValue(p model.GpuProcess, col model.SortColumn) (float64, bool) {
	switch v := ColumnValue(p, col).(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

// CellValue is the rendered text of one cell. Percentages keep one decimal
// because the interesting range is 0-5%; memory uses the shared byte
// formatter so the table and the memory panel never disagree.
func CellValue(p model.GpuProcess, column model.SortColumn, fullCommand bool) string {
	switch column {
	case model.SortPID:
		return fmt.Sprint(p.PID)
	case model.SortUser:
		return p.User
	case model.SortCommand:
		if fullCommand {
			return p.Command()
		}
		return p.Name
	case model.SortGfx:
		return FmtPercent(p.EnginePercentFor("gfx"), 1)
	case model.SortCompute:
		return FmtPercent(p.EnginePercentFor("compute"), 1)
	case model.SortDMA:
		return FmtPercent(p.EnginePercentFor("dma"), 1)
	case model.SortEncDec:
		return FmtPercent(p.EncDecPercent(), 1)
	case model.SortVRAM:
		return FmtBytes(p.VramUsed)
	case model.SortGTT:
		return FmtBytes(p.GttUsed)
	case model.SortTotal:
		v := p.EnginePercent()
		return FmtPercent(&v, 1)
	}
	return NA
}

// RowCells returns the rendered cells of one row in ColumnOrder.
func RowCells(p model.GpuProcess, fullCommand bool) []string {
	out := make([]string, 0, len(ColumnOrder))
	for _, c := range ColumnOrder {
		out = append(out, CellValue(p, c, fullCommand))
	}
	return out
}

// TopMarker marks the row using the most GPU: a glyph rather than a colour
// change alone, because the highlight has to survive --no-color.
const TopMarker = "▶"

// PanelTitle is the one-line summary above the table: how it is sorted,
// what is hidden, how much is hidden, and what the GPU is doing.
func PanelTitle(sort SortState, filters FilterState, shown, total int, top []model.GpuProcess, tree bool) string {
	parts := []string{"sort " + sort.Describe(), filters.Describe()}
	if tree {
		parts = append(parts, "tree")
	}
	if shown != total {
		parts = append(parts, fmt.Sprintf("%d of %d", shown, total))
	}
	if len(top) == 0 {
		if NumericSortColumns[sort.Column] {
			parts = append(parts, "idle")
		}
	} else {
		names := make([]string, 0, 2)
		for _, p := range top {
			if len(names) == 2 {
				break
			}
			names = append(names, fmt.Sprintf("%s %d %s", p.Name, p.PID, CellValue(p, sort.Column, false)))
		}
		parts = append(parts, "top "+strings.Join(names, ", "))
	}
	return strings.Join(parts, "  ·  ")
}

// TableWidth is the total width of the table columns, excluding borders.
func TableWidth() int {
	total := 0
	for _, c := range ColumnOrder {
		total += ColumnWidths[c]
	}
	return total
}
