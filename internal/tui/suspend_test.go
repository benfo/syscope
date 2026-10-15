package tui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"taskman/internal/metrics"
	"taskman/internal/view"
)

func selectPID(t *testing.T, m model, pid int32) model {
	t.Helper()
	for i, r := range m.table.rows {
		if r.PID == pid && r.Count == 1 {
			m.table.move(i - m.table.scroll.cursor)
			return m
		}
	}
	t.Fatalf("PID %d not listed", pid)
	return m
}

func TestSuspendAsksResumeDoesnt(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, selectPID(t, m, 4120), key("p"))
	sp, ok := m.confirm.(suspendPrompt)
	if !ok || sp.text() != "Suspend ollama.exe (PID 4120)? [y/N]" {
		t.Fatalf("p should ask before suspending: %#v", m.confirm)
	}
	if sp.respond("n") != nil {
		t.Error("only y suspends")
	}

	m = drive(t, m, key("n"), key("G")) // SongPlayer.exe is suspended in the fixture
	m = selectPID(t, m, 21772)
	m, cmd := m.toggleSuspend()
	if m.confirm != nil || cmd == nil {
		t.Error("p on a suspended process should resume it without asking")
	}
}

func TestCantSuspendTaskmanOrItsTerminal(t *testing.T) {
	procs := append(goldenProcs(),
		metrics.ProcInfo{PID: 900, Name: "terminal"},
		metrics.ProcInfo{PID: 901, PPID: 900, Name: "shell"},
		metrics.ProcInfo{PID: 902, PPID: 901, Name: "taskman"})
	m := sampled(t, &fakeSampler{procs: procs})
	m.selfPID = 902
	for _, pid := range []int32{900, 902} {
		got := drive(t, selectPID(t, m, pid), key("p"))
		if got.confirm != nil || got.status == "" {
			t.Errorf("PID %d: must refuse to suspend taskman or its ancestors", pid)
		}
	}
}

func TestQuitOffersToResume(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	if _, cmd := m.quit(); cmd == nil {
		t.Fatal("nothing suspended: q should quit")
	}
	// Pretend this session suspended SongPlayer.exe (suspended in the fixture).
	p, _ := m.procByPID(21772)
	m.trackSuspended(procDoneMsg{job: procJob{op: opSuspend}, done: []int32{p.PID}})
	if !slices.Equal(m.stillSuspended(), []int32{21772}) {
		t.Fatalf("still suspended: %v", m.stillSuspended())
	}
	m = drive(t, m, key("q"))
	qp, ok := m.confirm.(quitPrompt)
	if !ok || qp.text() != "Resume the process you suspended before quitting? [Y/n]" {
		t.Fatalf("q should ask to resume: %#v", m.confirm)
	}
	if qp.respond("enter") == nil || qp.respond("n") == nil || qp.respond("x") != nil {
		t.Error("Enter/y resume and quit, n quits, anything else stays")
	}
	if got := drive(t, m, keyMsg("esc")); got.confirm != nil {
		t.Error("esc should cancel quitting")
	}
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("a second Ctrl+C should quit")
	}

	// Resuming it elsewhere (another tool) means there's nothing to offer.
	m.trackSuspended(procDoneMsg{job: procJob{op: opResume}, done: []int32{21772}})
	if len(m.stillSuspended()) != 0 {
		t.Error("resumed processes shouldn't be offered")
	}
}

func TestSuspendedGroupNeedsEveryMember(t *testing.T) {
	rows := view.Rows([]metrics.ProcInfo{{PID: 1, Name: "a", Status: "stop"}, {PID: 2, Name: "a"}}, true)
	if rows[0].Suspended {
		t.Error("a group is suspended only when every member is")
	}
}
