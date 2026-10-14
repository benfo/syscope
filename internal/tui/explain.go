package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"taskman/internal/metrics"
)

// term is one explained metric: what it is, and when it matters.
type term struct{ name, text string }

// about returns the title and terms explaining what's currently on screen.
// Terms only appear when the metric is shown on this platform.
func (m model) about() (string, []term) {
	if m.tab == tabProcesses {
		return "Processes", processTerms
	}
	devs := m.perfDevices()
	sel, _ := m.selectedDevice(devs)
	switch sel.kind {
	case devMemory:
		return "Memory", m.memoryTerms()
	case devDisk:
		return diskTitle(m.snap.Disks[sel.idx]), m.diskTerms(m.snap.Disks[sel.idx])
	case devNet:
		return m.snap.Nets[sel.idx].Name, netTerms
	case devGPU:
		g := m.snap.GPUs[sel.idx]
		return "GPU " + g.ID + " · " + g.Name, gpuTerms(g)
	}
	return "CPU", m.cpuTerms()
}

func gpuTerms(g metrics.GPUStat) []term {
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

var processTerms = []term{
	{"PID", "Process ID, a number the OS gives each running process. Use it with `taskman kill <pid>`. IDs are reused after a process exits."},
	{"NAME", "The program's executable name. Grouping (z) combines processes with the same name, e.g. \"chrome (31)\", summing their CPU and memory."},
	{"USER", "The account the process runs as. Blank usually means a system account you can't inspect without admin rights."},
	{"CPU%", "Share of total CPU capacity used since the last refresh. Normalized (default): 100% means every core is busy, like Task Manager. " +
		"Per-core (c): 100% means one full core, so a busy process can exceed 100%. \"--\" means no permission to read it (run as admin / sudo)."},
	{"MEM", "Resident memory (the working set on Windows): RAM the process is using right now, including shared libraries. " +
		"It reads higher than Task Manager's \"private\" figure, and the column can add up to more than your RAM because shared pages count in every process."},
	{"Top line", "CPU and MEM as on the Performance tab. DISK is the busiest disk's active time. NET is total download/upload on the adapters shown. " +
		"SWAP is memory paged out to disk; if it keeps growing while MEM is high, you're short of RAM."},
}

func (m model) cpuTerms() []term {
	t := []term{
		{"Utilization", "Share of time all logical processors were busy over the last refresh. Short spikes are normal; staying above ~85% means the CPU is the bottleneck."},
		{"Base speed", "The CPU's rated clock. Real clocks boost above it under load and drop below it when the chip is too hot (thermal throttling)."},
		{"Cores / Logical", "Physical cores vs the threads the OS schedules on. With hyper-threading each core runs two threads, so Logical is double."},
	}
	if m.cpuInfo.Count(metrics.CorePerformance) > 0 && m.cpuInfo.Count(metrics.CoreEfficiency) > 0 {
		t = append(t, term{"P / E cores", "Hybrid CPU: Performance cores are fast; Efficiency cores are smaller and save power. " +
			"Background work on E cores is normal, but a game or build running mostly on E cores will be slower."})
	}
	return append(t,
		term{"Core grid", "Load on each logical CPU. One core pinned at 100% while the rest idle means a single-threaded program is the bottleneck, even if total CPU looks low."},
		term{"Graph", "Total CPU utilization over time, newest on the right."},
	)
}

func (m model) memoryTerms() []term {
	t := []term{
		{"In use", "RAM used by programs and the OS. It doesn't count cache that can be freed instantly."},
		{"Available", "RAM that programs can get right now without anything being moved to disk. Below ~10% of total, the system starts swapping and slows down."},
	}
	if m.snap.MemCached > 0 {
		t = append(t, term{"Cached", "Recently used file data kept in RAM to make it faster to open again. It's given back instantly when programs need it, so a big cache is good, not wasted."})
	}
	if m.snap.CommitLimit > 0 {
		t = append(t, term{"Committed", "Memory programs have reserved, which Windows promises to back with RAM or the page file. " +
			"Near the limit, Windows grows the page file; if it can't, programs fail with \"out of memory\" even while RAM shows free."})
	}
	return append(t,
		term{"Swap", "Data moved out of RAM to disk (the page file on Windows). Some is normal; steady growth while you work means you need more RAM."},
		term{"Composition", "The bar shows in-use memory, then cache (if reported), then free."},
	)
}

func (m model) diskTerms(d metrics.DiskStat) []term {
	var t []term
	if d.ActiveKnown {
		t = append(t, term{"Active time", "Share of time the disk was handling at least one request. Near 100% means the disk is the bottleneck, " +
			"even if MB/s looks low (lots of small random reads)."})
	} else {
		t = append(t, term{"Active time", "Not available on this OS; the graph shows transfer rate instead."})
	}
	t = append(t, term{"Read / Write", "Data transferred per second. Rough maximums: NVMe SSD 2,000–7,000 MB/s, SATA SSD ~550 MB/s, hard drive ~150 MB/s. " +
		"Each graph auto-scales; the scale above it is what full height means."})
	if d.RespKnown {
		t = append(t, term{"Response", "Average time per read or write. Under 1 ms is normal for NVMe; above ~20 ms feels sluggish; " +
			"hundreds of ms means something is hammering the disk, or it's failing."})
	}
	return append(t, term{"Volumes", "Drives on this disk and how full they are. Keep SSDs below ~90% full; they slow down when nearly full."})
}

var netTerms = []term{
	{"Receive / Send", "Data in and out, in bits per second, the same unit as internet plans (100 Mbps ≈ 12.5 MB/s)."},
	{"Link", "Speed negotiated with your router or switch. It's the ceiling for this adapter, not your internet speed. On Wi-Fi it changes with signal strength."},
	{"Type", "Hardware is a physical Wi-Fi or Ethernet adapter. Virtual is VPN, Hyper-V, WSL or Docker; that traffic usually also passes through a hardware adapter, so it's counted twice."},
	{"Address", "This machine's IP address on that network. 169.254.x.x means it didn't get an address (not properly connected)."},
	{"Shown adapters", "By default only adapters that are up and either physical or carrying traffic. Press . to show all."},
	{"Graph", "Receive and Send share one scale so you can compare them; the scale is shown in the subtitle."},
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
	case actCancel, actHelp, actQuit:
		m.showHelp = false
	}
	m.helpOff = max(0, min(m.helpOff, len(m.helpContent())-page))
	return m
}
