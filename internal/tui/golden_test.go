package tui

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/benfo/syscope/internal/metrics"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// Golden screen tests render every screen from fixed fake data and compare
// it, character for character (colours stripped), with testdata/*.golden.
// They make refactors provably invisible. After an intended visual change,
// regenerate with:  go test ./internal/tui -run Golden -update
var update = flag.Bool("update", false, "rewrite golden files")

// goldenModel is a model fed with a deterministic history of samples.
func goldenModel(t *testing.T, w, h int) model {
	f := &fakeSampler{procs: goldenProcs()}
	m := sampled(t, f)
	m = drive(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	for i := 0; i < 40; i++ {
		snap, procs, _ := f.Sample(context.Background())
		v := float64((i * 37) % 100)
		snap.CPUTotal = v
		snap.MemUsed = uint64(30+i%20) << 30
		snap.Disks[0].ActivePct = float64((i * 13) % 100)
		snap.Disks[0].ReadBps = float64((i * 7 % 10)) * 1e6
		snap.Nets[0].RecvBps = float64((i * 11 % 17)) * 1e5
		snap.GPUs[0].Util = float64((i * 29) % 100)
		m = drive(t, m, sampleMsg{snap: snap, procs: procs})
	}
	return m
}

func TestGoldenScreens(t *testing.T) {
	screens := []struct {
		name string
		keys []tea.KeyMsg
		pump bool // run the keys' commands (details fetches)
	}{
		{"processes", nil, false},
		{"processes_grouped_percore", []tea.KeyMsg{key("z"), key("c")}, false},
		{"processes_filter_confirm", []tea.KeyMsg{key("/"), key("a"), keyMsg("enter"), key("D")}, false},
		{"processes_kill_tree", []tea.KeyMsg{key("j"), key("d")}, false},
		{"processes_suspend_prompt", []tea.KeyMsg{key("p")}, false},
		{"processes_help", []tea.KeyMsg{key("?")}, false},
		{"processes_details", []tea.KeyMsg{keyMsg("enter")}, true},
		{"processes_details_group", []tea.KeyMsg{key("z"), keyMsg("enter")}, true},
		{"processes_details_hint", []tea.KeyMsg{key("s"), key("s"), key("s"), key("j"), keyMsg("enter")}, true},
		{"processes_tree", []tea.KeyMsg{key("t")}, false},
		{"processes_grouped_open", []tea.KeyMsg{key("z"), key("l")}, false},
		{"settings", []tea.KeyMsg{key(","), key("j")}, false},
		{"theme_picker", []tea.KeyMsg{key(","), keyMsg("enter"), key("j"), key("j")}, false},
		{"perf_cpu", []tea.KeyMsg{key("2")}, false},
		{"perf_cpu_blocks", []tea.KeyMsg{key(","), key("j"), key("l"), keyMsg("esc"), key("2")}, false},
		{"perf_memory", []tea.KeyMsg{key("2"), key("j")}, false},
		{"perf_disk", []tea.KeyMsg{key("2"), key("j"), key("j")}, false},
		{"perf_volume", []tea.KeyMsg{key("2"), key("j"), key("j"), key("j")}, false},
		{"perf_net", []tea.KeyMsg{key("2"), key("j"), key("j"), key("j")}, false},
		{"perf_gpu", []tea.KeyMsg{key("2"), key("G"), key("k")}, false},
		{"perf_igpu", []tea.KeyMsg{key("2"), key("G")}, false},
		{"perf_hidden_adapters", []tea.KeyMsg{key("2"), key("."), key("G")}, false},
		{"perf_gpu_help", []tea.KeyMsg{key("2"), key("G"), key("k"), key("?")}, false},
		{"ports", []tea.KeyMsg{key("3")}, false},
		{"ports_all_sorted", []tea.KeyMsg{key("3"), key("."), key("s"), key("s"), key("s")}, false},
		{"ports_filter_confirm", []tea.KeyMsg{key("3"), key("/"), key("o"), key("l"), keyMsg("enter"), key("d")}, false},
		{"ports_help", []tea.KeyMsg{key("3"), key("?")}, false},
		{"ports_goto_process", []tea.KeyMsg{key("3"), key("G"), keyMsg("enter")}, true},
	}
	for _, size := range []struct{ w, h int }{{100, 30}, {70, 16}} {
		for _, s := range screens {
			name := s.name + "_" + strconv.Itoa(size.w) + "x" + strconv.Itoa(size.h)
			t.Run(name, func(t *testing.T) {
				t.Cleanup(func() { applyTheme(DefaultTheme) })
				m := goldenModel(t, size.w, size.h)
				if s.pump {
					m = pump(t, m, s.keys...)
				} else {
					for _, k := range s.keys {
						m = drive(t, m, k)
					}
				}
				got := ansi.Strip(m.View())
				path := filepath.Join("testdata", name+".golden")
				if *update {
					if err := os.MkdirAll("testdata", 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("missing golden file (run with -update): %v", err)
				}
				if got != string(want) {
					t.Errorf("screen changed; diff the output below with %s\n%s", path, firstDiff(string(want), got))
				}
			})
		}
	}
}

// firstDiff shows the first differing line, which is usually enough to see
// what moved.
func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < max(len(w), len(g)); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return "line " + strconv.Itoa(i+1) + ":\nwant: " + wl + "\ngot:  " + gl
		}
	}
	return "(no line differs; trailing content changed)"
}

// goldenProcs covers the interesting row cases: a group, a long name, a
// protected process with unknown values, and GPU users.
func goldenProcs() []metrics.ProcInfo {
	gib := uint64(1 << 30)
	return []metrics.ProcInfo{
		{PID: 4, Name: "System", User: "SYSTEM"},
		{PID: 4120, PPID: 4, Name: "ollama.exe", User: `CORP\sam`, CreateTime: fakeNow().Add(-125 * time.Minute), CPU: 2.5, CPUKnown: true, RSS: 1 * gib, RSSKnown: true,
			GPU: 88, GPUKnown: true, GPUEngine: "GPU 0 · Compute", VRAM: 7 * gib, VRAMKnown: true},
		{PID: 21248, Name: "chrome.exe", User: `CORP\sam`, CPU: 1.9, CPUKnown: true, RSS: 535 << 20, RSSKnown: true,
			GPU: 0.4, GPUKnown: true, VRAM: 180 << 20, VRAMKnown: true},
		{PID: 47768, PPID: 21248, Name: "chrome.exe", User: `CORP\sam`, CPU: 0.7, CPUKnown: true, RSS: 175 << 20, RSSKnown: true,
			GPUKnown: true, VRAMKnown: true},
		{PID: 55916, Name: "Example.TextEditor.Language.ServiceHost.exe", User: `CORP\sam`, CPU: 0.1, CPUKnown: true,
			RSS: 542 << 20, RSSKnown: true, GPUKnown: true, VRAMKnown: true},
		{PID: 21772, Name: "SongPlayer.exe", Status: "stop", CPU: 0, CPUKnown: true, RSS: 187 << 20, RSSKnown: true,
			GPU: 0, GPUKnown: true, VRAM: 4400 << 20, VRAMKnown: true},
		{PID: 72244, Name: "Meet.exe", User: `CORP\sam`, CPU: 1.2, CPUKnown: true, RSS: 1 * gib, RSSKnown: true,
			GPU: 3.1, GPUKnown: true, VRAM: 412 << 20, VRAMKnown: true},
	}
}

// goldenSockets are listeners and connections of goldenProcs, plus one
// socket whose owner the OS didn't report.
func goldenSockets() []metrics.Socket {
	return []metrics.Socket{
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 11434, State: "LISTEN", PID: 4120},
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 445, State: "LISTEN", PID: 4},
		{Proto: "TCP", LocalIP: "::", LocalPort: 445, State: "LISTEN", PID: 4},
		{Proto: "UDP", LocalIP: "0.0.0.0", LocalPort: 5353, PID: 21248},
		{Proto: "UDP", LocalIP: "0.0.0.0", LocalPort: 3478, PID: 72244},
		{Proto: "TCP", LocalIP: "0.0.0.0", LocalPort: 7680, State: "LISTEN"},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52144, RemoteIP: "203.0.113.20", RemotePort: 443,
			State: "ESTABLISHED", PID: 21248},
		{Proto: "TCP", LocalIP: "127.0.0.1", LocalPort: 52150, RemoteIP: "127.0.0.1", RemotePort: 11434,
			State: "ESTABLISHED", PID: 55916},
		{Proto: "TCP", LocalIP: "192.0.2.106", LocalPort: 52101, RemoteIP: "198.51.100.9", RemotePort: 443,
			State: "TIME_WAIT"},
	}
}
