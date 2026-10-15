package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// pump sends keys and runs the commands they return (here: details fetches),
// feeding results back like Bubble Tea does. Only use it with keys whose
// commands return immediately.
func pump(t *testing.T, m model, keys ...tea.KeyMsg) model {
	t.Helper()
	var run func(cmd tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		switch msg := cmd().(type) {
		case tea.BatchMsg:
			for _, c := range msg {
				run(c)
			}
		case detailsMsg:
			next, more := m.Update(msg)
			m = next.(model)
			run(more)
		}
	}
	for _, k := range keys {
		next, cmd := m.Update(k)
		m = next.(model)
		run(cmd)
	}
	return m
}

func detailsModel(t *testing.T) model {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	return drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func TestDetailsPaneOpensAndFollowsCursor(t *testing.T) {
	m := pump(t, detailsModel(t), keyMsg("enter")) // top row: ollama (highest CPU)
	v := m.View()
	for _, want := range []string{"ollama.exe · PID 4120", `"C:\Program Files\Ollama\ollama.exe" serve`, "Threads 23", "88.0% on GPU 0 · Compute"} {
		if !strings.Contains(v, want) {
			t.Errorf("pane missing %q", want)
		}
	}
	if m.table.rowsHeight() >= m.table.bodyH {
		t.Error("rows should shrink to make room for the pane")
	}
	m = pump(t, m, key("j"))
	if v := m.View(); !strings.Contains(v, "chrome.exe · PID 21248") {
		t.Error("pane should follow the cursor")
	}
}

func TestDetailsIORateAfterTwoReads(t *testing.T) {
	m := pump(t, detailsModel(t), keyMsg("enter"))
	if strings.Contains(m.View(), "I/O") {
		t.Error("one read can't give a rate")
	}
	// A refresh re-reads details while the pane is open.
	next, _ := m.Update(sampleMsgFrom(t, m))
	m = next.(model)
	fetch := m.table.detailsCmd()
	if fetch == nil {
		t.Fatal("refresh should re-read details while the pane is open")
	}
	next, _ = m.Update(fetch())
	m = next.(model)
	if v := m.View(); !strings.Contains(v, "I/O R 2.0 MB/s  W 40.0 KB/s") {
		t.Errorf("I/O rate missing:\n%s", v)
	}
}

func TestEscClosesPaneBeforeClearingSearch(t *testing.T) {
	m := drive(t, detailsModel(t), key("/"), key("c"), keyMsg("enter")) // search for "c"
	m = pump(t, m, keyMsg("enter"))                                     // open the pane
	if !m.table.details.open || m.table.search.value() != "c" {
		t.Fatal("setup: pane open with a search")
	}
	m = drive(t, m, keyMsg("esc"))
	if m.table.details.open || m.table.search.value() != "c" {
		t.Error("first Esc should close only the pane")
	}
	m = drive(t, m, keyMsg("esc"))
	if m.table.search.value() != "" {
		t.Error("second Esc should clear the search")
	}
}

func TestDetailsForGroup(t *testing.T) {
	m := pump(t, detailsModel(t), key("z"), keyMsg("enter"))
	if v := m.View(); !strings.Contains(v, "chrome · 2 processes") || !strings.Contains(v, "PIDs 21248 47768") {
		t.Errorf("group pane:\n%s", v)
	}
}

func TestChildCount(t *testing.T) {
	m := detailsModel(t)
	m.table.procs = append(m.table.procs, goldenProcs()[0])
	m.table.procs[len(m.table.procs)-1].PID, m.table.procs[len(m.table.procs)-1].PPID = 9000, 4120
	if n := m.table.childCount(4120); n != 1 {
		t.Errorf("children of 4120: %d", n)
	}
}

// sampleMsgFrom builds the next sample from the fake sampler.
func sampleMsgFrom(t *testing.T, m model) sampleMsg {
	t.Helper()
	snap, procs, err := m.sampler.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sampleMsg{snap: snap, procs: procs}
}
