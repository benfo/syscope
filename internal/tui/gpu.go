package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"taskman/internal/metrics"
)

func (m model) recordGPUHistory() {
	for _, g := range m.snap.GPUs {
		k := "gpu:" + g.ID
		if g.UtilKnown {
			m.push(k+":util", g.Util)
		}
		if g.MemKnown && g.MemTotal > 0 {
			m.push(k+":vram", pct(g.MemUsed, g.MemTotal))
		}
	}
}

// gpuTitle is the sidebar name: "GPU 0 RTX 3080 Ti Laptop".
func gpuTitle(g metrics.GPUStat) string {
	return "GPU " + g.ID + " " + shortGPUName(g.Name)
}

// shortGPUName trims vendor boilerplate for narrow places:
// "NVIDIA GeForce RTX 3080 Ti Laptop GPU" → "RTX 3080 Ti Laptop".
func shortGPUName(name string) string {
	r := strings.NewReplacer("NVIDIA ", "", "GeForce ", "", "(R)", "", "(TM)", "", "AMD ", "", "Radeon(TM) ", "")
	return strings.TrimSuffix(strings.TrimSpace(r.Replace(name)), " GPU")
}

// gpuSummary is the sidebar value line: utilization and temperature.
func gpuSummary(g metrics.GPUStat) string {
	parts := []string{"--"}
	if g.UtilKnown {
		parts[0] = fmt.Sprintf("%.0f%%", g.Util)
	}
	if g.TempKnown {
		parts = append(parts, fmt.Sprintf("%.0f°C", g.TempC))
	}
	if g.MemKnown && g.MemTotal >= 1<<30 { // skip iGPUs' tiny carve-out
		parts = append(parts, gibPair(g.MemUsed, g.MemTotal))
	}
	return strings.Join(parts, "  ")
}

func (m model) gpuDetail(g metrics.GPUStat, w, h int) []string {
	key := "gpu:" + g.ID
	graphs := []graphSpec{{vals: m.histVals(key + ":util"), style: gpuGraphStyle}}
	if g.MemKnown && g.MemTotal > 0 {
		graphs = append(graphs, graphSpec{
			title: "Dedicated memory · " + shortBytes(g.MemUsed) + " / " + shortBytes(g.MemTotal),
			vals:  m.histVals(key + ":vram"), style: memGraphStyle,
		})
	}

	util := "--"
	if g.UtilKnown {
		util = fmt.Sprintf("%.0f%%", g.Util)
	}
	line1 := []string{kv("Utilization", util)}
	if g.MemKnown {
		line1 = append(line1, kv("Dedicated", shortBytes(g.MemUsed)+"/"+shortBytes(g.MemTotal)))
	}
	if g.SharedTotal > 0 {
		line1 = append(line1, kv("Shared", shortBytes(g.SharedUsed)+"/"+shortBytes(g.SharedTotal)))
	}

	var line2 []string
	if g.TempKnown {
		line2 = append(line2, kv("Temperature", tempStyle(g.TempC).Render(fmt.Sprintf("%.0f °C", g.TempC))))
	}
	switch {
	case g.PowerKnown && g.PowerLimitW > 0:
		line2 = append(line2, kv("Power", fmt.Sprintf("%.1f / %.0f W", g.PowerW, g.PowerLimitW)))
	case g.PowerKnown:
		line2 = append(line2, kv("Power", fmt.Sprintf("%.1f W", g.PowerW)))
	case g.PowerPct > 0:
		line2 = append(line2, kv("Power", fmt.Sprintf("%.0f%% of max", g.PowerPct)))
	}
	if g.ClockKnown {
		clock := fmt.Sprintf("%d MHz", g.ClockMHz)
		if g.ClockMaxMHz > 0 {
			clock = fmt.Sprintf("%d / %d MHz", g.ClockMHz, g.ClockMaxMHz)
		}
		line2 = append(line2, kv("Clock", clock))
		if g.MemClockMHz > 0 {
			line2 = append(line2, kv("Memory clock", fmt.Sprintf("%d MHz", g.MemClockMHz)))
		}
	}
	switch {
	case g.FanKnown:
		line2 = append(line2, kv("Fan", fmt.Sprintf("%d%%", g.FanPct)))
	case g.FanRPM > 0:
		line2 = append(line2, kv("Fan", fmt.Sprintf("%d rpm", g.FanRPM)))
	}
	if g.PState >= 0 {
		line2 = append(line2, kv("P-state", fmt.Sprintf("P%d", g.PState)))
	}

	tail := append([]string{""}, packLine(append(line1, line2...), w)...)
	if g.ThrottleKnown {
		tail = append(tail, kv("Limited by", throttleText(g.Throttle)))
	}
	if grid := engineGrid(g.Engines, w); len(grid) > 0 {
		tail = append(tail, "")
		tail = append(tail, grid...)
	}

	head := []string{
		spread(labelStyle.Render("GPU "+g.ID), dimStyle.Render(g.Name), w),
		dimStyle.Render("Utilization (busiest engine) · " + m.graphSpan(w-2)),
	}
	return withGraphs(head, tail, graphs, w, h)
}

// throttleText explains why clocks are below max; thermal and power limits
// are highlighted because they cost performance under load.
func throttleText(reasons []string) string {
	if len(reasons) == 0 {
		return coolStyle.Render("nothing (running at full speed when busy)")
	}
	var parts []string
	for _, r := range reasons {
		switch {
		case strings.Contains(r, "thermal"), strings.Contains(r, "slowdown"), strings.Contains(r, "brake"):
			parts = append(parts, hotStyle.Render(r))
		case strings.Contains(r, "power"):
			parts = append(parts, warmStyle.Render(r))
		default:
			parts = append(parts, r)
		}
	}
	return strings.Join(parts, ", ")
}

// engineGrid shows each engine's current load, like the CPU core grid.
// The main engines always show; niche ones (JPEG, OFA, Security, …) only
// while busy.
func engineGrid(all []metrics.EngineStat, w int) []string {
	var engines []metrics.EngineStat
	for _, e := range all {
		if mainEngines[e.Name] || e.Util >= 1 {
			engines = append(engines, e)
		}
	}
	const cellW = 28
	per := max(1, w/cellW)
	var lines []string
	var line strings.Builder
	for i, e := range engines {
		line.WriteString(fit(e.Name, 14) + " " + bar(e.Util, 5) + loadStyle(e.Util).Render(fmt.Sprintf("%4.0f%%", e.Util)) + "   ")
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

// stripGPU shows the busiest GPU's load and the largest card's VRAM use.
func (m model) stripGPU() string {
	var busiest *metrics.GPUStat
	var vram *metrics.GPUStat
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
		return dimStyle.Render("--")
	}
	s := loadStyle(busiest.Util).Render(fmt.Sprintf("%.0f%%", busiest.Util))
	if busiest.TempKnown {
		s += " " + tempStyle(busiest.TempC).Render(fmt.Sprintf("%.0f°C", busiest.TempC))
	}
	if vram != nil && vram.MemTotal >= 1<<30 {
		s += dimStyle.Render(" VRAM " + gibPair(vram.MemUsed, vram.MemTotal))
	}
	return s
}

func tempStyle(c float64) lipgloss.Style {
	switch {
	case c >= 85:
		return hotStyle
	case c >= 75:
		return warmStyle
	}
	return coolStyle
}

var mainEngines = map[string]bool{
	"3D": true, "Compute": true, "Copy": true, "Video Decode": true, "Video Encode": true, "Graphics": true,
}
