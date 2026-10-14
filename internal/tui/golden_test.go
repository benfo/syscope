package tui

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

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
	f := &fakeSampler{}
	m := sampled(t, f)
	m = drive(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	for i := 0; i < 40; i++ {
		snap, procs, _ := f.Sample(context.Background())
		v := float64((i * 37) % 100)
		snap.CPUTotal = v
		snap.MemUsed = uint64(40 + i%20)
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
	}{
		{"processes", nil},
		{"processes_grouped_percore", []tea.KeyMsg{key("z"), key("c")}},
		{"processes_filter_confirm", []tea.KeyMsg{key("/"), key("a"), keyMsg("enter"), key("D")}},
		{"processes_help", []tea.KeyMsg{key("?")}},
		{"perf_cpu", []tea.KeyMsg{key("2")}},
		{"perf_memory", []tea.KeyMsg{key("2"), key("j")}},
		{"perf_disk", []tea.KeyMsg{key("2"), key("j"), key("j")}},
		{"perf_net", []tea.KeyMsg{key("2"), key("j"), key("j"), key("j")}},
		{"perf_gpu", []tea.KeyMsg{key("2"), key("G"), key("k")}},
		{"perf_igpu", []tea.KeyMsg{key("2"), key("G")}},
		{"perf_hidden_adapters", []tea.KeyMsg{key("2"), key("."), key("G")}},
		{"perf_gpu_help", []tea.KeyMsg{key("2"), key("G"), key("k"), key("?")}},
	}
	for _, size := range []struct{ w, h int }{{100, 30}, {70, 16}} {
		for _, s := range screens {
			name := s.name + "_" + strconv.Itoa(size.w) + "x" + strconv.Itoa(size.h)
			t.Run(name, func(t *testing.T) {
				m := goldenModel(t, size.w, size.h)
				for _, k := range s.keys {
					m = drive(t, m, k)
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
