package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"taskman/internal/view"
)

// Fixed screen furniture: 3 header lines, table heading, footer.
const chromeLines = 5

const (
	colID   = 7
	colUser = 14
	colCPU  = 7
	colMem  = 11
	colGap  = 1
)

func (m model) bodyHeight() int { return max(1, m.height-chromeLines) }

func (m model) View() string {
	if m.width == 0 {
		return "loading…"
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString(m.renderTable())
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m model) renderHeader() string {
	const barW, textW = 20, 18
	sparkW := max(0, m.width-6-barW-1-textW-1)
	s := m.snap

	cpuText := "--"
	if s.CPUKnown {
		cpuText = fmt.Sprintf("%5.1f%%", s.CPUTotal)
	}
	memPct := 0.0
	if s.MemTotal > 0 {
		memPct = float64(s.MemUsed) / float64(s.MemTotal) * 100
	}
	memText := fmt.Sprintf("%s/%s", shortBytes(s.MemUsed), shortBytes(s.MemTotal))

	cpuLine := labelStyle.Render(" CPU  ") + bar(s.CPUTotal, barW) + " " + fit(cpuText, textW) + " " +
		loadStyle(s.CPUTotal).Render(sparkline(m.cpuHist.values(), sparkW))
	memLine := labelStyle.Render(" MEM  ") + bar(memPct, barW) + " " + fit(memText, textW) + " " +
		loadStyle(memPct).Render(sparkline(m.memHist.values(), sparkW))

	mode := "normalized"
	if m.irix {
		mode = "per-core"
	}
	layout := "flat"
	if m.group {
		layout = "grouped"
	}
	info := fmt.Sprintf(" Procs %d · Cores %d · Up %s · Swap %s/%s · CPU%% %s · %s",
		s.ProcCount, s.NumCPU, view.FormatUptime(s.Uptime),
		shortBytes(s.SwapUsed), shortBytes(s.SwapTotal), mode, layout)

	return ansi.Truncate(cpuLine, m.width, "") + "\n" + ansi.Truncate(memLine, m.width, "") + "\n" + dimStyle.Render(fit(info, m.width)) + "\n"
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

func (m model) renderFooter() string {
	switch {
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

	help := []struct{ k, t string }{
		{"↑↓", "move"}, {"s", "sort"}, {"S", "reverse"}, {"/", "filter"}, {"g", "group"},
		{"i", "irix"}, {"k", "end task"}, {"K", "force kill"}, {"q", "quit"},
	}
	var b strings.Builder
	if f := m.filter.Value(); f != "" {
		b.WriteString(confirmStyle.Render(" filter: "+f) + helpTextStyle.Render(" (esc clears) "))
	}
	for _, h := range help {
		b.WriteString(" " + helpKeyStyle.Render(h.k) + " " + helpTextStyle.Render(h.t))
	}
	return ansi.Truncate(b.String(), m.width, "")
}

// shortBytes is FormatBytes without the space, for the dense header.
func shortBytes(n uint64) string {
	return strings.Replace(view.FormatBytes(n), " ", "", 1)
}
