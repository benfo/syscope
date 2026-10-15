package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// devicePage is one device on the Performance tab. Each kind of device has
// its own page_*.go file implementing this, so adding a kind means adding a
// file and a line to pageSources, never editing the tab's layout code.
type devicePage interface {
	key() string     // stable identity across refreshes; prefixes its history series
	name() string    // sidebar title
	summary() string // sidebar value line
	trend() []float64
	style() lipgloss.Style // colour of its sparkline
	detail(w, h int) []string
	helpTitle() string
	terms() []term // ? explanations for what detail shows
	record()       // append this sample to the device's history series
	hidden() bool  // listed only after "show hidden" (.)
}

// pageSources lists the device kinds in sidebar order.
var pageSources = []func(m model) []devicePage{cpuPages, memoryPages, diskPages, netPages, gpuPages}

// pages returns every device page, optionally including hidden ones.
func (m model) pages(includeHidden bool) []devicePage {
	var out []devicePage
	for _, src := range pageSources {
		for _, p := range src(m) {
			if includeHidden || !p.hidden() {
				out = append(out, p)
			}
		}
	}
	return out
}

// visiblePages are the pages shown in the sidebar right now.
func (m model) visiblePages() []devicePage { return m.pages(m.showHidden) }

// selectedPage returns the selected page and its index, falling back to the
// first page when the selected device has gone.
func (m model) selectedPage(pages []devicePage) (devicePage, int) {
	for i, p := range pages {
		if p.key() == m.perfKey {
			return p, i
		}
	}
	return pages[0], 0
}

func (m model) perfAction(act action) model {
	pages := m.visiblePages()
	_, i := m.selectedPage(pages)
	if act == actShowHidden {
		m.showHidden = !m.showHidden
		return m
	}
	if i, moved := menuMove(i, len(pages), act); moved {
		m.perfKey = pages[i].key()
	}
	return m
}

// recordHistory appends this sample to every device's graphs, hidden ones
// included, so their history is there when they're shown.
func (m model) recordHistory() {
	for _, p := range m.pages(true) {
		p.record()
	}
}

const sideW = 24

func (m model) renderPerf() string {
	h := m.height - chromeLines
	pages := m.visiblePages()
	sel, selIdx := m.selectedPage(pages)

	detailW := max(10, m.width-sideW-2)
	detail := sel.detail(detailW, h)
	side := sidebar(pages, selIdx, h)

	var b strings.Builder
	sep := ui.muted.Render("│")
	for i := 0; i < h; i++ {
		var l, r string
		if i < len(side) {
			l = side[i]
		}
		if i < len(detail) {
			r = detail[i]
		}
		b.WriteString(padANSI(l, sideW) + sep + " " + padANSI(r, detailW) + "\n")
	}
	return b.String()
}

// sidebar renders device entries, dropping spacing and then scrolling when
// they don't fit in h lines.
func sidebar(pages []devicePage, selIdx, h int) []string {
	per := 4 // name, value, sparkline, blank
	if len(pages)*per > h {
		per = 3
	}
	first := 0
	if fit := max(1, h/per); selIdx >= fit {
		first = selIdx - fit + 1
	}
	var out []string
	if first > 0 {
		out = append(out, ui.muted.Render("  ▲ more"))
	}
	for i := first; i < len(pages); i++ {
		out = append(out, sidebarEntry(pages[i], i == selIdx)...)
		if per == 4 {
			out = append(out, "")
		}
	}
	if len(out) > h {
		out = append(out[:h-1], ui.muted.Render("  ▼ more"))
	}
	return out
}

func sidebarEntry(p devicePage, selected bool) []string {
	marker, title := "  ", indent(p)+p.name() // nested pages, e.g. volumes, are indented
	if selected {
		marker = ui.accent.Render("▌ ")
		title = ui.bold.Render(title)
	}
	return []string{
		marker + title,
		marker + indent(p) + ui.muted.Render(p.summary()),
		marker + indent(p) + p.style().Render(sparkline(p.trend(), sideW-3-len(indent(p)))),
	}
}

// histories holds one fixed-size series per graph, keyed by device key and
// series name ("disk:0" + "read").
type histories map[string]*history

func (h histories) push(device, series string, v float64) {
	k := device + ":" + series
	r := h[k]
	if r == nil {
		r = newHistory(historyLen)
		h[k] = r
	}
	r.push(v)
}

func (h histories) values(device, series string) []float64 {
	if r := h[device+":"+series]; r != nil {
		return r.values()
	}
	return nil
}

// graphSpan describes how much time a graph of width w covers.
func (m model) graphSpan(w int) string {
	return (time.Duration(min(w*m.graphStyle().perCell, historyLen)) * m.refresh).Round(time.Second).String()
}

// kv renders one "Label value" stat.
func kv(k, v string) string { return ui.muted.Render(k+" ") + v }

// indent is the extra left margin for nested pages.
func indent(p devicePage) string {
	if n, ok := p.(nested); ok {
		return strings.Repeat("  ", n.depth())
	}
	return ""
}
