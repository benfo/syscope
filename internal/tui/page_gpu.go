package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

type gpuPage struct {
	m model
	g metrics.GPUStat
}

func gpuPages(m model) []devicePage {
	out := make([]devicePage, len(m.snap.GPUs))
	for i, g := range m.snap.GPUs {
		out[i] = gpuPage{m, g}
	}
	return out
}

func (p gpuPage) key() string           { return "gpu:" + p.g.ID }
func (p gpuPage) hidden() bool          { return false }
func (p gpuPage) style() lipgloss.Style { return ui.gpu }
func (p gpuPage) helpTitle() string     { return "GPU " + p.g.ID + " · " + p.g.Name }

// name is the sidebar title: "GPU 0 RTX 3080 Ti Laptop".
func (p gpuPage) name() string { return "GPU " + p.g.ID + " " + shortGPUName(p.g.Name) }

// shortGPUName trims vendor boilerplate for narrow places:
// "NVIDIA GeForce RTX 3080 Ti Laptop GPU" → "RTX 3080 Ti Laptop".
func shortGPUName(name string) string {
	r := strings.NewReplacer("NVIDIA ", "", "GeForce ", "", "(R)", "", "(TM)", "", "AMD ", "", "Radeon(TM) ", "")
	return strings.TrimSuffix(strings.TrimSpace(r.Replace(name)), " GPU")
}

func (p gpuPage) record() {
	if p.g.UtilKnown {
		p.m.histories.push(p.key(), "util", p.g.Util)
	}
	if p.g.MemKnown && p.g.MemTotal > 0 {
		p.m.histories.push(p.key(), "vram", view.Percent(p.g.MemUsed, p.g.MemTotal))
	}
}

func (p gpuPage) series(name string) []float64 { return p.m.histories.values(p.key(), name) }
func (p gpuPage) trend() []float64             { return p.series("util") }

// summary is the sidebar value line: utilization, temperature, VRAM.
func (p gpuPage) summary() string {
	g := p.g
	parts := []string{"--"}
	if g.UtilKnown {
		parts[0] = fmt.Sprintf("%.0f%%", g.Util)
	}
	if g.TempKnown {
		parts = append(parts, fmt.Sprintf("%.0f°C", g.TempC))
	}
	if hasVRAM(g) {
		parts = append(parts, view.FormatUsedTotal(g.MemUsed, g.MemTotal))
	}
	return strings.Join(parts, "  ")
}

// hasVRAM is false for integrated GPUs, whose "dedicated" memory is a tiny
// carve-out not worth showing.
func hasVRAM(g metrics.GPUStat) bool { return g.MemKnown && g.MemTotal >= 1<<30 }

func (p gpuPage) detail(w, h int) []string {
	g := p.g
	graphs := []graphSpec{{vals: p.series("util"), style: ui.gpu}}
	if g.MemKnown && g.MemTotal > 0 {
		graphs = append(graphs, graphSpec{
			title: "Dedicated memory · " + view.FormatBytesCompact(g.MemUsed) + " / " + view.FormatBytesCompact(g.MemTotal),
			vals:  p.series("vram"), style: ui.memory,
		})
	}

	tail := append([]string{""}, packLine(p.stats(), statSep, w)...)
	if g.ThrottleKnown {
		tail = append(tail, kv("Limited by", throttleText(g.Throttle)))
	}
	if grid := engineGrid(g.Engines, w); len(grid) > 0 {
		tail = append(tail, "")
		tail = append(tail, grid...)
	}
	head := []string{
		spread(ui.label.Render("GPU "+g.ID), ui.muted.Render(g.Name), w),
		ui.muted.Render("Utilization (busiest engine) · " + p.m.graphSpan(w-2)),
	}
	return p.m.withGraphs(head, tail, graphs, w, h)
}

// stats lists every reading the GPU reports, skipping the ones it doesn't.
func (p gpuPage) stats() []string {
	g := p.g
	util := "--"
	if g.UtilKnown {
		util = fmt.Sprintf("%.0f%%", g.Util)
	}
	s := []string{kv("Utilization", util)}
	if g.MemKnown {
		s = append(s, kv("Dedicated", view.FormatBytesCompact(g.MemUsed)+"/"+view.FormatBytesCompact(g.MemTotal)))
	}
	if g.SharedTotal > 0 {
		s = append(s, kv("Shared", view.FormatBytesCompact(g.SharedUsed)+"/"+view.FormatBytesCompact(g.SharedTotal)))
	}
	if g.TempKnown {
		s = append(s, kv("Temperature", tempStyle(g.TempC).Render(fmt.Sprintf("%.0f °C", g.TempC))))
	}
	switch {
	case g.PowerKnown && g.PowerLimitW > 0:
		s = append(s, kv("Power", fmt.Sprintf("%.1f / %.0f W", g.PowerW, g.PowerLimitW)))
	case g.PowerKnown:
		s = append(s, kv("Power", fmt.Sprintf("%.1f W", g.PowerW)))
	case g.PowerPct > 0:
		s = append(s, kv("Power", fmt.Sprintf("%.0f%% of max", g.PowerPct)))
	}
	if g.ClockKnown {
		clock := fmt.Sprintf("%d MHz", g.ClockMHz)
		if g.ClockMaxMHz > 0 {
			clock = fmt.Sprintf("%d / %d MHz", g.ClockMHz, g.ClockMaxMHz)
		}
		s = append(s, kv("Clock", clock))
		if g.MemClockMHz > 0 {
			s = append(s, kv("Memory clock", fmt.Sprintf("%d MHz", g.MemClockMHz)))
		}
	}
	switch {
	case g.FanKnown:
		s = append(s, kv("Fan", fmt.Sprintf("%d%%", g.FanPct)))
	case g.FanRPM > 0:
		s = append(s, kv("Fan", fmt.Sprintf("%d rpm", g.FanRPM)))
	}
	if g.PState >= 0 {
		s = append(s, kv("P-state", fmt.Sprintf("P%d", g.PState)))
	}
	return s
}

// throttleText explains why clocks are below max; thermal and power limits
// are highlighted because they cost performance under load.
func throttleText(reasons []string) string {
	if len(reasons) == 0 {
		return ui.good.Render("nothing (running at full speed when busy)")
	}
	var parts []string
	for _, r := range reasons {
		switch {
		case strings.Contains(r, "thermal"), strings.Contains(r, "slowdown"), strings.Contains(r, "brake"):
			parts = append(parts, ui.bad.Render(r))
		case strings.Contains(r, "power"):
			parts = append(parts, ui.warn.Render(r))
		default:
			parts = append(parts, r)
		}
	}
	return strings.Join(parts, ", ")
}

var mainEngines = map[string]bool{
	"3D": true, "Compute": true, "Copy": true, "Video Decode": true, "Video Encode": true, "Graphics": true,
}

// engineGrid shows each engine's current load, like the CPU core grid.
// The main engines always show; niche ones (JPEG, OFA, Security, …) only
// while busy.
func engineGrid(all []metrics.EngineStat, w int) []string {
	var cells []string
	for _, e := range all {
		if mainEngines[e.Name] || e.Util >= 1 {
			cells = append(cells, meterCell(fit(e.Name, 14), e.Util))
		}
	}
	return packLine(cells, statSep, w)
}

func tempStyle(c float64) lipgloss.Style {
	switch {
	case c >= 85:
		return ui.bad
	case c >= 75:
		return ui.warn
	}
	return ui.good
}

func (p gpuPage) terms() []term {
	g := p.g
	t := []term{
		{"Utilization", "How busy the GPU is: the busiest engine, as in Task Manager. Games show up as 3D; AI and CUDA work as 3D or Compute. " +
			"High utilization is normal while gaming or running a model."},
	}
	if len(g.Engines) > 0 {
		t = append(t, term{"Engines", "3D: games, the desktop, some AI. Compute: CUDA / AI work. Copy: moving data between RAM and the GPU, " +
			"high while loading a game or model. Video Decode / Encode: watching, streaming or recording video, done by dedicated hardware."})
	}
	if g.MemKnown {
		t = append(t, term{"Dedicated", "The GPU's own memory (VRAM). A local LLM must fit here to run fast; when it's full, data spills into " +
			"shared memory over PCIe and tokens per second drop sharply. Games stutter when they run out."})
	}
	if g.SharedTotal > 0 {
		t = append(t, term{"Shared", "System RAM the GPU can borrow. Integrated GPUs use only this. On a discrete GPU, heavy use means VRAM has run out."})
	}
	if g.TempKnown {
		t = append(t, term{"Temperature", "GPU core temperature. 30–50 °C idle is normal; laptops often run 70–87 °C under load. " +
			"Near the GPU's limit (about 87 °C for NVIDIA) clocks drop to cool down."})
	}
	if g.PowerKnown || g.PowerPct > 0 {
		t = append(t, term{"Power", "Current power draw, and the limit when the driver reports one. On laptops the limit is set by the manufacturer " +
			"and is often what caps performance."})
	}
	if g.ClockKnown {
		t = append(t, term{"Clock", "Current vs maximum graphics clock. Low clocks while idle are normal; low clocks under load mean something is limiting the GPU (see Limited by)."})
	}
	if g.FanKnown || g.FanRPM > 0 {
		t = append(t, term{"Fan", "Cooling fan speed. Laptop GPUs often report none; the laptop's own fans cool them."})
	}
	if g.PState >= 0 {
		t = append(t, term{"P-state", "Performance state: P0 is full performance, higher numbers (P8, P12) are power-saving idle states."})
	}
	if g.ThrottleKnown {
		t = append(t, term{"Limited by", "Why clocks are below maximum right now. idle: nothing to do. power cap: hit the power limit (usual on laptops under load). " +
			"thermal: too hot, so improve cooling. reliability: the driver is keeping voltage in a safe range (normal). " +
			"hardware slowdown / power brake: an emergency limit, worth investigating."})
	}
	return t
}
