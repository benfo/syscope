package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// term is one explained metric: what it is, and when it matters.
type term struct{ name, text string }

// about returns the title and terms explaining what's currently on screen.
// On the Performance tab the selected device page provides them, and only for
// metrics this platform reports.
func (m model) about() (string, []term) {
	if m.tab == tabProcesses {
		return "Processes", m.table.terms()
	}
	sel, _ := m.selectedPage(m.visiblePages())
	return sel.helpTitle(), sel.terms()
}

var processTerms = []term{
	{"PID", "Process ID, a number the OS gives each running process. Use it with `taskman kill <pid>`. IDs are reused after a process exits."},
	{"NAME", "The program's executable name. Grouping (z) combines processes with the same name, e.g. \"chrome (31)\", summing their CPU and memory."},
	{"USER", "The account the process runs as. Blank usually means a system account you can't inspect without admin rights."},
	{"CPU%", "Share of total CPU capacity used since the last refresh. Normalized (default): 100% means every core is busy, like Task Manager. " +
		"Per-core (c): 100% means one full core, so a busy process can exceed 100%. \"--\" means no permission to read it (run as admin / sudo)."},
	{"MEM", "Resident memory (the working set on Windows): RAM the process is using right now, including shared libraries. " +
		"It reads higher than Task Manager's \"private\" figure, and the column can add up to more than your RAM because shared pages count in every process."},
	{"Details (Enter)", "A pane for the selected process: path, command line, parent and children, start time, threads, handles and live I/O. " +
		"Hints explain its resource use: ℹ is an explanation, ⚠ is worth acting on."},
	{"Top line", "CPU and MEM as on the Performance tab. DISK is the busiest disk's active time. NET is total download/upload on the adapters shown. " +
		"SWAP is memory paged out to disk; if it keeps growing while MEM is high, you're short of RAM."},
}

// helpContent is the full ? screen: explanations for this view, then keys.
func (m model) helpContent() []string {
	const nameW = 16
	title, terms := m.about()
	textW := max(20, m.width-nameW-4)

	lines := []string{ui.label.Render("About: " + title)}
	for _, t := range terms {
		wrapped := strings.Split(ansi.Wordwrap(t.text, textW, ""), "\n")
		for i, w := range wrapped {
			name := ""
			if i == 0 {
				name = t.name
			}
			lines = append(lines, "  "+ui.key.Render(fit(name, nameW))+w)
		}
	}
	lines = append(lines, "")
	return append(lines, helpLines(m.scope())...)
}

// helpAction handles keys while the ? screen is open. It reuses the global
// movement actions to scroll, so j/k/g/G mean the same thing everywhere.
func (m model) helpAction(act action) model {
	page := max(1, m.height-chromeLines)
	switch act {
	case actDown:
		m.helpOff++
	case actUp:
		m.helpOff--
	case actHalfDown:
		m.helpOff += page / 2
	case actHalfUp:
		m.helpOff -= page / 2
	case actPageDown:
		m.helpOff += page
	case actPageUp:
		m.helpOff -= page
	case actTop:
		m.helpOff = 0
	case actBottom:
		m.helpOff = len(m.helpContent())
	case actTab1, actTab2, actNextTab, actPrevTab:
		// Stay in help and explain the new tab.
		m = m.switchTab(act)
		m.helpOff = 0
	case actThemePicker:
		m.showHelp = false
		m = m.openThemePicker()
	case actCancel, actHelp, actQuit:
		m.showHelp = false
	}
	m.helpOff = max(0, min(m.helpOff, len(m.helpContent())-page))
	return m
}
