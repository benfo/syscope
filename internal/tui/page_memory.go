package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/benfo/syscope/internal/view"
)

const memoryKey = "mem"

type memoryPage struct{ m model }

func memoryPages(m model) []devicePage { return []devicePage{memoryPage{m}} }

func (p memoryPage) key() string           { return memoryKey }
func (p memoryPage) name() string          { return "Memory" }
func (p memoryPage) helpTitle() string     { return "Memory" }
func (p memoryPage) hidden() bool          { return false }
func (p memoryPage) style() lipgloss.Style { return ui.memory }
func (p memoryPage) trend() []float64      { return p.m.histories.values(memoryKey, "used") }

func (p memoryPage) record() {
	if s := p.m.snap; s.MemTotal > 0 {
		p.m.histories.push(memoryKey, "used", view.Percent(s.MemUsed, s.MemTotal))
	}
}

func (p memoryPage) summary() string {
	s := p.m.snap
	return fmt.Sprintf("%s/%s (%.0f%%)", view.FormatBytesCompact(s.MemUsed), view.FormatBytesCompact(s.MemTotal),
		view.Percent(s.MemUsed, s.MemTotal))
}

func (p memoryPage) detail(w, h int) []string {
	s := p.m.snap
	stats := []string{
		kv("In use", fmt.Sprintf("%s (%.0f%%)", view.FormatBytes(s.MemUsed), view.Percent(s.MemUsed, s.MemTotal))),
		kv("Available", view.FormatBytes(s.MemAvailable)),
	}
	if s.MemCached > 0 {
		stats = append(stats, kv("Cached", view.FormatBytes(s.MemCached)))
	}
	if s.CommitLimit > 0 {
		stats = append(stats, kv("Committed", view.FormatBytesCompact(s.CommitUsed)+"/"+view.FormatBytesCompact(s.CommitLimit)))
	}
	stats = append(stats, kv("Swap", view.FormatBytesCompact(s.SwapUsed)+"/"+view.FormatBytesCompact(s.SwapTotal)))

	head := []string{
		spread(ui.label.Render("Memory"), ui.muted.Render(view.FormatBytes(s.MemTotal)), w),
		ui.muted.Render("% In use · " + p.m.graphSpan(w-2)),
	}
	tail := []string{"", ui.muted.Render("Composition"), p.composition(w), ""}
	tail = append(tail, packLine(stats, statSep, w)...)
	return withGraphs(head, tail, []graphSpec{{vals: p.trend(), style: ui.memory}}, w, h)
}

// composition is a single bar: in use | cached | free.
func (p memoryPage) composition(w int) string {
	s := p.m.snap
	if s.MemTotal == 0 || w <= 0 {
		return ""
	}
	used := int(float64(w) * float64(s.MemUsed) / float64(s.MemTotal))
	cached := min(w-used, int(float64(w)*float64(s.MemCached)/float64(s.MemTotal)))
	free := max(0, w-used-cached)
	return ui.memory.Render(strings.Repeat("█", used)) +
		ui.cached.Render(strings.Repeat("▓", cached)) +
		ui.muted.Render(strings.Repeat("░", free))
}

func (p memoryPage) terms() []term {
	s := p.m.snap
	t := []term{
		{"In use", "RAM used by programs and the OS. It doesn't count cache that can be freed instantly."},
		{"Available", "RAM that programs can get right now without anything being moved to disk. Below ~10% of total, the system starts swapping and slows down."},
	}
	if s.MemCached > 0 {
		t = append(t, term{"Cached", "Recently used file data kept in RAM to make it faster to open again. It's given back instantly when programs need it, so a big cache is good, not wasted."})
	}
	if s.CommitLimit > 0 {
		t = append(t, term{"Committed", "Memory programs have reserved, which Windows promises to back with RAM or the page file. " +
			"Near the limit, Windows grows the page file; if it can't, programs fail with \"out of memory\" even while RAM shows free."})
	}
	return append(t,
		term{"Swap", "Data moved out of RAM to disk (the page file on Windows). Some is normal; steady growth while you work means you need more RAM."},
		term{"Composition", "The bar shows in-use memory, then cache (if reported), then free."},
	)
}
