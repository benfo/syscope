package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
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
	layout     view.Layout
	folds      [3]view.Folds // open rows per layout; groups start closed, the tree open
	perCoreCPU bool
	showGPU    bool // the machine has a GPU: show GPU% and VRAM

	search  searchBox
	scroll  scroller
	details detailsPane
}

func newProcessTable(fetch detailsFetcher, now func() time.Time) processTable {
	if now == nil {
		now = time.Now
	}
	t := processTable{sortKey: view.SortCPU, desc: true, search: newSearchBox("name or PID"),
		details: detailsPane{fetch: fetch, now: now}}
	t.folds[view.LayoutTree].Default = true // the tree starts open; groups start closed
	return t
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
	if delta, ok := scrollMove(act, len(t.rows), t.rowsHeight()); ok {
		t.move(delta)
		return t, nil
	}
	switch act {
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
		return t, t.search.start()
	case actDetails:
		t.details.open = !t.details.open
		t.clampScroll()
	case actCancel:
		if t.details.open { // Esc closes the pane first, then clears the search
			t.details.open = false
			t.clampScroll()
		} else if t.search.value() != "" {
			t.search.clear()
			t.rebuild()
		}
	case actGroup:
		t.setLayout(view.LayoutGrouped)
	case actTree:
		t.setLayout(view.LayoutTree)
	case actExpand:
		t.expand()
	case actCollapse:
		t.collapse()
	case actExpandAll, actCollapseAll:
		t.folds[t.layout] = view.Folds{Default: act == actExpandAll}
		t.rebuild()
	case actCPUMode:
		t.perCoreCPU = !t.perCoreCPU
	}
	return t, nil
}

// searchKey handles a key typed into the search box.
func (t processTable) searchKey(msg tea.KeyMsg) (processTable, tea.Cmd) {
	cmd := t.search.key(msg)
	t.rebuild()
	return t, cmd
}

// rebuild derives the visible rows and keeps the cursor on the followed
// process or group (see scroller).
func (t *processTable) rebuild() {
	t.rows = view.Build(t.procs, t.search.value(), t.layout, t.sortKey, t.desc, t.folds[t.layout])
	t.scroll.refresh(t.keys(), t.rowsHeight())
}

func (t *processTable) keys() rowKeys {
	return rowKeys{len(t.rows), func(i int) string { return t.rows[i].Key }}
}

func (t *processTable) move(delta int) { t.scroll.move(delta, t.keys(), t.rowsHeight()) }

// resetSelection returns the cursor to the top and stops following a row.
func (t *processTable) resetSelection() { t.scroll.reset() }

func (t *processTable) clampScroll() { t.scroll.clamp(len(t.rows), t.rowsHeight()) }

func (t processTable) selected() (view.Row, bool) {
	if c := t.scroll.cursor; c >= 0 && c < len(t.rows) {
		return t.rows[c], true
	}
	return view.Row{}, false
}

// modeText summarises sort, CPU% mode and layout for the tab bar.
func (t processTable) modeText() string {
	mode := "normalized"
	if t.perCoreCPU {
		mode = "per-core"
	}
	dir := "▲"
	if t.desc {
		dir = "▼"
	}
	return fmt.Sprintf("sort %s%s · CPU%% %s · %s ", t.sortKey, dir, mode, t.layout)
}

func hasGPU(t processTable) bool { return t.showGPU }

// processColumns are the Processes table's columns; adding a column means
// adding an entry here.
var processColumns = []column[processTable, view.Row]{
	{title: "PID", width: 7, right: true, sortID: int(view.SortPID),
		cell: func(_ processTable, r view.Row) string {
			if r.Count > 1 {
				return "" // a group: its PIDs are in the details pane
			}
			return strconv.Itoa(int(r.PID))
		}},
	{title: "NAME", sortID: int(view.SortName), cell: nameCell},
	{title: "USER", width: 14, sortID: noSort,
		cell: func(_ processTable, r view.Row) string { return view.ShortUser(r.User) }},
	{title: "CPU%", width: 7, right: true, sortID: int(view.SortCPU),
		cell: func(t processTable, r view.Row) string {
			return view.FormatCPU(r.CPU, r.CPUKnown, t.numCPU, t.perCoreCPU)
		}},
	{title: "MEM", width: 11, right: true, sortID: int(view.SortMem),
		cell: func(_ processTable, r view.Row) string { return view.FormatMem(r.RSS, r.RSSKnown) }},
	{title: "GPU%", width: 6, right: true, sortID: int(view.SortGPU), visible: hasGPU,
		cell: func(_ processTable, r view.Row) string { return view.FormatGPU(r.GPU, r.GPUKnown) }},
	{title: "VRAM", width: 10, right: true, sortID: int(view.SortVRAM), visible: hasGPU,
		cell: func(_ processTable, r view.Row) string { return view.FormatMem(r.VRAM, r.VRAMKnown) }},
}

// render draws the heading, the rows and, when open, the details pane at
// width w.
func (t processTable) render(w int) string {
	out := renderTable(tableView[processTable, view.Row]{
		owner: t, cols: processColumns, rows: t.rows, scroll: t.scroll,
		height: t.rowsHeight(), sortID: int(t.sortKey), desc: t.desc,
	}, w)
	if t.details.open {
		out += t.renderDetails(w)
	}
	return out
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

// nameCell indents nested rows and marks expandable ones (▸ closed, ▾ open);
// group rows show their size, e.g. "chrome (31)".
func nameCell(t processTable, r view.Row) string {
	name := r.Name
	if r.Count > 1 {
		name = fmt.Sprintf("%s (%d)", r.Name, r.Count)
	}
	if t.layout == view.LayoutFlat {
		return name
	}
	mark := "  "
	switch {
	case r.Expanded:
		mark = "▾ "
	case r.Expandable:
		mark = "▸ "
	}
	return strings.Repeat("  ", r.Depth) + mark + name
}

// setLayout switches to l, or back to flat if l is already active.
func (t *processTable) setLayout(l view.Layout) {
	if t.layout == l {
		l = view.LayoutFlat
	}
	t.layout = l
	t.resetSelection()
	t.rebuild()
}

// expand opens the selected row; on an open row it moves to the first child.
func (t *processTable) expand() {
	r, ok := t.selected()
	switch {
	case !ok || !r.Expandable:
	case r.Expanded:
		t.move(1)
	default:
		t.setOpen(r.Key, true)
	}
}

// collapse closes the selected row; on a closed or leaf row it moves to the
// row it's nested under.
func (t *processTable) collapse() {
	r, ok := t.selected()
	switch {
	case !ok:
	case r.Expanded:
		t.setOpen(r.Key, false)
	case r.Parent != "":
		for i, p := range t.rows {
			if p.Key == r.Parent {
				t.move(i - t.scroll.cursor)
				break
			}
		}
	}
}

func (t *processTable) setOpen(key string, open bool) {
	f := &t.folds[t.layout]
	if f.Open == nil {
		f.Open = map[string]bool{}
	}
	f.Open[key] = open
	t.rebuild()
}
