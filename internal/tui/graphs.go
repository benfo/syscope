package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// graphStyle is one way of drawing history graphs. Adding a style means
// adding an entry to graphStyles; the Graphs setting cycles through them.
type graphStyle struct {
	name    string // as saved in preferences and shown in Settings
	perCell int    // samples per character column
	draw    func(vals []float64, w, h int) []string
}

// graphStyles lists the styles, default first.
var graphStyles = []graphStyle{
	{name: "braille", perCell: 2, draw: brailleGraph},
	{name: "blocks", perCell: 1, draw: blockGraph},
}

// graphStyleIndex finds a style by name.
func graphStyleIndex(name string) (int, bool) {
	for i, s := range graphStyles {
		if strings.EqualFold(s.name, name) {
			return i, true
		}
	}
	return 0, false
}

// graphStyle is the style chosen in Settings (the default when unknown).
func (m model) graphStyle() graphStyle {
	i, _ := graphStyleIndex(m.prefs.Graphs)
	return graphStyles[i]
}

var blockLevels = []rune(" ▁▂▃▄▅▆▇█")

// blockGraph renders the newest w values (0–100) as a filled area chart h
// rows tall, one sample per column, using eighth-block characters for
// sub-row resolution.
func blockGraph(vals []float64, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	if len(vals) > w {
		vals = vals[len(vals)-w:]
	}
	rows := make([][]rune, h)
	for r := range rows {
		rows[r] = []rune(strings.Repeat(" ", w))
	}
	pad := w - len(vals)
	for i, v := range vals {
		eighths := int(max(0, min(v, 100))/100*float64(h*8) + 0.5)
		for r := 0; r < h; r++ { // r = 0 is the bottom row
			fill := eighths - r*8
			switch {
			case fill >= 8:
				rows[h-1-r][pad+i] = '█'
			case fill > 0:
				rows[h-1-r][pad+i] = blockLevels[fill]
			}
		}
	}
	out := make([]string, h)
	for r := range rows {
		out[r] = string(rows[r])
	}
	return out
}

// brailleDots[side][row] is the dot of a Braille cell (U+2800 + dots) in
// column side (0 left, 1 right) and dot row (0 top … 3 bottom).
var brailleDots = [2][4]rune{{0x01, 0x02, 0x04, 0x40}, {0x08, 0x10, 0x20, 0x80}}

// brailleGraph renders the newest 2w values (0–100) as a filled area chart
// h rows tall. Each character is a 2×4 grid of Braille dots, so it shows
// twice the samples of blockGraph at half its vertical steps per row.
// Empty cells are spaces rather than blank Braille, so they copy cleanly.
func brailleGraph(vals []float64, w, h int) []string {
	if w <= 0 || h <= 0 {
		return nil
	}
	cols := 2 * w
	if len(vals) > cols {
		vals = vals[len(vals)-cols:]
	}
	cells := make([][]rune, h) // cells[0] is the top row
	for r := range cells {
		cells[r] = make([]rune, w)
	}
	pad := cols - len(vals)
	for i, v := range vals {
		col := pad + i
		dots := int(max(0, min(v, 100))/100*float64(h*4) + 0.5)
		for d := 0; d < dots; d++ { // d = 0 is the bottom dot
			cells[h-1-d/4][col/2] |= brailleDots[col%2][3-d%4]
		}
	}
	out := make([]string, h)
	for r, row := range cells {
		var b strings.Builder
		for _, dots := range row {
			if dots == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(0x2800 + dots)
			}
		}
		out[r] = b.String()
	}
	return out
}

type graphSpec struct {
	title string // optional line above the graph
	vals  []float64
	style lipgloss.Style
}

// withGraphs stacks head, framed graphs sharing the spare height, and tail.
// Graphs that can't get at least one row are dropped. Output is cut to h.
func (m model) withGraphs(head, tail []string, graphs []graphSpec, w, h int) []string {
	draw := m.graphStyle().draw
	avail := h - len(head) - len(tail)
	for len(graphs) > 1 && avail/len(graphs) < 4 {
		graphs = graphs[:len(graphs)-1]
	}
	lines := append([]string{}, head...)
	for i, g := range graphs {
		share := avail / len(graphs)
		if i == len(graphs)-1 {
			share = avail - share*(len(graphs)-1)
		}
		gh := share - 2 // frame
		if g.title != "" {
			lines = append(lines, ui.muted.Render(g.title))
			gh--
		}
		graph := ui.frame.Render(g.style.Render(strings.Join(draw(g.vals, w-2, max(1, gh)), "\n")))
		lines = append(lines, strings.Split(graph, "\n")...)
	}
	lines = append(lines, tail...)
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}
