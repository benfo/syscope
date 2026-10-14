package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"taskman/internal/view"
)

const (
	tabProcesses = iota
	tabPerformance
	numTabs
)

var tabNames = [numTabs]string{"Processes", "Performance"}

// Fixed screen furniture on every tab: summary strip, tab bar, footer.
const chromeLines = 3

const (
	colID   = 7
	colUser = 14
	colCPU  = 7
	colMem  = 11
	colGap  = 1
)

// bodyHeight is the number of process rows (the table heading takes a line).
func (m model) bodyHeight() int { return max(1, m.height-chromeLines-1) }

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
		b.WriteString(m.renderTable())
	}
	b.WriteString(m.renderFooter())
	return b.String()
}

// renderStrip is the one-line summary shown on every tab.
func (m model) renderStrip() string {
	s := m.snap
	cpuText := "  --"
	if s.CPUKnown {
		cpuText = fmt.Sprintf("%4.1f%%", s.CPUTotal)
	}
	memPct := view.Percent(s.MemUsed, s.MemTotal)

	// keep is the drop priority when the line is too wide: lowest goes first.
	type item struct {
		text string
		keep int
	}
	items := []item{
		{labelStyle.Render("CPU ") + loadStyle(s.CPUTotal).Render(cpuText+" "+sparkline(m.cpuHist.values(), 8)), 9},
		{labelStyle.Render("MEM ") + loadStyle(memPct).Render(fmt.Sprintf("%.0f%%", memPct)) +
			dimStyle.Render(" "+view.FormatUsedTotal(s.MemUsed, s.MemTotal)), 8},
	}
	if len(s.GPUs) > 0 {
		items = append(items, item{labelStyle.Render("GPU ") + m.stripGPU(), 7})
	}
	items = append(items,
		item{labelStyle.Render("DISK ") + m.stripDisk(), 5},
		item{labelStyle.Render("NET ") + m.stripNet(), 6},
		item{labelStyle.Render("SWAP ") + dimStyle.Render(view.FormatUsedTotal(s.SwapUsed, s.SwapTotal)), 3},
		item{labelStyle.Render("PROCS ") + strconv.Itoa(s.ProcCount), 2},
		item{labelStyle.Render("UP ") + view.FormatUptime(s.Uptime), 1},
	)
	join := func() string {
		parts := make([]string, len(items))
		for i, it := range items {
			parts[i] = it.text
		}
		return " " + strings.Join(parts, "  ")
	}
	for line := join(); lipgloss.Width(line) > m.width && len(items) > 1; line = join() {
		low := 0
		for i, it := range items {
			if it.keep < items[low].keep {
				low = i
			}
		}
		items = append(items[:low], items[low+1:]...)
	}
	return ansi.Truncate(join(), m.width, "")
}

func (m model) renderTabs() string {
	var left strings.Builder
	for i, n := range tabNames {
		t := fmt.Sprintf(" %d %s ", i+1, n)
		if i == m.tab {
			left.WriteString(tabActiveStyle.Render(t))
		} else {
			left.WriteString(tabInactiveStyle.Render(t))
		}
		left.WriteString(" ")
	}

	var right string
	if m.tab == tabProcesses {
		mode, layout := "normalized", "flat"
		if m.irix {
			mode = "per-core"
		}
		if m.group {
			layout = "grouped"
		}
		dir := "▲"
		if m.desc {
			dir = "▼"
		}
		right = dimStyle.Render(fmt.Sprintf("sort %s%s · CPU%% %s · %s ", m.sortKey, dir, mode, layout))
	}
	return spread(left.String(), right, m.width)
}

func (m model) renderTable() string {
	nameW := max(8, m.width-colID-colUser-colCPU-colMem-4*colGap)
	gap := strings.Repeat(" ", colGap)

	arrow := func(k view.SortKey, title string) string {
		if m.sortKey != k {
			return title
		}
		if m.desc {
			return title + "▼"
		}
		return title + "▲"
	}
	idTitle := arrow(view.SortPID, "PID")
	if m.group {
		idTitle = "COUNT"
	}
	head := fitRight(idTitle, colID) + gap + fit(arrow(view.SortName, "NAME"), nameW) + gap +
		fit("USER", colUser) + gap + fitRight(arrow(view.SortCPU, "CPU%"), colCPU) + gap +
		fitRight(arrow(view.SortMem, "MEM"), colMem)

	var b strings.Builder
	b.WriteString(headStyle.Render(fit(head, m.width)) + "\n")

	h := m.bodyHeight()
	for i := m.offset; i < m.offset+h; i++ {
		if i >= len(m.rows) {
			b.WriteString("\n")
			continue
		}
		r := m.rows[i]
		id := strconv.Itoa(int(r.PID))
		if m.group {
			id = strconv.Itoa(r.Count)
		}
		line := fitRight(id, colID) + gap + fit(r.Name, nameW) + gap +
			fit(view.ShortUser(r.User), colUser) + gap +
			fitRight(view.FormatCPU(r.CPU, r.CPUKnown, m.snap.NumCPU, m.irix), colCPU) + gap +
			fitRight(view.FormatMem(r.RSS, r.RSSKnown), colMem)
		line = fit(line, m.width)
		if i == m.cursor {
			line = selStyle.Render(line)
		}
		b.WriteString(line + "\n")
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

func (m model) renderFooter() string {
	switch {
	case m.showHelp:
		return m.helpFooter()
	case m.confirm != nil:
		verb := "End task"
		if m.confirm.force {
			verb = "Force kill"
		}
		return confirmStyle.Render(fit(fmt.Sprintf(" %s %s? [y/N]", verb, rowDesc(m.confirm.row)), m.width))
	case m.filtering:
		return ansi.Truncate(" "+m.filter.View(), m.width, "")
	case m.status != "" && time.Since(m.statusAt) < 5*time.Second:
		st := dimStyle
		if !m.statusOK {
			st = errStyle
		}
		return st.Render(fit(" "+m.status, m.width))
	}

	var b strings.Builder
	if f := m.filter.Value(); f != "" && m.tab == tabProcesses {
		b.WriteString(confirmStyle.Render(" filter: "+f) + helpTextStyle.Render(" (esc clears) "))
	}
	for _, h := range hints(m.scope()) {
		b.WriteString(" " + helpKeyStyle.Render(h.label) + " " + helpTextStyle.Render(h.short))
	}
	return ansi.Truncate(b.String(), m.width, "")
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

// stripDisk shows the busiest disk's active time, or total transfer rate
// where active time isn't available.
func (m model) stripDisk() string {
	busiest, known := 0.0, false
	var rate float64
	for _, d := range m.snap.Disks {
		rate += d.ReadBps + d.WriteBps
		if d.ActiveKnown {
			busiest, known = max(busiest, d.ActivePct), true
		}
	}
	if known {
		return loadStyle(busiest).Render(fmt.Sprintf("%.0f%%", busiest))
	}
	if len(m.snap.Disks) == 0 {
		return dimStyle.Render("--")
	}
	return view.FormatByteRate(rate)
}

// stripNet totals receive/send across the adapters shown by default.
func (m model) stripNet() string {
	var rx, tx float64
	for _, n := range m.snap.Nets {
		if n.Default {
			rx += n.RecvBps
			tx += n.SentBps
		}
	}
	return "↓" + view.FormatBitRate(rx) + " ↑" + view.FormatBitRate(tx)
}

// helpFooter shows scroll keys and position while the ? screen is open.
func (m model) helpFooter() string {
	total, page := len(m.helpContent()), max(1, m.height-chromeLines)
	s := " " + helpKeyStyle.Render("esc") + " " + helpTextStyle.Render("close")
	if total > page {
		s += " " + helpKeyStyle.Render("j/k") + " " + helpTextStyle.Render("scroll") +
			dimStyle.Render(fmt.Sprintf("  lines %d–%d of %d", m.helpOff+1, min(total, m.helpOff+page), total))
	}
	return ansi.Truncate(s, m.width, "")
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
