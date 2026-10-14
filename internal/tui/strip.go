package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

// renderStrip is the one-line summary shown on every tab. When it's too
// wide, the least important items are dropped first.
func (m model) renderStrip() string {
	s := m.snap
	cpuText := "  --"
	if s.CPUKnown {
		cpuText = fmt.Sprintf("%4.1f%%", s.CPUTotal)
	}
	memPct := view.Percent(s.MemUsed, s.MemTotal)

	type item struct {
		text string
		keep int // drop priority: lowest goes first
	}
	items := []item{
		{ui.label.Render("CPU ") + loadStyle(s.CPUTotal).Render(cpuText+" "+sparkline(m.histories.values(cpuKey, "util"), 8)), 9},
		{ui.label.Render("MEM ") + loadStyle(memPct).Render(fmt.Sprintf("%.0f%%", memPct)) +
			ui.muted.Render(" "+view.FormatUsedTotal(s.MemUsed, s.MemTotal)), 8},
	}
	if len(s.GPUs) > 0 {
		items = append(items, item{ui.label.Render("GPU ") + m.stripGPU(), 7})
	}
	items = append(items,
		item{ui.label.Render("DISK ") + m.stripDisk(), 5},
		item{ui.label.Render("NET ") + m.stripNet(), 6},
		item{ui.label.Render("SWAP ") + ui.muted.Render(view.FormatUsedTotal(s.SwapUsed, s.SwapTotal)), 3},
		item{ui.label.Render("PROCS ") + strconv.Itoa(s.ProcCount), 2},
		item{ui.label.Render("UP ") + view.FormatUptime(s.Uptime), 1},
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

// stripGPU shows the busiest GPU's load and temperature, and the largest
// card's VRAM use.
func (m model) stripGPU() string {
	var busiest, vram *metrics.GPUStat
	for i := range m.snap.GPUs {
		g := &m.snap.GPUs[i]
		if g.UtilKnown && (busiest == nil || g.Util > busiest.Util) {
			busiest = g
		}
		if g.MemKnown && (vram == nil || g.MemTotal > vram.MemTotal) {
			vram = g
		}
	}
	if busiest == nil {
		return ui.muted.Render("--")
	}
	s := loadStyle(busiest.Util).Render(fmt.Sprintf("%.0f%%", busiest.Util))
	if busiest.TempKnown {
		s += " " + tempStyle(busiest.TempC).Render(fmt.Sprintf("%.0f°C", busiest.TempC))
	}
	if vram != nil && hasVRAM(*vram) {
		s += ui.muted.Render(" VRAM " + view.FormatUsedTotal(vram.MemUsed, vram.MemTotal))
	}
	return s
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
		return ui.muted.Render("--")
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
