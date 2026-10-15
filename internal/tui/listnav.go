package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// scroller is the cursor and scroll position of a scrolling list whose rows
// have stable keys. Once the user moves, the cursor follows that row across
// refreshes; until then it stays put, so the top row stays highlighted.
type scroller struct {
	cursor int
	offset int
	selKey string // followed row; "" until the user moves
}

// rowKeys gives a list's length and the key of row i.
type rowKeys struct {
	n   int
	key func(i int) string
}

// refresh re-finds the followed row after the rows changed; if it's gone,
// the cursor stays at the same position and follows the row now there.
func (s *scroller) refresh(rows rowKeys, height int) {
	found := false
	if s.selKey != "" {
		for i := 0; i < rows.n; i++ {
			if rows.key(i) == s.selKey {
				s.cursor, found = i, true
				break
			}
		}
	}
	if !found {
		s.cursor = max(0, min(s.cursor, rows.n-1))
		if s.selKey != "" {
			s.follow(rows)
		}
	}
	s.clamp(rows.n, height)
}

// move moves the cursor by delta, clamped to the list (scrolling lists don't
// wrap; see "Key rules"), and starts following the new row.
func (s *scroller) move(delta int, rows rowKeys, height int) {
	s.cursor = max(0, min(s.cursor+delta, rows.n-1))
	s.follow(rows)
	s.clamp(rows.n, height)
}

func (s *scroller) follow(rows rowKeys) {
	if s.cursor >= 0 && s.cursor < rows.n {
		s.selKey = rows.key(s.cursor)
	}
}

// reset returns to the top and stops following a row.
func (s *scroller) reset() { *s = scroller{} }

// clamp keeps the cursor visible in a window of height rows.
func (s *scroller) clamp(n, height int) {
	h := max(1, height)
	if s.cursor < s.offset {
		s.offset = s.cursor
	}
	if s.cursor >= s.offset+h {
		s.offset = s.cursor - h + 1
	}
	s.offset = max(0, min(s.offset, n-h))
}

// scrollMove is how far a movement action moves the cursor in a scrolling
// list of n rows showing height rows at a time; ok is false for other actions.
func scrollMove(act action, n, height int) (delta int, ok bool) {
	half := max(1, height/2)
	switch act {
	case actDown:
		return 1, true
	case actUp:
		return -1, true
	case actTop:
		return -n, true
	case actBottom:
		return n, true
	case actHalfDown:
		return half, true
	case actHalfUp:
		return -half, true
	case actPageDown:
		return height, true
	case actPageUp:
		return -height, true
	}
	return 0, false
}

// searchBox is a list's "/" live filter. Typing in it works like vim's
// insert mode: keys type, Enter keeps the search, Esc clears it.
type searchBox struct {
	input  textinput.Model
	active bool // keys go to the box
}

func newSearchBox(placeholder string) searchBox {
	ti := textinput.New()
	ti.Prompt = "/"
	ti.Placeholder = placeholder
	return searchBox{input: ti}
}

func (s searchBox) value() string { return s.input.Value() }

func (s *searchBox) start() tea.Cmd {
	s.active = true
	return s.input.Focus()
}

func (s *searchBox) clear() { s.input.SetValue("") }

// key handles a key while the box is active.
func (s *searchBox) key(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		s.active = false
		s.input.Blur()
	case "esc":
		s.active = false
		s.input.Blur()
		s.clear()
	default:
		var cmd tea.Cmd
		s.input, cmd = s.input.Update(msg)
		return cmd
	}
	return nil
}

// column is one column of a table of R rows rendered by a T. Adding a column
// to a table means adding an entry to its column list.
type column[T, R any] struct {
	title   string
	width   int // 0: flexible, takes the remaining width (at least minFlexW)
	right   bool
	sortID  int            // the table's sort key for this column; noSort if none
	visible func(t T) bool // nil: always shown
	cell    func(t T, r R) string
}

const (
	noSort   = -1
	colGap   = 1
	minFlexW = 8
)

// visibleColumns filters columns by their visible rule.
func visibleColumns[T, R any](t T, cols []column[T, R]) []column[T, R] {
	var out []column[T, R]
	for _, c := range cols {
		if c.visible == nil || c.visible(t) {
			out = append(out, c)
		}
	}
	return out
}

// columnWidths resolves flexible columns against the available width w.
func columnWidths[T, R any](cols []column[T, R], w int) []int {
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

// tableLine lays out one row of cells at the given column widths.
func tableLine[T, R any](cols []column[T, R], widths []int, cells []string, w int) string {
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

// tableView is what renderTable needs to draw a table.
type tableView[T, R any] struct {
	owner  T
	cols   []column[T, R]
	rows   []R
	scroll scroller
	height int // visible rows
	sortID int
	desc   bool
}

// renderTable draws the heading (sort column marked ▲/▼) and the visible
// rows, with the cursor row highlighted.
func renderTable[T, R any](v tableView[T, R], w int) string {
	cols := visibleColumns(v.owner, v.cols)
	widths := columnWidths(cols, w)

	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.title
		if c.sortID != noSort && c.sortID == v.sortID {
			titles[i] += arrowFor(v.desc)[1:] // "▲"/"▼" without the leading space
		}
	}
	var b strings.Builder
	b.WriteString(ui.inverse.Render(tableLine(cols, widths, titles, w)) + "\n")

	cells := make([]string, len(cols))
	for i := v.scroll.offset; i < v.scroll.offset+v.height; i++ {
		if i >= len(v.rows) {
			b.WriteString("\n")
			continue
		}
		for j, c := range cols {
			cells[j] = c.cell(v.owner, v.rows[i])
		}
		row := tableLine(cols, widths, cells, w)
		if i == v.scroll.cursor {
			row = ui.selected.Render(row)
		}
		b.WriteString(row + "\n")
	}
	return b.String()
}
