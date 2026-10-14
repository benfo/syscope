package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

var graphLevels = []rune(" ▁▂▃▄▅▆▇█")

// areaGraph renders the newest w values (0–100) as a filled area chart h
// rows tall, using eighth-block characters for sub-row resolution.
func areaGraph(vals []float64, w, h int) []string {
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
				rows[h-1-r][pad+i] = graphLevels[fill]
			}
		}
	}
	out := make([]string, h)
	for r := range rows {
		out[r] = string(rows[r])
	}
	return out
}

// meterCell is a usage-grid cell: "label ███░░  42%".
func meterCell(label string, pct float64) string {
	return label + " " + bar(pct, 5) + loadStyle(pct).Render(fmt.Sprintf("%4.0f%%", pct))
}

type graphSpec struct {
	title string // optional line above the graph
	vals  []float64
	style lipgloss.Style
}

// withGraphs stacks head, framed graphs sharing the spare height, and tail.
// Graphs that can't get at least one row are dropped. Output is cut to h.
func withGraphs(head, tail []string, graphs []graphSpec, w, h int) []string {
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
		graph := ui.frame.Render(g.style.Render(strings.Join(areaGraph(g.vals, w-2, max(1, gh)), "\n")))
		lines = append(lines, strings.Split(graph, "\n")...)
	}
	lines = append(lines, tail...)
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
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
