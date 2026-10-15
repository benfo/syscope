package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// history is a fixed-size ring of samples for sparklines and graphs.
type history struct {
	vals []float64
	next int
	full bool
}

func newHistory(size int) *history { return &history{vals: make([]float64, size)} }

func (r *history) push(v float64) {
	r.vals[r.next] = v
	r.next = (r.next + 1) % len(r.vals)
	if r.next == 0 {
		r.full = true
	}
}

// values returns the history oldest first.
func (r *history) values() []float64 {
	if !r.full {
		return append([]float64(nil), r.vals[:r.next]...)
	}
	return append(append([]float64(nil), r.vals[r.next:]...), r.vals[:r.next]...)
}

var sparkLevels = []rune("▁▂▃▄▅▆▇█")

// sparkline renders the newest width values (0–100), right-aligned.
func sparkline(vals []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(vals) > width {
		vals = vals[len(vals)-width:]
	}
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", width-len(vals)))
	for _, v := range vals {
		i := int(v / 100 * float64(len(sparkLevels)-1))
		i = max(0, min(i, len(sparkLevels)-1))
		b.WriteRune(sparkLevels[i])
	}
	return b.String()
}

// bar renders a usage meter coloured by load.
func bar(pct float64, width int) string {
	filled := int(pct/100*float64(width) + 0.5)
	filled = max(0, min(filled, width))
	return loadStyle(pct).Render(strings.Repeat("█", filled)) +
		ui.muted.Render(strings.Repeat("░", width-filled))
}

func loadStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 85:
		return ui.bad
	case pct >= 60:
		return ui.warn
	}
	return ui.good
}

// fit pads or truncates s to exactly w display cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.FillRight(runewidth.Truncate(s, w, "…"), w)
}

// fitRight right-aligns s in w display cells.
func fitRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.FillLeft(runewidth.Truncate(s, w, "…"), w)
}

// meterCell is a usage-grid cell: "label ███░░  42%".
func meterCell(label string, pct float64) string {
	return label + " " + bar(pct, 5) + loadStyle(pct).Render(fmt.Sprintf("%4.0f%%", pct))
}

// scaled converts series to 0–100 against their shared maximum (at least
// minTop) and returns that maximum.
func scaled(minTop float64, series ...[]float64) ([]float64, float64) {
	top := minTop
	for _, s := range series {
		for _, v := range s {
			top = max(top, v)
		}
	}
	if len(series) == 0 {
		return nil, top
	}
	out := make([]float64, len(series[0]))
	for i, v := range series[0] {
		out[i] = v / top * 100
	}
	return out, top
}

// sumSeries adds two histories element-wise, aligned at the newest value.
func sumSeries(a, b []float64) []float64 {
	if len(b) > len(a) {
		a, b = b, a
	}
	out := append([]float64(nil), a...)
	off := len(a) - len(b)
	for i, v := range b {
		out[off+i] += v
	}
	return out
}

// spread places left and right on one line of width w, dropping right if
// there is no room.
func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if right == "" || gap < 1 {
		return ansi.Truncate(left, w, "")
	}
	return left + strings.Repeat(" ", gap) + right
}

// padANSI truncates or pads a styled string to exactly w cells.
func padANSI(s string, w int) string {
	s = ansi.Truncate(s, w, "")
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

// Separators for packLine: between "Label value" stats, and between grid cells.
const (
	statSep = "   "
	gridSep = " "
)

// packLine lays items out left to right separated by sep, wrapping onto
// more lines rather than overflowing width w.
func packLine(items []string, sep string, w int) []string {
	var lines []string
	cur := ""
	for _, it := range items {
		switch {
		case cur == "":
			cur = it
		case lipgloss.Width(cur)+len(sep)+lipgloss.Width(it) <= w:
			cur += sep + it
		default:
			lines = append(lines, cur)
			cur = it
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}
