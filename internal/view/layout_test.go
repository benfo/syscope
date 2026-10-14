package view

import (
	"reflect"
	"strings"
	"testing"

	"taskman/internal/metrics"
)

// treeProcs is a small process tree:
//
//	1 init
//	├─ 10 explorer
//	│  ├─ 100 chrome (cpu 5)
//	│  │  └─ 1000 chrome (cpu 9)
//	│  └─ 101 code (cpu 1)
//	└─ 11 sshd
//	500 orphan (parent 999 not running)
func treeProcs() []metrics.ProcInfo {
	return []metrics.ProcInfo{
		{PID: 1, PPID: 0, Name: "init"},
		{PID: 10, PPID: 1, Name: "explorer"},
		{PID: 100, PPID: 10, Name: "chrome", CPU: 5, CPUKnown: true},
		{PID: 1000, PPID: 100, Name: "chrome", CPU: 9, CPUKnown: true},
		{PID: 101, PPID: 10, Name: "code", CPU: 1, CPUKnown: true},
		{PID: 11, PPID: 1, Name: "sshd"},
		{PID: 500, PPID: 999, Name: "orphan"},
	}
}

func shape(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		mark := " "
		if r.Expandable {
			mark = "+"
			if r.Expanded {
				mark = "-"
			}
		}
		out[i] = mark + strings.Repeat(".", r.Depth) + r.Name
	}
	return out
}

func TestTreeOpenByDefault(t *testing.T) {
	got := shape(Build(treeProcs(), "", LayoutTree, SortPID, false, Folds{Default: true}))
	want := []string{"-init", "-.explorer", "-..chrome", " ...chrome", " ..code", " .sshd", " orphan"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree:\n got %v\nwant %v", got, want)
	}
}

func TestTreeSiblingsSortedAndFolded(t *testing.T) {
	folds := Folds{Default: true, Open: map[string]bool{ProcessKey(100): false}}
	rows := Build(treeProcs(), "", LayoutTree, SortCPU, true, folds)
	got := shape(rows)
	// Under explorer, chrome (cpu 5) sorts before code (cpu 1); chrome is folded.
	want := []string{"-init", "-.explorer", "+..chrome", " ..code", " .sshd", " orphan"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree:\n got %v\nwant %v", got, want)
	}
	if rows[2].Parent != ProcessKey(10) || rows[1].Parent != ProcessKey(1) || rows[0].Parent != "" {
		t.Error("parents should be recorded for collapse-to-parent")
	}
}

func TestTreeSearchKeepsAncestorsOpen(t *testing.T) {
	folds := Folds{Default: false} // even with everything folded…
	got := shape(Build(treeProcs(), "code", LayoutTree, SortPID, false, folds))
	want := []string{"-init", "-.explorer", " ..code"} // …the match shows with its ancestors
	if !reflect.DeepEqual(got, want) {
		t.Errorf("search:\n got %v\nwant %v", got, want)
	}
}

func TestTreeSurvivesParentCycles(t *testing.T) {
	procs := []metrics.ProcInfo{{PID: 1, PPID: 2, Name: "a"}, {PID: 2, PPID: 1, Name: "b"}, {PID: 3, PPID: 3, Name: "self"}}
	rows := Build(procs, "", LayoutTree, SortPID, false, Folds{Default: true})
	// a and b only have each other as parents: no root reaches them, but they
	// must still be listed (and not loop forever). self is its own parent: a root.
	if len(rows) != 3 {
		t.Errorf("every process must be listed: %v", shape(rows))
	}
}

func TestGroupedExpands(t *testing.T) {
	procs := []metrics.ProcInfo{
		{PID: 7, Name: "chrome.exe", CPU: 1, CPUKnown: true},
		{PID: 3, Name: "chrome.exe", CPU: 4, CPUKnown: true},
		{PID: 5, Name: "code.exe", CPU: 2, CPUKnown: true},
	}
	closed := Build(procs, "", LayoutGrouped, SortCPU, true, Folds{})
	if got := shape(closed); !reflect.DeepEqual(got, []string{"+chrome", " code"}) {
		t.Errorf("closed groups: %v", got)
	}
	open := Build(procs, "", LayoutGrouped, SortCPU, true, Folds{Open: map[string]bool{"g:chrome": true}})
	got := shape(open)
	if !reflect.DeepEqual(got, []string{"-chrome", " .chrome.exe", " .chrome.exe", " code"}) || open[1].PID != 3 || open[1].Parent != "g:chrome" {
		t.Errorf("open group, members sorted by CPU: %v", got)
	}
}

func TestParseLayout(t *testing.T) {
	if l, err := ParseLayout("Tree"); err != nil || l != LayoutTree || l.String() != "tree" {
		t.Errorf("%v %v", l, err)
	}
	if _, err := ParseLayout("spiral"); err == nil {
		t.Error("expected error")
	}
}
