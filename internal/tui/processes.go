package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/proc"
	"taskman/internal/view"
)

// processTable is the Processes tab: the filtered, sorted, optionally
// grouped process list with its cursor, search box and kill confirmation.
type processTable struct {
	procs  []metrics.ProcInfo
	numCPU int
	rows   []view.Row
	height int // visible rows

	sortKey    view.SortKey
	desc       bool
	group      bool
	perCoreCPU bool

	search    textinput.Model
	searching bool

	cursor int
	offset int
	selKey string // row the cursor follows across refreshes; "" until the user moves

	confirm *pendingKill
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

func newProcessTable(perCoreCPU bool) processTable {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = "name or PID"
	return processTable{sortKey: view.SortCPU, desc: true, perCoreCPU: perCoreCPU, search: ti}
}

func (t *processTable) setProcs(procs []metrics.ProcInfo, numCPU int) {
	t.procs, t.numCPU = procs, numCPU
	t.rebuild()
}

func (t *processTable) setHeight(h int) {
	t.height = max(1, h)
	t.clampScroll()
}

// action applies a keymap action for the Processes scope.
func (t processTable) action(act action) (processTable, tea.Cmd) {
	half := max(1, t.height/2)
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
		t.move(t.height)
	case actPageUp:
		t.move(-t.height)
	case actSortNext:
		t.sortKey = t.sortKey.Next()
		t.desc = t.sortKey.DefaultDesc()
		t.rebuild()
	case actSortReverse:
		t.desc = !t.desc
		t.rebuild()
	case actSearch:
		t.searching = true
		return t, t.search.Focus()
	case actCancel:
		if t.search.Value() != "" {
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
	h := max(1, t.height)
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

const (
	colID   = 7
	colUser = 14
	colCPU  = 7
	colMem  = 11
	colGap  = 1
)

// render draws the heading and t.height rows at width w.
func (t processTable) render(w int) string {
	nameW := max(8, w-colID-colUser-colCPU-colMem-4*colGap)
	gap := strings.Repeat(" ", colGap)

	arrow := func(k view.SortKey, title string) string {
		switch {
		case t.sortKey != k:
			return title
		case t.desc:
			return title + "▼"
		}
		return title + "▲"
	}
	idTitle := arrow(view.SortPID, "PID")
	if t.group {
		idTitle = "COUNT"
	}
	head := fitRight(idTitle, colID) + gap + fit(arrow(view.SortName, "NAME"), nameW) + gap +
		fit("USER", colUser) + gap + fitRight(arrow(view.SortCPU, "CPU%"), colCPU) + gap +
		fitRight(arrow(view.SortMem, "MEM"), colMem)

	var b strings.Builder
	b.WriteString(ui.inverse.Render(fit(head, w)) + "\n")
	for i := t.offset; i < t.offset+t.height; i++ {
		if i >= len(t.rows) {
			b.WriteString("\n")
			continue
		}
		r := t.rows[i]
		id := strconv.Itoa(int(r.PID))
		if t.group {
			id = strconv.Itoa(r.Count)
		}
		line := fit(fitRight(id, colID)+gap+fit(r.Name, nameW)+gap+
			fit(view.ShortUser(r.User), colUser)+gap+
			fitRight(view.FormatCPU(r.CPU, r.CPUKnown, t.numCPU, t.perCoreCPU), colCPU)+gap+
			fitRight(view.FormatMem(r.RSS, r.RSSKnown), colMem), w)
		if i == t.cursor {
			line = ui.selected.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
