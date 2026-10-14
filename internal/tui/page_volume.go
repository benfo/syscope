package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

// volumePage is one volume (C:, /home, …) of a physical disk that has more
// than one, shown nested under its disk in the sidebar. Active time is a
// property of the physical disk, so it stays on the disk's page.
type volumePage struct {
	m model
	d metrics.DiskStat
	v metrics.VolumeStat
}

// nested is implemented by pages shown indented under another page.
type nested interface{ depth() int }

func (p volumePage) depth() int { return 1 }

func (p volumePage) key() string           { return "vol:" + p.d.ID + ":" + p.v.Mount }
func (p volumePage) name() string          { return p.v.Mount }
func (p volumePage) helpTitle() string     { return p.v.Mount + " · " + p.d.Name }
func (p volumePage) hidden() bool          { return false }
func (p volumePage) style() lipgloss.Style { return ui.disk }

func (p volumePage) record() {
	if p.v.RatesKnown {
		p.m.histories.push(p.key(), "read", p.v.ReadBps)
		p.m.histories.push(p.key(), "write", p.v.WriteBps)
	}
}

func (p volumePage) series(name string) []float64 { return p.m.histories.values(p.key(), name) }

func (p volumePage) trend() []float64 {
	t, _ := scaled(diskRateFloor, sumSeries(p.series("read"), p.series("write")))
	return t
}

func (p volumePage) summary() string {
	if !p.v.RatesKnown {
		return fmt.Sprintf("%.0f%% full", view.Percent(p.v.Used, p.v.Total))
	}
	return view.FormatByteRate(p.v.ReadBps + p.v.WriteBps)
}

func (p volumePage) detail(w, h int) []string {
	v := p.v
	used := view.Percent(v.Used, v.Total)
	tail := []string{"", fmt.Sprintf("%-6s ", "Used") + bar(used, 20) +
		fmt.Sprintf(" %s/%s (%.0f%%)", view.FormatBytesCompact(v.Used), view.FormatBytesCompact(v.Total), used)}

	var graphs []graphSpec
	sub := "Transfer rate · " + p.m.graphSpan(w-2)
	if v.RatesKnown {
		read, readMax := scaled(diskRateFloor, p.series("read"))
		write, writeMax := scaled(diskRateFloor, p.series("write"))
		graphs = []graphSpec{
			{title: "Read · scale " + view.FormatByteRate(readMax), vals: read, style: ui.read},
			{title: "Write · scale " + view.FormatByteRate(writeMax), vals: write, style: ui.write},
		}
		tail = append(tail, packLine([]string{
			kv("Read", view.FormatByteRate(v.ReadBps)), kv("Write", view.FormatByteRate(v.WriteBps)),
		}, statSep, w)...)
	} else {
		sub = "No per-volume transfer rates on this volume"
	}
	tail = append(tail, ui.muted.Render("Active time and response time are on "+p.d.Name+"'s page."))

	head := []string{
		spread(ui.label.Render(p.v.Mount), ui.muted.Render("on "+diskPage{p.m, p.d}.name()), w),
		ui.muted.Render(sub),
	}
	return withGraphs(head, tail, graphs, w, h)
}

func (p volumePage) terms() []term {
	t := []term{
		{"Volume", "A drive letter or mount point on a physical disk. Several volumes can share one disk; their activity adds up to the disk's."},
	}
	if p.v.RatesKnown {
		t = append(t, term{"Read / Write", "Data transferred per second on this volume alone, to see which one is busy. Each graph auto-scales."})
	} else {
		t = append(t, term{"No transfer rates", "The OS doesn't report per-volume counters here: macOS doesn't, and on Windows neither do virtual drives (for example cloud-sync drives) that only appear to live on this disk."})
	}
	return append(t,
		term{"Used", "How full the volume is. Keep SSD volumes below ~90% full; they slow down when nearly full."},
		term{"Active time", "Measured per physical disk, not per volume: see the disk's page above."},
	)
}
