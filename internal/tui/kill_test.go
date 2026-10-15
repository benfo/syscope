package tui

import (
	"slices"
	"strings"
	"taskman/internal/metrics"
	"testing"
)

func TestKillPromptOffersTree(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("j"), key("d")) // chrome.exe 21248, parent of 47768
	pk := killPrompt(t, m)
	if !slices.Equal(pk.tree, []int32{47768}) {
		t.Fatalf("tree: %v", pk.tree)
	}
	if job, ok := pk.answer("y"); !ok || !slices.Equal(job.pids, []int32{21248}) {
		t.Errorf("y should end only the process: %v", job.pids)
	}
	job, ok := pk.answer("t")
	if !ok || !slices.Equal(job.pids, []int32{21248, 47768}) || job.desc != "chrome.exe (PID 21248) and 1 process it started" {
		t.Errorf("t should end the tree, parent first: %+v", job)
	}
	if _, ok := pk.answer("n"); ok {
		t.Error("n should cancel")
	}
}

func TestKillPromptWithoutChildren(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("d")) // ollama.exe has no children
	if pk := killPrompt(t, m); len(pk.tree) != 0 || pk.text() != "End task ollama.exe (PID 4120)? [y/N]" {
		t.Errorf("prompt: %q", m.confirm.text())
	}
	if _, ok := killPrompt(t, m).answer("t"); ok {
		t.Error("t means nothing without children; it should cancel")
	}
}

// zombieProcs is a parent with one running and one exited child.
func zombieProcs() []metrics.ProcInfo {
	return []metrics.ProcInfo{
		p(100, "build.sh", 5),
		{PID: 101, PPID: 100, Name: "compiler", Status: "running", CPUKnown: true, CPU: 4},
		{PID: 102, PPID: 100, Name: "linker", Status: "zombie", CPUKnown: true},
	}
}

func TestZombiesMarkedAndNotKillable(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: zombieProcs()})
	if !strings.Contains(m.View(), "linker (exited)") {
		t.Error("an exited process should be marked")
	}
	m = drive(t, m, key("d")) // build.sh: its tree counts only the running child
	if !slices.Equal(killPrompt(t, m).tree, []int32{101}) {
		t.Fatalf("tree should skip exited children: %+v", m.confirm)
	}
	m = drive(t, m, key("n"), key("G")) // linker, lowest CPU
	if r, _ := m.table.selected(); r.PID != 102 {
		t.Fatalf("selected %d", r.PID)
	}
	if m = drive(t, m, key("d")); m.confirm != nil || !strings.Contains(m.status, "already exited") {
		t.Errorf("d on an exited process should explain, not prompt: %q", m.status)
	}
}

// killPrompt is the open kill prompt; it fails the test if there isn't one.
func killPrompt(t *testing.T, m model) pendingKill {
	t.Helper()
	pk, ok := m.confirm.(pendingKill)
	if !ok {
		t.Fatalf("no kill prompt open (got %T)", m.confirm)
	}
	return pk
}
