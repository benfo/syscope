package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	tabProcesses = iota
	tabPerformance
	numTabs
)

var tabNames = [numTabs]string{"Processes", "Performance"}

// Fixed screen furniture on every tab: summary strip, tab bar, footer.
const chromeLines = 3

// tableHeight is the number of process rows (the table heading takes a line).
func (m model) tableHeight() int { return max(1, m.height-chromeLines-1) }

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.renderStrip() + "\n")
	b.WriteString(m.renderTabs() + "\n")
	switch {
	case m.showHelp:
		b.WriteString(m.renderHelp())
	case m.tab == tabPerformance:
		b.WriteString(m.renderPerf())
	default:
		b.WriteString(m.table.render(m.width))
	}
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m model) renderTabs() string {
	var left strings.Builder
	for i, n := range tabNames {
		t := fmt.Sprintf(" %d %s ", i+1, n)
		if i == m.tab {
			left.WriteString(ui.inverse.Render(t))
		} else {
			left.WriteString(ui.muted.Render(t))
		}
		left.WriteString(" ")
	}
	var right string
	if m.tab == tabProcesses {
		right = ui.muted.Render(m.table.modeText())
	}
	return spread(left.String(), right, m.width)
}

// renderHelp is the ? overlay, filling the body area.
func (m model) renderHelp() string {
	lines := m.helpContent()
	var b strings.Builder
	for i := 0; i < m.height-chromeLines; i++ {
		var l string
		if j := m.helpOff + i; j < len(lines) {
			l = " " + lines[j]
		}
		b.WriteString(padANSI(l, m.width) + "\n")
	}
	return b.String()
}

func (m model) renderFooter() string {
	t := m.table
	switch {
	case m.showHelp:
		return m.helpFooter()
	case t.confirm != nil:
		verb := "End task"
		if t.confirm.force {
			verb = "Force kill"
		}
		return ui.prompt.Render(fit(fmt.Sprintf(" %s %s? [y/N]", verb, rowDesc(t.confirm.row)), m.width))
	case t.searching:
		return ansi.Truncate(" "+t.search.View(), m.width, "")
	case m.status != "" && time.Since(m.statusAt) < 5*time.Second:
		style := ui.muted
		if !m.statusOK {
			style = ui.bad
		}
		return style.Render(fit(" "+m.status, m.width))
	}

	var b strings.Builder
	if s := t.search.Value(); s != "" && m.tab == tabProcesses {
		b.WriteString(ui.prompt.Render(" filter: "+s) + ui.muted.Render(" (esc clears) "))
	}
	for _, h := range hints(m.scope()) {
		b.WriteString(" " + ui.key.Render(h.label) + " " + ui.muted.Render(h.short))
	}
	return ansi.Truncate(b.String(), m.width, "")
}

// helpFooter shows scroll keys and position while the ? screen is open.
func (m model) helpFooter() string {
	total, page := len(m.helpContent()), max(1, m.height-chromeLines)
	s := " " + ui.key.Render("esc") + " " + ui.muted.Render("close")
	if total > page {
		s += " " + ui.key.Render("j/k") + " " + ui.muted.Render("scroll") +
			ui.muted.Render(fmt.Sprintf("  lines %d–%d of %d", m.helpOff+1, min(total, m.helpOff+page), total))
	}
	return ansi.Truncate(s, m.width, "")
}
