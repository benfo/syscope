package tui

import (
	"slices"
	"testing"
)

func TestKillPromptOffersTree(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("j"), key("d")) // chrome.exe 21248, parent of 47768
	pk := *m.confirm
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
	if len(m.confirm.tree) != 0 || m.confirm.prompt() != "End task ollama.exe (PID 4120)? [y/N]" {
		t.Errorf("prompt: %q", m.confirm.prompt())
	}
	if _, ok := m.confirm.answer("t"); ok {
		t.Error("t means nothing without children; it should cancel")
	}
}
