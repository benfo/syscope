package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// diskRateFloor keeps idle disks from showing noise as full-height graphs.
const diskRateFloor = 1_000_000 // 1 MB/s

type diskPage struct {
	m model
	d metrics.DiskStat
}

// diskPages lists each physical disk followed by its volumes (only when it
// has more than one; a single volume would just repeat the disk).
func diskPages(m model) []devicePage {
	var out []devicePage
	for _, d := range m.snap.Disks {
		out = append(out, diskPage{m, d})
		if len(d.Volumes) > 1 {
			for _, v := range d.Volumes {
				out = append(out, volumePage{m, d, v})
			}
		}
	}
	return out
}

func (p diskPage) key() string           { return "disk:" + p.d.ID }
func (p diskPage) helpTitle() string     { return p.name() }
func (p diskPage) hidden() bool          { return false }
func (p diskPage) style() lipgloss.Style { return ui.disk }

// name is "Disk 0 (C: G:)": the disk and the volumes on it.
func (p diskPage) name() string {
	if len(p.d.Volumes) == 0 {
		return p.d.Name
	}
	mounts := make([]string, len(p.d.Volumes))
	for i, v := range p.d.Volumes {
		mounts[i] = v.Mount
	}
	return p.d.Name + " (" + strings.Join(mounts, " ") + ")"
}

func (p diskPage) record() {
	if p.d.RatesKnown {
		h := p.m.histories
		h.push(p.key(), "active", p.d.ActivePct)
		h.push(p.key(), "read", p.d.ReadBps)
		h.push(p.key(), "write", p.d.WriteBps)
	}
}

func (p diskPage) series(name string) []float64 { return p.m.histories.values(p.key(), name) }

// trend is active time where the OS reports it, otherwise transfer rate.
func (p diskPage) trend() []float64 {
	if p.d.ActiveKnown {
		return p.series("active")
	}
	t, _ := scaled(diskRateFloor, sumSeries(p.series("read"), p.series("write")))
	return t
}

func (p diskPage) summary() string {
	s := view.FormatByteRate(p.d.ReadBps + p.d.WriteBps)
	if p.d.ActiveKnown {
		s = fmt.Sprintf("%.0f%%  %s", p.d.ActivePct, s)
	}
	return s
}

func (p diskPage) detail(w, h int) []string {
	d := p.d
	desc := d.Model
	if d.Bus != "" {
		desc = strings.TrimSpace(desc + " · " + d.Bus)
	}
	read, readMax := scaled(diskRateFloor, p.series("read"))
	write, writeMax := scaled(diskRateFloor, p.series("write"))

	var graphs []graphSpec
	sub := "Transfer rate · " + p.m.graphSpan(w-2)
	if d.ActiveKnown {
		sub = "% Active time · " + p.m.graphSpan(w-2)
		graphs = append(graphs, graphSpec{vals: p.series("active"), style: ui.disk})
	}
	graphs = append(graphs,
		graphSpec{title: "Read · scale " + view.FormatByteRate(readMax), vals: read, style: ui.read},
		graphSpec{title: "Write · scale " + view.FormatByteRate(writeMax), vals: write, style: ui.write},
	)

	var stats []string
	if d.ActiveKnown {
		stats = append(stats, kv("Active", fmt.Sprintf("%.0f%%", d.ActivePct)))
	}
	stats = append(stats, kv("Read", view.FormatByteRate(d.ReadBps)), kv("Write", view.FormatByteRate(d.WriteBps)))
	if d.RespKnown {
		stats = append(stats, kv("Response", fmt.Sprintf("%.2f ms", d.RespMs)))
	}
	tail := append([]string{""}, packLine(stats, statSep, w)...)
	for _, v := range d.Volumes {
		pct := view.Percent(v.Used, v.Total)
		tail = append(tail, fmt.Sprintf("%-6s ", v.Mount)+bar(pct, 20)+
			fmt.Sprintf(" %s/%s (%.0f%%)", view.FormatBytesCompact(v.Used), view.FormatBytesCompact(v.Total), pct))
	}
	if note := virtualDrivesNote(p.m.snap.VirtualDrives); note != "" {
		for _, l := range strings.Split(ansi.Wordwrap(note, w, ""), "\n") {
			tail = append(tail, ui.muted.Render(l))
		}
	}
	head := []string{spread(ui.label.Render(p.name()), ui.muted.Render(desc), w), ui.muted.Render(sub)}
	return withGraphs(head, tail, graphs, w, h)
}

func (p diskPage) terms() []term {
	var t []term
	if p.d.ActiveKnown {
		t = append(t, term{"Active time", "Share of time the disk was handling at least one request. Near 100% means the disk is the bottleneck, " +
			"even if MB/s looks low (lots of small random reads)."})
	} else {
		t = append(t, term{"Active time", "Not available on this OS; the graph shows transfer rate instead."})
	}
	t = append(t, term{"Read / Write", "Data transferred per second. Rough maximums: NVMe SSD 2,000–7,000 MB/s, SATA SSD ~550 MB/s, hard drive ~150 MB/s. " +
		"Each graph auto-scales; the scale above it is what full height means."})
	if p.d.RespKnown {
		t = append(t, term{"Response", "Average time per read or write. Under 1 ms is normal for NVMe; above ~20 ms feels sluggish; " +
			"hundreds of ms means something is hammering the disk, or it's failing."})
	}
	t = append(t, term{"Volumes", "Drives on this disk and how full they are. Keep SSDs below ~90% full; they slow down when nearly full."})
	if len(p.m.snap.VirtualDrives) > 0 {
		t = append(t, term{"Virtual drives", "Drive letters created by software, such as cloud-sync drives, rather than disk partitions. " +
			"They report sizes that aren't real disk space, so they aren't listed as volumes. Reading and writing their files is disk activity on the disk they cache to."})
	}
	return t
}

// virtualDrivesNote explains why drive letters such as a cloud-sync drive
// aren't listed under the disk: "Virtual drives (G: Google Drive) aren't
// disks; …".
func virtualDrivesNote(drives []metrics.VirtualDrive) string {
	if len(drives) == 0 {
		return ""
	}
	names := make([]string, len(drives))
	for i, d := range drives {
		names[i] = strings.TrimSpace(d.Mount + " " + d.Label)
	}
	return "Virtual drives (" + strings.Join(names, ", ") + ") aren't disks; their file activity shows on the disk they cache to."
}
