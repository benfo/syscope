package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/benfo/syscope/internal/insight"
	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

// detailsFetcher reads on-demand details for one process.
type detailsFetcher func(ctx context.Context, pid int32) (metrics.ProcDetails, error)

// detailsPane is the bottom split opened with Enter on the Processes tab. It
// follows the cursor and refreshes with every sample.
type detailsPane struct {
	open  bool
	fetch detailsFetcher
	now   func() time.Time // injectable so screens are reproducible in tests

	cur, prev metrics.ProcDetails // latest two reads, for I/O rates
}

// detailsMsg delivers a details read back to the model.
type detailsMsg struct {
	d   metrics.ProcDetails
	err error
}

// The pane has a fixed height (smaller on short terminals) so the list
// doesn't jump as the cursor moves between processes with more or less detail.
const (
	minPaneH = 5
	maxPaneH = 9
)

// paneHeight is the details pane's share of the body, 0 when closed.
func (t processTable) paneHeight() int {
	if !t.details.open {
		return 0
	}
	return min(maxPaneH, max(minPaneH, t.bodyH/2), t.bodyH-1)
}

// rowsHeight is how many process rows fit above the pane.
func (t processTable) rowsHeight() int { return max(1, t.bodyH-t.paneHeight()) }

// detailsCmd fetches details for the selected process when the pane is open.
// Group rows show a summary instead, so they need no fetch.
func (t processTable) detailsCmd() tea.Cmd {
	r, ok := t.selected()
	if !t.details.open || !ok || r.Count != 1 || t.details.fetch == nil {
		return nil
	}
	fetch, pid := t.details.fetch, r.PID
	return func() tea.Msg {
		d, err := fetch(context.Background(), pid)
		return detailsMsg{d, err}
	}
}

// setDetails stores a read, keeping the previous one for the same process
// so I/O rates can be computed.
func (t *processTable) setDetails(msg detailsMsg) {
	if msg.err != nil {
		return
	}
	if msg.d.PID == t.details.cur.PID {
		t.details.prev = t.details.cur
	} else {
		t.details.prev = metrics.ProcDetails{}
	}
	t.details.cur = msg.d
}

// proc finds the latest sample for pid.
func (t processTable) proc(pid int32) (metrics.ProcInfo, bool) {
	for _, p := range t.procs {
		if p.PID == pid {
			return p, true
		}
	}
	return metrics.ProcInfo{}, false
}

// renderDetails draws the pane: a title rule and the selected row's details.
func (t processTable) renderDetails(w int) string {
	h := t.paneHeight()
	r, ok := t.selected()
	var title string
	var lines []string
	switch {
	case !ok:
		title, lines = "No process selected", nil
	case r.Count > 1:
		title, lines = fmt.Sprintf("%s · %d processes", r.Name, r.Count), t.groupLines(r, w)
	default:
		title, lines = fmt.Sprintf("%s · PID %d", r.Name, r.PID), t.processLines(r, w)
	}
	rule := ui.muted.Render("─ ") + ui.label.Render(title) + " "
	out := []string{rule + ui.muted.Render(strings.Repeat("─", max(0, w-lipgloss.Width(rule))))}
	for _, l := range lines {
		out = append(out, " "+l)
	}
	var b strings.Builder
	for i := 0; i < h; i++ {
		var l string
		if i < len(out) {
			l = out[i]
		}
		b.WriteString(padANSI(l, w) + "\n")
	}
	return b.String()
}

func (t processTable) processLines(r view.Row, w int) []string {
	p, _ := t.proc(r.PID)
	d := t.details.cur
	if d.PID != r.PID {
		d = metrics.ProcDetails{} // not read yet
	}
	orDash := func(s string) string {
		if s == "" {
			return ui.muted.Render("--")
		}
		return s
	}
	lines := []string{
		kv("Path", orDash(p.Exe)),
		kv("Command", orDash(d.Cmdline)),
	}

	var who []string
	parent := "--"
	if p.PPID != 0 {
		parent = strconv.Itoa(int(p.PPID))
		if pp, ok := t.proc(p.PPID); ok {
			parent = pp.Name + " (" + parent + ")"
		}
	}
	who = append(who, kv("Parent", parent), kv("Children", strconv.Itoa(t.childCount(r.PID))))
	if p.User != "" {
		who = append(who, kv("User", view.ShortUser(p.User)))
	}
	if !p.CreateTime.IsZero() {
		now := t.details.now()
		who = append(who, kv("Started", p.CreateTime.Format("15:04")+" ("+view.FormatUptime(now.Sub(p.CreateTime))+" ago)"))
	}
	if d.ThreadsKnown {
		who = append(who, kv("Threads", strconv.Itoa(int(d.Threads))))
	}
	if d.HandlesKnown {
		who = append(who, kv(d.HandlesName, strconv.Itoa(int(d.Handles))))
	}
	lines = append(lines, packLine(who, statSep, w-1)...)

	use := []string{
		kv("CPU", view.FormatCPU(p.CPU, p.CPUKnown, t.numCPU, t.perCoreCPU)+"%"),
		kv("Memory", view.FormatMem(p.RSS, p.RSSKnown)),
	}
	if rd, wr, ok := metrics.IORates(t.details.prev, d); ok {
		use = append(use, kv("I/O", "R "+view.FormatByteRate(rd)+"  W "+view.FormatByteRate(wr)))
	}
	if t.showGPU {
		gpu := view.FormatGPU(p.GPU, p.GPUKnown) + "%"
		if p.GPUEngine != "" {
			gpu += " on " + p.GPUEngine
		}
		use = append(use, kv("GPU", gpu), kv("VRAM", view.FormatMem(p.VRAM, p.VRAMKnown)))
	}
	lines = append(lines, packLine(use, statSep, w-1)...)
	return append(lines, hintLines(insight.For(insight.Context{
		Proc: p, Details: d, SameName: t.sameName(p.Name), NumCPU: t.numCPU, MemTotal: t.memTotal,
	}), w-1)...)
}

func (t processTable) groupLines(r view.Row, w int) []string {
	const shown = 16
	pids := make([]string, 0, shown)
	for i, pid := range r.PIDs {
		if i == shown {
			pids = append(pids, fmt.Sprintf("… +%d", len(r.PIDs)-shown))
			break
		}
		pids = append(pids, strconv.Itoa(int(pid)))
	}
	total := []string{
		kv("CPU", view.FormatCPU(r.CPU, r.CPUKnown, t.numCPU, t.perCoreCPU)+"%"),
		kv("Memory", view.FormatMem(r.RSS, r.RSSKnown)),
	}
	if t.showGPU {
		total = append(total, kv("GPU", view.FormatGPU(r.GPU, r.GPUKnown)+"%"), kv("VRAM", view.FormatMem(r.VRAM, r.VRAMKnown)))
	}
	lines := packLine([]string{kv("PIDs", strings.Join(pids, " "))}, statSep, w-1)
	lines = append(lines, ui.muted.Render("Totals for the group:"))
	lines = append(lines, packLine(total, statSep, w-1)...)
	// The group's totals behave like one big process for hints.
	group := metrics.ProcInfo{Name: r.Name, CPU: r.CPU, CPUKnown: r.CPUKnown, RSS: r.RSS, RSSKnown: r.RSSKnown,
		GPU: r.GPU, GPUKnown: r.GPUKnown, VRAM: r.VRAM, VRAMKnown: r.VRAMKnown}
	return append(lines, hintLines(insight.For(insight.Context{Proc: group, NumCPU: t.numCPU, MemTotal: t.memTotal}), w-1)...)
}

// childCount is how many processes have pid as their parent.
func (t processTable) childCount(pid int32) int {
	n := 0
	for _, p := range t.procs {
		if p.PPID == pid && p.PID != pid {
			n++
		}
	}
	return n
}

// sameName counts processes with this executable name.
func (t processTable) sameName(name string) int {
	n := 0
	for _, p := range t.procs {
		if view.MatchesName(p.Name, name) {
			n++
		}
	}
	return n
}

// hintLines renders insight hints, wrapped to w: ℹ explains, ⚠ suggests
// acting.
func hintLines(hints []insight.Hint, w int) []string {
	var out []string
	for _, h := range hints {
		icon := ui.accent.Render("ℹ ")
		if h.Level == insight.Warn {
			icon = ui.warn.Render("⚠ ")
		}
		for i, l := range strings.Split(ansi.Wordwrap(h.Text, max(10, w-2), ""), "\n") {
			if i == 0 {
				out = append(out, icon+l)
			} else {
				out = append(out, "  "+l)
			}
		}
	}
	return out
}
