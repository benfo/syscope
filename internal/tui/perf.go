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

type devKind int

const (
	devCPU devKind = iota
	devMemory
	devDisk
	devNet
)

// perfDevice is one entry in the Performance sidebar. key is stable across
// refreshes so the selection survives devices appearing or disappearing.
type perfDevice struct {
	kind devKind
	key  string
	idx  int // index into snap.Disks or snap.Nets
}

const (
	sideW = 24
	// Graph scale floors so idle devices don't show noise as full-height.
	diskRateFloor = 1_000_000 // 1 MB/s
	netRateFloor  = 12_500    // 100 Kbps
)

func (m model) perfDevices() []perfDevice {
	devs := []perfDevice{{kind: devCPU, key: "cpu"}, {kind: devMemory, key: "mem"}}
	for i, d := range m.snap.Disks {
		devs = append(devs, perfDevice{kind: devDisk, key: "disk:" + d.ID, idx: i})
	}
	for i, n := range m.snap.Nets {
		if n.Default || m.showAllNets {
			devs = append(devs, perfDevice{kind: devNet, key: "net:" + n.Name, idx: i})
		}
	}
	return devs
}

// selectedDevice returns the selected device and its index, falling back to
// the first device when the selected one has gone.
func (m model) selectedDevice(devs []perfDevice) (perfDevice, int) {
	for i, d := range devs {
		if d.key == m.perfKey {
			return d, i
		}
	}
	return devs[0], 0
}

func (m model) perfAction(act action) model {
	devs := m.perfDevices()
	_, i := m.selectedDevice(devs)
	switch act {
	case actUp, actHalfUp, actPageUp:
		i = max(0, i-1)
	case actDown, actHalfDown, actPageDown:
		i = min(len(devs)-1, i+1)
	case actTop:
		i = 0
	case actBottom:
		i = len(devs) - 1
	case actShowHidden:
		m.showAllNets = !m.showAllNets
		return m
	default:
		return m
	}
	m.perfKey = devs[i].key
	return m
}

// recordHistory appends this sample's per-device values to their graphs.
func (m model) recordHistory() {
	for _, d := range m.snap.Disks {
		if d.RatesKnown {
			k := "disk:" + d.ID
			m.push(k+":active", d.ActivePct)
			m.push(k+":read", d.ReadBps)
			m.push(k+":write", d.WriteBps)
		}
	}
	for _, n := range m.snap.Nets {
		if n.RatesKnown {
			m.push("net:"+n.Name+":rx", n.RecvBps)
			m.push("net:"+n.Name+":tx", n.SentBps)
		}
	}
}

func (m model) push(key string, v float64) {
	r := m.hist[key]
	if r == nil {
		r = newRing(historyLen)
		m.hist[key] = r
	}
	r.push(v)
}

func (m model) histVals(key string) []float64 {
	if r := m.hist[key]; r != nil {
		return r.values()
	}
	return nil
}

func (m model) renderPerf() string {
	h := m.height - chromeLines
	devs := m.perfDevices()
	sel, selIdx := m.selectedDevice(devs)

	detailW := max(10, m.width-sideW-2)
	var detail []string
	switch sel.kind {
	case devCPU:
		detail = m.cpuDetail(detailW, h)
	case devMemory:
		detail = m.memDetail(detailW, h)
	case devDisk:
		detail = m.diskDetail(m.snap.Disks[sel.idx], detailW, h)
	case devNet:
		detail = m.netDetail(m.snap.Nets[sel.idx], detailW, h)
	}

	side := m.sidebar(devs, selIdx, h)
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

// sidebar renders device entries, dropping spacing and then scrolling when
// they don't fit in h lines.
func (m model) sidebar(devs []perfDevice, selIdx, h int) []string {
	per := 4 // name, value, sparkline, blank
	if len(devs)*per > h {
		per = 3
	}
	first := 0
	if fit := max(1, h/per); selIdx >= fit {
		first = selIdx - fit + 1
	}
	var out []string
	if first > 0 {
		out = append(out, dimStyle.Render("  ▲ more"))
	}
	for i := first; i < len(devs); i++ {
		out = append(out, m.sidebarEntry(devs[i], i == selIdx)...)
		if per == 4 {
			out = append(out, "")
		}
	}
	if len(out) > h {
		out = append(out[:h-1], dimStyle.Render("  ▼ more"))
	}
	return out
}

func (m model) sidebarEntry(d perfDevice, selected bool) []string {
	var name, value string
	var hist []float64
	style := cpuGraphStyle
	switch d.kind {
	case devCPU:
		name, hist = "CPU", m.cpuHist.values()
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
	case devDisk:
		dk := m.snap.Disks[d.idx]
		name, style = diskTitle(dk), diskGraphStyle
		value = view.FormatByteRate(dk.ReadBps + dk.WriteBps)
		if dk.ActiveKnown {
			value = fmt.Sprintf("%.0f%%  %s", dk.ActivePct, value)
			hist = m.histVals(d.key + ":active")
		} else {
			hist, _ = scaled(diskRateFloor, sumSeries(m.histVals(d.key+":read"), m.histVals(d.key+":write")))
		}
	case devNet:
		n := m.snap.Nets[d.idx]
		name, style = n.Name, netGraphStyle
		value = "↓" + view.FormatBitRate(n.RecvBps) + " ↑" + view.FormatBitRate(n.SentBps)
		hist, _ = scaled(netRateFloor, sumSeries(m.histVals(d.key+":rx"), m.histVals(d.key+":tx")))
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
	head := []string{
		spread(labelStyle.Render("CPU"), dimStyle.Render(m.cpu.Model), w),
		dimStyle.Render("% Utilization · " + m.graphSpan(w-2)),
	}
	tail := append(append([]string{""}, stats...), "")
	tail = append(tail, m.coreGrid(w)...)
	return withGraphs(head, tail, []graphSpec{{vals: m.cpuHist.values(), style: cpuGraphStyle}}, w, h)
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
	return withGraphs(head, tail, []graphSpec{{vals: m.memHist.values(), style: memGraphStyle}}, w, h)
}

func (m model) diskDetail(d metrics.DiskStat, w, h int) []string {
	key := "disk:" + d.ID
	desc := d.Model
	if d.Bus != "" {
		desc = strings.TrimSpace(desc + " · " + d.Bus)
	}
	read, readMax := scaled(diskRateFloor, m.histVals(key+":read"))
	write, writeMax := scaled(diskRateFloor, m.histVals(key+":write"))

	var graphs []graphSpec
	sub := "Transfer rate · " + m.graphSpan(w-2)
	if d.ActiveKnown {
		sub = "% Active time · " + m.graphSpan(w-2)
		graphs = append(graphs, graphSpec{vals: m.histVals(key + ":active"), style: diskGraphStyle})
	}
	graphs = append(graphs,
		graphSpec{title: "Read · scale " + view.FormatByteRate(readMax), vals: read, style: readStyle},
		graphSpec{title: "Write · scale " + view.FormatByteRate(writeMax), vals: write, style: writeStyle},
	)

	var stats []string
	if d.ActiveKnown {
		stats = append(stats, kv("Active", fmt.Sprintf("%.0f%%", d.ActivePct)))
	}
	stats = append(stats, kv("Read", view.FormatByteRate(d.ReadBps)), kv("Write", view.FormatByteRate(d.WriteBps)))
	if d.RespKnown {
		stats = append(stats, kv("Response", fmt.Sprintf("%.2f ms", d.RespMs)))
	}
	tail := []string{"", strings.Join(stats, "   ")}
	for _, v := range d.Volumes {
		p := pct(v.Used, v.Total)
		tail = append(tail, fmt.Sprintf("%-6s ", v.Mount)+bar(p, 20)+
			fmt.Sprintf(" %s/%s (%.0f%%)", shortBytes(v.Used), shortBytes(v.Total), p))
	}
	head := []string{spread(labelStyle.Render(diskTitle(d)), dimStyle.Render(desc), w), dimStyle.Render(sub)}
	return withGraphs(head, tail, graphs, w, h)
}

func (m model) netDetail(n metrics.NetStat, w, h int) []string {
	key := "net:" + n.Name
	rx, tx := m.histVals(key+":rx"), m.histVals(key+":tx")
	// Receive and send share one scale so they're comparable at a glance.
	_, top := scaled(netRateFloor, sumSeries(rx, nil), tx)
	rxs, _ := scaled(top, rx)
	txs, _ := scaled(top, tx)
	graphs := []graphSpec{
		{title: "Receive", vals: rxs, style: rxStyle},
		{title: "Send", vals: txs, style: txStyle},
	}

	kind := "Virtual"
	if n.Physical {
		kind = "Hardware"
	}
	link := "--"
	if n.SpeedBps > 0 {
		link = view.FormatBitRate(float64(n.SpeedBps) / 8)
	}
	state := "Up"
	if !n.Up {
		state = "Down"
	}
	tail := []string{
		"",
		kv("Receive", view.FormatBitRate(n.RecvBps)) + "   " + kv("Send", view.FormatBitRate(n.SentBps)) + "   " +
			kv("Link", link) + "   " + kv("Type", kind) + "   " + kv("State", state),
	}
	if len(n.Addrs) > 0 {
		tail = append(tail, kv("Address", strings.Join(n.Addrs, "  ")))
	}
	head := []string{
		spread(labelStyle.Render(n.Name), dimStyle.Render(n.Desc), w),
		dimStyle.Render("Throughput · scale " + view.FormatBitRate(top) + " · " + m.graphSpan(w-2)),
	}
	return withGraphs(head, tail, graphs, w, h)
}

func diskTitle(d metrics.DiskStat) string {
	if len(d.Volumes) == 0 {
		return d.Name
	}
	mounts := make([]string, len(d.Volumes))
	for i, v := range d.Volumes {
		mounts[i] = v.Mount
	}
	return d.Name + " (" + strings.Join(mounts, " ") + ")"
}

type graphSpec struct {
	title string // optional line above the graph
	vals  []float64
	style lipgloss.Style
}

// withGraphs stacks head, framed graphs sharing the spare height, and tail.
// Graphs that can't get at least one row are dropped. Output is cut to h.
func withGraphs(head, tail []string, graphs []graphSpec, w, h int) []string {
	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8"))
	avail := h - len(head) - len(tail)
	for len(graphs) > 1 && avail/len(graphs) < 4 {
		graphs = graphs[:len(graphs)-1]
	}
	lines := append([]string{}, head...)
	for i, g := range graphs {
		share := avail / len(graphs)
		if i == len(graphs)-1 {
			share = avail - share*(len(graphs)-1)
		}
		gh := share - 2 // frame
		if g.title != "" {
			lines = append(lines, dimStyle.Render(g.title))
			gh--
		}
		graph := frame.Render(g.style.Render(strings.Join(areaGraph(g.vals, w-2, max(1, gh)), "\n")))
		lines = append(lines, strings.Split(graph, "\n")...)
	}
	lines = append(lines, tail...)
	if len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

// scaled converts series to 0–100 against their shared maximum (at least
// min) and returns that maximum.
func scaled(minTop float64, series ...[]float64) ([]float64, float64) {
	top := minTop
	for _, s := range series {
		for _, v := range s {
			top = max(top, v)
		}
	}
	if len(series) == 0 {
		return nil, top
	}
	out := make([]float64, len(series[0]))
	for i, v := range series[0] {
		out[i] = v / top * 100
	}
	return out, top
}

// sumSeries adds two histories element-wise, aligned at the newest value.
func sumSeries(a, b []float64) []float64 {
	if len(b) > len(a) {
		a, b = b, a
	}
	out := append([]float64(nil), a...)
	off := len(a) - len(b)
	for i, v := range b {
		out[off+i] += v
	}
	return out
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
