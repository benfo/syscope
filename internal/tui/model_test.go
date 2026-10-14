package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"taskman/internal/metrics"
)

type fakeSampler struct{ procs []metrics.ProcInfo }

func (f *fakeSampler) Sample(context.Context) (metrics.SystemSnapshot, []metrics.ProcInfo, error) {
	return metrics.SystemSnapshot{NumCPU: 4, CPUKnown: true, CPUTotal: 40, CPUPerCore: []float64{10, 90, 40, 20},
		MemTotal: 100, MemUsed: 50, MemCached: 10, ProcCount: len(f.procs)}, f.procs, nil
}

func p(pid int32, name string, cpu float64) metrics.ProcInfo {
	return metrics.ProcInfo{PID: pid, Name: name, CPU: cpu, CPUKnown: true, RSS: uint64(pid), RSSKnown: true}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drive feeds messages through Update, running any commands that complete
// synchronously (samples, kills) but not ticks.
func drive(t *testing.T, m model, msgs ...tea.Msg) model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

func sampled(t *testing.T, f *fakeSampler) model {
	m := newModel(Options{Refresh: time.Second, Sampler: f, CPU: metrics.CPUInfo{
		Model: "Test CPU", Logical: 4, Physical: 4, BaseMHz: 2700,
		Kinds: []metrics.CoreKind{metrics.CorePerformance, metrics.CorePerformance, metrics.CoreEfficiency, metrics.CoreEfficiency},
	}})
	snap, procs, _ := f.Sample(context.Background())
	return drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 20}, sampleMsg{snap: snap, procs: procs})
}

func TestSelectionFollowsProcessAcrossRefresh(t *testing.T) {
	f := &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 30), p(2, "b", 20), p(3, "c", 10)}}
	m := sampled(t, f)
	m = drive(t, m, key("down")) // select b (PID 2)
	if r, _ := m.selected(); r.PID != 2 {
		t.Fatalf("selected PID %d, want 2", r.PID)
	}

	f.procs = []metrics.ProcInfo{p(1, "a", 5), p(2, "b", 50), p(3, "c", 10)} // b jumps to top
	snap, procs, _ := f.Sample(context.Background())
	m = drive(t, m, sampleMsg{snap: snap, procs: procs})
	if r, _ := m.selected(); r.PID != 2 || m.cursor != 0 {
		t.Errorf("selection lost: PID %d at cursor %d", r.PID, m.cursor)
	}
}

func TestCursorStaysOnTopUntilUserMoves(t *testing.T) {
	f := &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 0), p(2, "b", 0)}}
	m := sampled(t, f) // ties sort by PID: a is on top

	f.procs = []metrics.ProcInfo{p(1, "a", 0), p(2, "b", 9)}
	snap, procs, _ := f.Sample(context.Background())
	m = drive(t, m, sampleMsg{snap: snap, procs: procs})
	if r, _ := m.selected(); r.PID != 2 {
		t.Errorf("cursor should stay on the top row, got PID %d", r.PID)
	}
}

func TestStaleTickIgnored(t *testing.T) {
	m := sampled(t, &fakeSampler{})
	_, cmd := m.Update(tickMsg{seq: m.seq - 1})
	if cmd != nil {
		t.Error("stale tick should not start a sample")
	}
	_, cmd = m.Update(tickMsg{seq: m.seq})
	if cmd == nil {
		t.Error("current tick should start a sample")
	}
}

func TestFilterAndGroup(t *testing.T) {
	f := &fakeSampler{procs: []metrics.ProcInfo{p(1, "chrome.exe", 1), p(2, "chrome.exe", 2), p(3, "code.exe", 1)}}
	m := sampled(t, f)
	m = drive(t, m, key("/"), key("c"), key("h"), key("enter"))
	if len(m.rows) != 2 || m.filtering {
		t.Fatalf("filter: %d rows, filtering=%v", len(m.rows), m.filtering)
	}
	m = drive(t, m, key("g"))
	if len(m.rows) != 1 || m.rows[0].Count != 2 {
		t.Fatalf("group: %+v", m.rows)
	}
	m = drive(t, m, key("esc"), key("g"))
	if len(m.rows) != 3 {
		t.Errorf("esc should clear filter: %d rows", len(m.rows))
	}
}

func TestKillNeedsConfirmation(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 1)}})
	m = drive(t, m, key("K"))
	if m.confirm == nil || !m.confirm.force {
		t.Fatal("K should ask to confirm a force kill")
	}
	if !strings.Contains(m.View(), "Force kill a (PID 1)? [y/N]") {
		t.Error("confirmation prompt not shown")
	}
	next, cmd := m.Update(key("n"))
	m = next.(model)
	if cmd != nil || m.confirm != nil {
		t.Error("n should cancel without killing")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	var procs []metrics.ProcInfo
	for i := int32(1); i <= 50; i++ {
		procs = append(procs, p(i, strings.Repeat("x", 80), 1))
	}
	m := sampled(t, &fakeSampler{procs: procs})
	for _, size := range [][2]int{{100, 20}, {60, 12}, {160, 45}} {
		m = drive(t, m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, tab := range []string{"1", "2"} {
			m = drive(t, m, key(tab))
			for _, dev := range []string{"up", "down"} {
				m = drive(t, m, keyMsg(dev))
				lines := strings.Split(m.View(), "\n")
				if len(lines) != m.height {
					t.Errorf("%v tab %s %s: %d lines, want %d", size, tab, dev, len(lines), m.height)
				}
				for i, l := range lines {
					if w := lipgloss.Width(l); w > m.width {
						t.Errorf("%v tab %s line %d: width %d > %d", size, tab, i, w, m.width)
					}
				}
			}
		}
	}
}

func TestTabs(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: []metrics.ProcInfo{p(1, "a", 1)}})
	m = drive(t, m, key("2"))
	if m.tab != tabPerformance || !strings.Contains(m.View(), "Test CPU") {
		t.Fatal("2 should show the Performance tab")
	}
	if v := m.View(); !strings.Contains(v, "2P + 2E") || !strings.Contains(v, "2.70 GHz") {
		t.Error("CPU detail missing core split or base speed")
	}
	m = drive(t, m, keyMsg("down"))
	if !strings.Contains(m.View(), "Composition") {
		t.Error("down should select Memory")
	}
	m = drive(t, m, key("K")) // process keys are inert on the Performance tab
	if m.confirm != nil {
		t.Error("kill must not trigger from the Performance tab")
	}
	m = drive(t, m, keyMsg("tab"))
	if m.tab != tabProcesses {
		t.Error("tab should cycle back to Processes")
	}
	m = drive(t, m, key("/"), key("2"))
	if m.tab != tabProcesses || m.filter.Value() != "2" {
		t.Error("digits typed into the filter must not switch tabs")
	}
}

func TestAreaGraph(t *testing.T) {
	got := areaGraph([]float64{100, 50, 0, 6.25}, 5, 2)
	want := []string{" █   ", " ██ ▁"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	}
	return key(s)
}
