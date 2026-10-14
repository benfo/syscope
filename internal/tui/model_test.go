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

type fakeSampler struct {
	procs []metrics.ProcInfo
	nets  []metrics.NetStat
}

func (f *fakeSampler) Sample(context.Context) (metrics.SystemSnapshot, []metrics.ProcInfo, error) {
	return metrics.SystemSnapshot{NumCPU: 4, CPUKnown: true, CPUTotal: 40, CPUPerCore: []float64{10, 90, 40, 20},
		MemTotal: 100, MemUsed: 50, MemCached: 10, ProcCount: len(f.procs),
		Disks: []metrics.DiskStat{{ID: "0", Name: "Disk 0", Model: "Test SSD", Bus: "NVMe", RatesKnown: true,
			ReadBps: 5e6, WriteBps: 1e6, ActivePct: 30, ActiveKnown: true, RespMs: 0.2, RespKnown: true,
			Volumes: []metrics.VolumeStat{{Mount: "C:", Total: 1000, Used: 400}}}},
		Nets: f.netsOrDefault()}, f.procs, nil
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
			for _, dev := range []string{"up", "down", "down", "down", "down"} {
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

func (f *fakeSampler) netsOrDefault() []metrics.NetStat {
	if f.nets != nil {
		return f.nets
	}
	return []metrics.NetStat{
		{Name: "Wi-Fi", Desc: "Test Wi-Fi", Physical: true, Up: true, Default: true, RatesKnown: true,
			RecvBps: 250_000, SentBps: 20_000, SpeedBps: 866_700_000, Addrs: []string{"192.0.2.106"}},
		{Name: "VPN", Desc: "Virtual adapter", Up: true, RatesKnown: true},
	}
}

func resample(t *testing.T, m model, f *fakeSampler) model {
	snap, procs, _ := f.Sample(context.Background())
	return drive(t, m, sampleMsg{snap: snap, procs: procs})
}

func TestPerfDisksAndNets(t *testing.T) {
	f := &fakeSampler{}
	m := sampled(t, f)
	m = drive(t, m, key("2"), keyMsg("down"), keyMsg("down"))
	if v := m.View(); !strings.Contains(v, "Disk 0 (C:)") || !strings.Contains(v, "Test SSD · NVMe") ||
		!strings.Contains(v, "Response") || !strings.Contains(v, "0.20 ms") {
		t.Error("disk page incomplete")
	}
	m = drive(t, m, keyMsg("down"))
	if v := m.View(); !strings.Contains(v, "Test Wi-Fi") || !strings.Contains(v, "2.0 Mbps") ||
		!strings.Contains(v, "192.0.2.106") {
		t.Error("network page incomplete")
	}
	if devs := m.perfDevices(); len(devs) != 4 {
		t.Errorf("hidden VPN should not be listed by default: %d devices", len(devs))
	}
	m = drive(t, m, key("a"))
	if devs := m.perfDevices(); len(devs) != 5 {
		t.Errorf("a should show all adapters: %d devices", len(devs))
	}

	// A new adapter sorting before Wi-Fi must not move the selection.
	f.nets = append([]metrics.NetStat{{Name: "Ethernet", Physical: true, Up: true, Default: true}}, f.netsOrDefault()...)
	m = resample(t, m, f)
	if sel, _ := m.selectedDevice(m.perfDevices()); sel.key != "net:Wi-Fi" {
		t.Errorf("selection moved to %s", sel.key)
	}
}

func TestStripShowsDiskAndNet(t *testing.T) {
	v := sampled(t, &fakeSampler{}).renderStrip()
	if !strings.Contains(v, "DISK") || !strings.Contains(v, "30%") || !strings.Contains(v, "↓2.0 Mbps") {
		t.Errorf("strip missing disk/net: %q", v)
	}
}

func TestScaledAndSumSeries(t *testing.T) {
	got, top := scaled(10, []float64{5, 20}, []float64{40})
	if top != 40 || got[0] != 12.5 || got[1] != 50 {
		t.Errorf("scaled: %v %v", got, top)
	}
	if _, top := scaled(10, []float64{1}); top != 10 {
		t.Errorf("floor not applied: %v", top)
	}
	if s := sumSeries([]float64{1, 2, 3}, []float64{10, 20}); s[0] != 1 || s[1] != 12 || s[2] != 23 {
		t.Errorf("sumSeries aligned wrong: %v", s)
	}
}
