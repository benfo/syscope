package tui

import (
	"fmt"
	"strconv"

	"github.com/charmbracelet/lipgloss"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

const cpuKey = "cpu"

type cpuPage struct{ m model }

func cpuPages(m model) []devicePage { return []devicePage{cpuPage{m}} }

func (p cpuPage) key() string           { return cpuKey }
func (p cpuPage) name() string          { return "CPU" }
func (p cpuPage) helpTitle() string     { return "CPU" }
func (p cpuPage) hidden() bool          { return false }
func (p cpuPage) style() lipgloss.Style { return ui.cpu }
func (p cpuPage) trend() []float64      { return p.m.histories.values(cpuKey, "util") }

func (p cpuPage) record() {
	if p.m.snap.CPUKnown {
		p.m.histories.push(cpuKey, "util", p.m.snap.CPUTotal)
	}
}

func (p cpuPage) summary() string {
	s := "--"
	if p.m.snap.CPUKnown {
		s = fmt.Sprintf("%.0f%%", p.m.snap.CPUTotal)
	}
	if p.m.cpuInfo.BaseMHz > 0 {
		s += fmt.Sprintf("  %.2f GHz", p.m.cpuInfo.BaseMHz/1000)
	}
	return s
}

func (p cpuPage) detail(w, h int) []string {
	info, snap := p.m.cpuInfo, p.m.snap
	cores := strconv.Itoa(info.Physical)
	if pc, ec := info.Count(metrics.CorePerformance), info.Count(metrics.CoreEfficiency); pc > 0 && ec > 0 {
		cores = fmt.Sprintf("%d (%dP + %dE)", info.Physical, pc, ec)
	}
	util := "--"
	if snap.CPUKnown {
		util = fmt.Sprintf("%.1f%%", snap.CPUTotal)
	}
	base := "--"
	if info.BaseMHz > 0 {
		base = fmt.Sprintf("%.2f GHz", info.BaseMHz/1000)
	}
	stats := packLine([]string{
		kv("Utilization", util), kv("Base speed", base), kv("Cores", cores), kv("Logical", strconv.Itoa(info.Logical)),
		kv("Processes", strconv.Itoa(snap.ProcCount)), kv("Up", view.FormatUptime(snap.Uptime)),
	}, statSep, w)
	head := []string{
		spread(ui.label.Render("CPU"), ui.muted.Render(info.Model), w),
		ui.muted.Render("% Utilization · " + p.m.graphSpan(w-2)),
	}
	tail := append(append([]string{""}, stats...), "")
	tail = append(tail, p.coreGrid(w)...)
	return withGraphs(head, tail, []graphSpec{{vals: p.trend(), style: ui.cpu}}, w, h)
}

// coreGrid renders one cell per logical CPU: index, P/E kind, meter, %.
func (p cpuPage) coreGrid(w int) []string {
	kinds := p.m.cpuInfo.Kinds
	cells := make([]string, len(p.m.snap.CPUPerCore))
	for i, v := range p.m.snap.CPUPerCore {
		label := fmt.Sprintf("%3d", i)
		var kind metrics.CoreKind
		if i < len(kinds) {
			kind = kinds[i]
		}
		switch kind {
		case metrics.CorePerformance:
			label += ui.pCore.Render("P")
		case metrics.CoreEfficiency:
			label += ui.eCore.Render("E")
		default:
			label += " "
		}
		cells[i] = meterCell(label, v)
	}
	return packLine(cells, gridSep, w)
}

func (p cpuPage) terms() []term {
	t := []term{
		{"Utilization", "Share of time all logical processors were busy over the last refresh. Short spikes are normal; staying above ~85% means the CPU is the bottleneck."},
		{"Base speed", "The CPU's rated clock. Real clocks boost above it under load and drop below it when the chip is too hot (thermal throttling)."},
		{"Cores / Logical", "Physical cores vs the threads the OS schedules on. With hyper-threading each core runs two threads, so Logical is double."},
	}
	if p.m.cpuInfo.Count(metrics.CorePerformance) > 0 && p.m.cpuInfo.Count(metrics.CoreEfficiency) > 0 {
		t = append(t, term{"P / E cores", "Hybrid CPU: Performance cores are fast; Efficiency cores are smaller and save power. " +
			"Background work on E cores is normal, but a game or build running mostly on E cores will be slower."})
	}
	return append(t,
		term{"Core grid", "Load on each logical CPU. One core pinned at 100% while the rest idle means a single-threaded program is the bottleneck, even if total CPU looks low."},
		term{"Graph", "Total CPU utilization over time, newest on the right."},
	)
}
