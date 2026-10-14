package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// ring is a fixed-size history of percentages for sparklines.
type ring struct {
	vals []float64
	next int
	full bool
}

func newRing(size int) *ring { return &ring{vals: make([]float64, size)} }

func (r *ring) push(v float64) {
	r.vals[r.next] = v
	r.next = (r.next + 1) % len(r.vals)
	if r.next == 0 {
		r.full = true
	}
}

// values returns the history oldest first.
func (r *ring) values() []float64 {
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
		dimStyle.Render(strings.Repeat("░", width-filled))
}

func loadStyle(pct float64) lipgloss.Style {
	switch {
	case pct >= 85:
		return hotStyle
	case pct >= 60:
		return warmStyle
	}
	return coolStyle
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

var (
	labelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	coolStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	warmStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	hotStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	headStyle     = lipgloss.NewStyle().Bold(true).Reverse(true)
	selStyle      = lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("15"))
	confirmStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	helpKeyStyle  = lipgloss.NewStyle().Bold(true)
	helpTextStyle = dimStyle
)

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

var (
	tabActiveStyle   = lipgloss.NewStyle().Bold(true).Reverse(true)
	tabInactiveStyle = dimStyle
	selMarkerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	cpuGraphStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	memGraphStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	cachedStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	pCoreStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5"))
	eCoreStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

var (
	diskGraphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	readStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	writeStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	netGraphStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	rxStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	txStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
)

var gpuGraphStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
