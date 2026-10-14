package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/proc"
	"taskman/internal/view"
)

// processTable is the Processes tab: the filtered, sorted, optionally
// grouped process list with its cursor, search box and kill confirmation.
type processTable struct {
	procs    []metrics.ProcInfo
	numCPU   int
	memTotal uint64
	rows     []view.Row
	bodyH    int // lines below the heading: rows, plus the details pane when open

	sortKey    view.SortKey
	desc       bool
	group      bool
	perCoreCPU bool
	showGPU    bool // the machine has a GPU: show GPU% and VRAM

	search    textinput.Model
	searching bool

	cursor int
	offset int
	selKey string // row the cursor follows across refreshes; "" until the user moves

	confirm *pendingKill
	details detailsPane
}

// pendingKill is a kill awaiting y/N confirmation.
type pendingKill struct {
	row   view.Row
	force bool
}

// killDoneMsg reports a finished kill back to the model.
type killDoneMsg struct {
	desc   string
	force  bool
	failed int
	total  int
	err    error // last error
}

func newProcessTable(fetch detailsFetcher, now func() time.Time) processTable {
	if now == nil {
		now = time.Now
	}
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "name or PID"
	return processTable{sortKey: view.SortCPU, desc: true, search: ti, details: detailsPane{fetch: fetch, now: now}}
}

// setProcs takes a new sample; system figures from snap give the table
// context (core count for CPU%, whether to show GPU columns, RAM for hints).
func (t *processTable) setProcs(procs []metrics.ProcInfo, snap metrics.SystemSnapshot) {
	t.procs, t.numCPU, t.memTotal, t.showGPU = procs, snap.NumCPU, snap.MemTotal, len(snap.GPUs) > 0
	t.rebuild()
}

func (t *processTable) setHeight(h int) {
	t.bodyH = max(1, h)
	t.clampScroll()
}

// action applies a keymap action for the Processes scope, then fetches
// details when the pane is open and now shows a different process.
func (t processTable) action(act action) (processTable, tea.Cmd) {
	before, _ := t.selected()
	wasOpen := t.details.open
	t, cmd := t.apply(act)
	if after, _ := t.selected(); t.details.open && (!wasOpen || after.Key != before.Key) {
		return t, tea.Batch(cmd, t.detailsCmd())
	}
	return t, cmd
}

func (t processTable) apply(act action) (processTable, tea.Cmd) {
	half := max(1, t.rowsHeight()/2)
	switch act {
	case actDown:
		t.move(1)
	case actUp:
		t.move(-1)
	case actTop:
		t.move(-len(t.rows))
	case actBottom:
		t.move(len(t.rows))
	case actHalfDown:
		t.move(half)
	case actHalfUp:
		t.move(-half)
	case actPageDown:
		t.move(t.rowsHeight())
	case actPageUp:
		t.move(-t.rowsHeight())
	case actSortNext:
		t.sortKey = t.sortKey.Next()
		for t.sortKey.IsGPU() && !t.showGPU { // skip columns this machine doesn't show
			t.sortKey = t.sortKey.Next()
		}
		t.desc = t.sortKey.DefaultDesc()
		t.rebuild()
	case actSortReverse:
		t.desc = !t.desc
		t.rebuild()
	case actSearch:
		t.searching = true
		return t, t.search.Focus()
	case actDetails:
		t.details.open = !t.details.open
		t.clampScroll()
	case actCancel:
		if t.details.open { // Esc closes the pane first, then clears the search
			t.details.open = false
			t.clampScroll()
		} else if t.search.Value() != "" {
			t.search.SetValue("")
			t.rebuild()
		}
	case actGroup:
		t.group = !t.group
		t.resetSelection()
		t.rebuild()
	case actCPUMode:
		t.perCoreCPU = !t.perCoreCPU
	case actEndTask, actForceKill:
		if r, ok := t.selected(); ok {
			t.confirm = &pendingKill{row: r, force: act == actForceKill}
		}
	}
	return t, nil
}

// searchKey handles typing in the search box, like vim's insert mode: keys
// type, Enter keeps the search, Esc clears it.
func (t processTable) searchKey(msg tea.KeyMsg) (processTable, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "enter":
		t.searching = false
		t.search.Blur()
	case "esc":
		t.searching = false
		t.search.Blur()
		t.search.SetValue("")
	default:
		t.search, cmd = t.search.Update(msg)
	}
	t.rebuild()
	return t, cmd
}

// confirmKey answers the y/N kill prompt; any key but y cancels.
func (t processTable) confirmKey(key string) (processTable, tea.Cmd, bool) {
	pk := *t.confirm
	t.confirm = nil
	if key == "y" || key == "Y" {
		return t, killCmd(pk), false
	}
	return t, nil, true
}

func killCmd(pk pendingKill) tea.Cmd {
	return func() tea.Msg {
		res := killDoneMsg{desc: rowDesc(pk.row), force: pk.force, total: len(pk.row.PIDs)}
		for _, pid := range pk.row.PIDs {
			if err := proc.Kill(pid, pk.force); err != nil {
				res.failed++
				res.err = err
			}
		}
		return res
	}
}

func rowDesc(r view.Row) string {
	if r.Count > 1 {
		return fmt.Sprintf("%s (%d processes)", r.Name, r.Count)
	}
	return fmt.Sprintf("%s (PID %d)", r.Name, r.PID)
}

// rebuild derives the visible rows. Once the user has moved the cursor, the
// selection follows that process or group across refreshes; until then the
// cursor stays put so the top consumer stays highlighted.
func (t *processTable) rebuild() {
	t.rows = view.Rows(view.Filter(t.procs, t.search.Value()), t.group)
	view.Sort(t.rows, t.sortKey, t.desc)

	found := false
	if t.selKey != "" {
		for i, r := range t.rows {
			if r.Key == t.selKey {
				t.cursor, found = i, true
				break
			}
		}
	}
	if !found {
		t.cursor = max(0, min(t.cursor, len(t.rows)-1))
		if t.selKey != "" {
			t.syncSelection() // selected process exited: follow its replacement row
		}
	}
	t.clampScroll()
}

func (t *processTable) move(delta int) {
	t.cursor = max(0, min(t.cursor+delta, len(t.rows)-1))
	t.syncSelection()
	t.clampScroll()
}

// resetSelection returns the cursor to the top and stops following a row.
func (t *processTable) resetSelection() {
	t.cursor, t.offset, t.selKey = 0, 0, ""
}

func (t *processTable) syncSelection() {
	if r, ok := t.selected(); ok {
		t.selKey = r.Key
	}
}

func (t *processTable) clampScroll() {
	h := t.rowsHeight()
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
	t.offset = max(0, min(t.offset, len(t.rows)-h))
}

func (t processTable) selected() (view.Row, bool) {
	if t.cursor < 0 || t.cursor >= len(t.rows) {
		return view.Row{}, false
	}
	return t.rows[t.cursor], true
}

// modeText summarises sort, CPU% mode and grouping for the tab bar.
func (t processTable) modeText() string {
	mode, layout := "normalized", "flat"
	if t.perCoreCPU {
		mode = "per-core"
	}
	if t.group {
		layout = "grouped"
	}
	dir := "▲"
	if t.desc {
		dir = "▼"
	}
	return fmt.Sprintf("sort %s%s · CPU%% %s · %s ", t.sortKey, dir, mode, layout)
}

// column is one Processes table column; adding a column means adding an
// entry to processColumns.
type column struct {
	title    string
	width    int // 0: flexible, takes the remaining width (at least minFlexW)
	right    bool
	sortKey  view.SortKey
	sortable bool
	gpuOnly  bool                        // shown only on machines with a GPU
	header   func(t processTable) string // overrides title when set
	cell     func(t processTable, r view.Row) string
}

const (
	colGap   = 1
	minFlexW = 8
)

var processColumns = []column{
	{title: "PID", width: 7, right: true, sortKey: view.SortPID, sortable: true,
		header: func(t processTable) string {
			if t.group {
				return "COUNT"
			}
			return t.arrow(view.SortPID, "PID")
		},
		cell: func(t processTable, r view.Row) string {
			if t.group {
				return strconv.Itoa(r.Count)
			}
			return strconv.Itoa(int(r.PID))
		}},
	{title: "NAME", sortKey: view.SortName, sortable: true,
		cell: func(_ processTable, r view.Row) string { return r.Name }},
	{title: "USER", width: 14,
		cell: func(_ processTable, r view.Row) string { return view.ShortUser(r.User) }},
	{title: "CPU%", width: 7, right: true, sortKey: view.SortCPU, sortable: true,
		cell: func(t processTable, r view.Row) string {
			return view.FormatCPU(r.CPU, r.CPUKnown, t.numCPU, t.perCoreCPU)
		}},
	{title: "MEM", width: 11, right: true, sortKey: view.SortMem, sortable: true,
		cell: func(_ processTable, r view.Row) string { return view.FormatMem(r.RSS, r.RSSKnown) }},
	{title: "GPU%", width: 6, right: true, sortKey: view.SortGPU, sortable: true, gpuOnly: true,
		cell: func(_ processTable, r view.Row) string { return view.FormatGPU(r.GPU, r.GPUKnown) }},
	{title: "VRAM", width: 10, right: true, sortKey: view.SortVRAM, sortable: true, gpuOnly: true,
		cell: func(_ processTable, r view.Row) string { return view.FormatMem(r.VRAM, r.VRAMKnown) }},
}

// columns are the columns shown on this machine.
func (t processTable) columns() []column {
	var out []column
	for _, c := range processColumns {
		if !c.gpuOnly || t.showGPU {
			out = append(out, c)
		}
	}
	return out
}

// arrow marks the sort column's title with the sort direction.
func (t processTable) arrow(k view.SortKey, title string) string {
	switch {
	case t.sortKey != k:
		return title
	case t.desc:
		return title + "▼"
	}
	return title + "▲"
}

// columnWidths resolves flexible columns against the available width w.
func columnWidths(cols []column, w int) []int {
	fixed, flex := colGap*(len(cols)-1), 0
	for _, c := range cols {
		if c.width == 0 {
			flex++
		}
		fixed += c.width
	}
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = c.width
		if c.width == 0 {
			widths[i] = max(minFlexW, (w-fixed)/max(1, flex))
		}
	}
	return widths
}

// line lays out one row of cells at the given column widths.
func line(cols []column, widths []int, cells []string, w int) string {
	gap := strings.Repeat(" ", colGap)
	parts := make([]string, len(cols))
	for i, c := range cols {
		if c.right {
			parts[i] = fitRight(cells[i], widths[i])
		} else {
			parts[i] = fit(cells[i], widths[i])
		}
	}
	return fit(strings.Join(parts, gap), w)
}

// render draws the heading, the rows and, when open, the details pane at
// width w.
func (t processTable) render(w int) string {
	cols := t.columns()
	widths := columnWidths(cols, w)

	titles := make([]string, len(cols))
	for i, c := range cols {
		switch {
		case c.header != nil:
			titles[i] = c.header(t)
		case c.sortable:
			titles[i] = t.arrow(c.sortKey, c.title)
		default:
			titles[i] = c.title
		}
	}
	var b strings.Builder
	b.WriteString(ui.inverse.Render(line(cols, widths, titles, w)) + "\n")

	cells := make([]string, len(cols))
	for i := t.offset; i < t.offset+t.rowsHeight(); i++ {
		if i >= len(t.rows) {
			b.WriteString("\n")
			continue
		}
		for j, c := range cols {
			cells[j] = c.cell(t, t.rows[i])
		}
		row := line(cols, widths, cells, w)
		if i == t.cursor {
			row = ui.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	if t.details.open {
		b.WriteString(t.renderDetails(w))
	}
	return b.String()
}

// terms are the ? explanations for the Processes tab.
func (t processTable) terms() []term {
	if !t.showGPU {
		return processTerms
	}
	return append(append([]term{}, processTerms...),
		term{"GPU%", "How busy the GPU is for this process: its busiest GPU engine, as in Task Manager. Watching video counts (video decode). " +
			"A local LLM showing high CPU% but ~0 GPU% has fallen back to the CPU, usually because the model didn't fit in VRAM."},
		term{"VRAM", "Dedicated GPU memory this process holds. This is what fills your GPU: sort by it (s) to see who's using it. " +
			"Shared memory can be counted in more than one process, so the column can add up to more than the card has."},
	)
}
