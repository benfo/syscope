package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

type perfDevice int

const (
	devCPU perfDevice = iota
	devMemory
)

// perfDevices lists the entries in the Performance sidebar.
func (m model) perfDevices() []perfDevice { return []perfDevice{devCPU, devMemory} }

const sideW = 24

func (m model) handlePerfKey(key string) model {
	switch key {
	case "up":
		m.perfSel = max(0, m.perfSel-1)
	case "down":
		m.perfSel = min(len(m.perfDevices())-1, m.perfSel+1)
	}
	return m
}

func (m model) renderPerf() string {
	h := m.height - chromeLines
	devs := m.perfDevices()
	sel := devs[min(m.perfSel, len(devs)-1)]

	var side []string
	for _, d := range devs {
		side = append(side, m.sidebarEntry(d, d == sel)...)
		side = append(side, "")
	}

	detailW := max(10, m.width-sideW-2)
	var detail []string
	switch sel {
	case devCPU:
		detail = m.cpuDetail(detailW, h)
	case devMemory:
		detail = m.memDetail(detailW, h)
	}

	var b strings.Builder
	sep := dimStyle.Render("│")
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

func (m model) sidebarEntry(d perfDevice, selected bool) []string {
	var name, value string
	var hist []float64
	var style lipgloss.Style
	switch d {
	case devCPU:
		name, hist, style = "CPU", m.cpuHist.values(), cpuGraphStyle
		value = "--"
		if m.snap.CPUKnown {
			value = fmt.Sprintf("%.0f%%", m.snap.CPUTotal)
		}
		if m.cpu.BaseMHz > 0 {
			value += fmt.Sprintf("  %.2f GHz", m.cpu.BaseMHz/1000)
		}
	case devMemory:
		name, hist, style = "Memory", m.memHist.values(), memGraphStyle
		value = fmt.Sprintf("%s/%s (%.0f%%)", shortBytes(m.snap.MemUsed), shortBytes(m.snap.MemTotal),
			pct(m.snap.MemUsed, m.snap.MemTotal))
	}
	marker := "  "
	title := name
	if selected {
		marker = selMarkerStyle.Render("▌ ")
		title = lipgloss.NewStyle().Bold(true).Render(name)
	}
	return []string{
		marker + title,
		marker + dimStyle.Render(value),
		marker + style.Render(sparkline(hist, sideW-3)),
	}
}

func (m model) cpuDetail(w, h int) []string {
	cores := strconv.Itoa(m.cpu.Physical)
	if p, e := m.cpu.Count(metrics.CorePerformance), m.cpu.Count(metrics.CoreEfficiency); p > 0 && e > 0 {
		cores = fmt.Sprintf("%d (%dP + %dE)", m.cpu.Physical, p, e)
	}
	util := "--"
	if m.snap.CPUKnown {
		util = fmt.Sprintf("%.1f%%", m.snap.CPUTotal)
	}
	base := "--"
	if m.cpu.BaseMHz > 0 {
		base = fmt.Sprintf("%.2f GHz", m.cpu.BaseMHz/1000)
	}
	stats := []string{
		kv("Utilization", util) + "   " + kv("Base speed", base) + "   " +
			kv("Cores", cores) + "   " + kv("Logical", strconv.Itoa(m.cpu.Logical)),
		kv("Processes", strconv.Itoa(m.snap.ProcCount)) + "   " + kv("Up", view.FormatUptime(m.snap.Uptime)),
	}
	grid := m.coreGrid(w)

	head := []string{
		spread(labelStyle.Render("CPU"), dimStyle.Render(m.cpu.Model), w),
		dimStyle.Render("% Utilization · " + m.graphSpan(w-2)),
	}
	tail := append(append([]string{""}, stats...), "")
	tail = append(tail, grid...)
	return m.withGraph(head, tail, m.cpuHist.values(), cpuGraphStyle, w, h)
}

func (m model) memDetail(w, h int) []string {
	s := m.snap
	stats := []string{
		kv("In use", fmt.Sprintf("%s (%.0f%%)", view.FormatBytes(s.MemUsed), pct(s.MemUsed, s.MemTotal))) + "   " +
			kv("Available", view.FormatBytes(s.MemAvailable)),
	}
	var second []string
	if s.MemCached > 0 {
		second = append(second, kv("Cached", view.FormatBytes(s.MemCached)))
	}
	if s.CommitLimit > 0 {
		second = append(second, kv("Committed", shortBytes(s.CommitUsed)+"/"+shortBytes(s.CommitLimit)))
	}
	second = append(second, kv("Swap", shortBytes(s.SwapUsed)+"/"+shortBytes(s.SwapTotal)))
	stats = append(stats, strings.Join(second, "   "))

	head := []string{
		spread(labelStyle.Render("Memory"), dimStyle.Render(view.FormatBytes(s.MemTotal)), w),
		dimStyle.Render("% In use · " + m.graphSpan(w-2)),
	}
	tail := []string{"", dimStyle.Render("Composition"), m.memComposition(w), ""}
	tail = append(tail, stats...)
	return m.withGraph(head, tail, m.memHist.values(), memGraphStyle, w, h)
}

// withGraph stacks head, a framed history graph filling the spare height,
// and tail. Output is cut to h lines.
func (m model) withGraph(head, tail []string, hist []float64, style lipgloss.Style, w, h int) []string {
	gh := max(3, h-len(head)-len(tail)-2) // -2 for the frame
	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
	graph := frame.Render(style.Render(strings.Join(areaGraph(hist, w-2, gh), "\n")))

	lines := append(head, strings.Split(graph, "\n")...)
	lines = append(lines, tail...)
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

// coreGrid renders one cell per logical CPU: index, P/E kind, meter, %.
func (m model) coreGrid(w int) []string {
	const cellW = 16
	per := max(1, w/cellW)
	var lines []string
	var line strings.Builder
	for i, v := range m.snap.CPUPerCore {
		var kind metrics.CoreKind
		if i < len(m.cpu.Kinds) {
			kind = m.cpu.Kinds[i]
		}
		label := fmt.Sprintf("%3d", i)
		switch kind {
		case metrics.CorePerformance:
			label += pCoreStyle.Render("P")
		case metrics.CoreEfficiency:
			label += eCoreStyle.Render("E")
		default:
			label += " "
		}
		line.WriteString(label + " " + bar(v, 5) + loadStyle(v).Render(fmt.Sprintf("%4.0f%%", v)) + " ")
		if (i+1)%per == 0 {
			lines = append(lines, line.String())
			line.Reset()
		}
	}
	if line.Len() > 0 {
		lines = append(lines, line.String())
	}
	return lines
}

// memComposition is a single bar: in use | cached | free.
func (m model) memComposition(w int) string {
	s := m.snap
	if s.MemTotal == 0 || w <= 0 {
		return ""
	}
	used := int(float64(w) * float64(s.MemUsed) / float64(s.MemTotal))
	cached := min(w-used, int(float64(w)*float64(s.MemCached)/float64(s.MemTotal)))
	free := max(0, w-used-cached)
	return memGraphStyle.Render(strings.Repeat("█", used)) +
		cachedStyle.Render(strings.Repeat("▓", cached)) +
		dimStyle.Render(strings.Repeat("░", free))
}

// graphSpan describes how much time a graph of width w covers.
func (m model) graphSpan(w int) string {
	return (time.Duration(min(w, historyLen)) * m.refresh).Round(time.Second).String()
}

func kv(k, v string) string { return dimStyle.Render(k+" ") + v }
