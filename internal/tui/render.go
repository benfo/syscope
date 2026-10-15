package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	tabProcesses = iota
	tabPerformance
	tabPorts
	numTabs
)

var tabNames = [numTabs]string{"Processes", "Performance", "Ports"}

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
	case m.tab == tabPorts:
		b.WriteString(m.ports.render(m.width))
	default:
		b.WriteString(m.table.render(m.width))
	}
	b.WriteString(m.renderFooter())
	screen := b.String()
	switch {
	case m.picker != nil: // the theme list replaces Settings while open
		screen = overlay(screen, m.picker.box(), m.width, m.height)
	case m.settingsUI != nil:
		screen = overlay(screen, m.settingsUI.box(m), m.width, m.height)
	}
	return screen
}

// renderTabs is the tab bar, with the tab's mode on the right. When both
// don't fit, inactive tabs shrink to their number.
func (m model) renderTabs() string {
	var right string
	switch m.tab {
	case tabProcesses:
		right = ui.muted.Render(m.table.modeText())
	case tabPorts:
		right = ui.muted.Render(m.ports.modeText())
	}
	left := m.tabLabels(false)
	if lipgloss.Width(left)+lipgloss.Width(right) >= m.width {
		left = m.tabLabels(true)
	}
	return spread(left, right, m.width)
}

func (m model) tabLabels(compact bool) string {
	var b strings.Builder
	for i, n := range tabNames {
		switch {
		case i == m.tab:
			b.WriteString(ui.inverse.Render(fmt.Sprintf(" %d %s ", i+1, n)))
		case compact:
			b.WriteString(ui.muted.Render(fmt.Sprintf(" %d ", i+1)))
		default:
			b.WriteString(ui.muted.Render(fmt.Sprintf(" %d %s ", i+1, n)))
		}
		b.WriteString(" ")
	}
	return b.String()
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

// activeSearch is the current tab's search box; nil on tabs without one.
func (m model) activeSearch() *searchBox {
	switch m.tab {
	case tabProcesses:
		return &m.table.search
	case tabPorts:
		return &m.ports.search
	}
	return nil
}

func (m model) renderFooter() string {
	search := m.activeSearch()
	switch {
	case m.showHelp:
		return m.helpFooter()
	case m.confirm != nil:
		return ui.prompt.Render(fit(" "+m.confirm.text(), m.width))
	case search != nil && search.active:
		return ansi.Truncate(" "+search.input.View(), m.width, "")
	case m.status != "" && time.Since(m.statusAt) < 5*time.Second:
		style := ui.muted
		if !m.statusOK {
			style = ui.bad
		}
		return style.Render(fit(" "+m.status, m.width))
	}

	var b strings.Builder
	if search != nil && search.value() != "" {
		s := search.value()
		b.WriteString(ui.prompt.Render(" filter: "+s) + ui.muted.Render(" (esc clears) "))
	}
	// "? help" lists every key, so it always shows; other hints are dropped
	// from the end when the line is too narrow.
	hint := func(h binding) string { return " " + ui.key.Render(h.label) + " " + ui.muted.Render(h.short) }
	var help string
	var others []string
	for _, h := range hints(m.scope()) {
		if _, isHelp := h.keys["?"]; isHelp {
			help = hint(h)
		} else {
			others = append(others, hint(h))
		}
	}
	for _, h := range others {
		if lipgloss.Width(b.String()+h+help) > m.width {
			break
		}
		b.WriteString(h)
	}
	return ansi.Truncate(b.String()+help, m.width, "")
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
