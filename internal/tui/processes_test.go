package tui

import (
	"strings"
	"testing"

	"github.com/benfo/syscope/internal/metrics"
	"github.com/benfo/syscope/internal/view"
)

func gpuProcs() []metrics.ProcInfo {
	return []metrics.ProcInfo{
		{PID: 1, Name: "ollama", CPU: 2, CPUKnown: true, GPU: 88, GPUKnown: true, VRAM: 7 << 30, VRAMKnown: true},
		{PID: 2, Name: "chrome.exe", CPU: 9, CPUKnown: true, GPU: 1, GPUKnown: true, VRAM: 200 << 20, VRAMKnown: true},
		{PID: 3, Name: "sshd"},
	}
}

func TestGPUColumnsWithGPU(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: gpuProcs()})
	v := m.View()
	for _, want := range []string{"GPU%", "VRAM", "88.0", "7.0 GiB"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q", want)
		}
	}
	// s cycles cpu → mem → gpu → vram.
	m = drive(t, m, key("s"), key("s"), key("s"))
	if m.table.sortKey != view.SortVRAM || m.table.rows[0].PID != 1 {
		t.Errorf("sort by vram: key %v, top PID %d", m.table.sortKey, m.table.rows[0].PID)
	}
	if help := strings.Join(drive(t, m, key("?")).helpContent(), "\n"); !strings.Contains(help, "fallen back to the CPU") {
		t.Error("help should explain GPU% when a GPU is present")
	}
}

func TestGPUColumnsHiddenWithoutGPU(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: gpuProcs(), noGPU: true})
	if v := m.View(); strings.Contains(v, "GPU%") || strings.Contains(v, "VRAM") {
		t.Error("GPU columns shown on a machine without a GPU")
	}
	m = drive(t, m, key("s"), key("s"))
	if m.table.sortKey != view.SortPID {
		t.Errorf("s should skip GPU sort keys without a GPU: got %v", m.table.sortKey)
	}
	if help := strings.Join(drive(t, m, key("?")).helpContent(), "\n"); strings.Contains(help, "VRAM") {
		t.Error("help mentions VRAM on a machine without a GPU")
	}
}

func TestColumnWidths(t *testing.T) {
	cols := []column[processTable, view.Row]{{width: 7}, {}, {width: 10}}
	if got := columnWidths(cols, 40); got[1] != 40-7-10-2 {
		t.Errorf("flex width: %v", got)
	}
	if got := columnWidths(cols, 10); got[1] != minFlexW {
		t.Errorf("flex width floor: %v", got)
	}
}

func TestLayoutKeys(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("t"))
	if m.table.layout != view.LayoutTree {
		t.Fatal("t should switch to the tree")
	}
	m = drive(t, m, key("z"))
	if m.table.layout != view.LayoutGrouped {
		t.Error("z from the tree should switch to grouped, not stack")
	}
	m = drive(t, m, key("z"))
	if m.table.layout != view.LayoutFlat {
		t.Error("z again should go back to flat")
	}
}

func TestFoldKeysOnGroups(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	m = drive(t, m, key("z")) // grouped, sorted by CPU: chrome (2 processes) on top
	top, _ := m.table.selected()
	if top.Key != "g:chrome" || top.Expanded {
		t.Fatalf("setup: top row %+v", top)
	}
	m = drive(t, m, key("l"))
	if r, _ := m.table.selected(); !r.Expanded || len(m.table.rows) != 8 {
		t.Fatalf("l should open the group: %d rows", len(m.table.rows))
	}
	m = drive(t, m, key("l")) // on an open group: move to its first child
	if r, _ := m.table.selected(); r.Parent != "g:chrome" || r.PID != 21248 {
		t.Errorf("second l should enter the group: %+v", r)
	}
	m = drive(t, m, key("h")) // on a child: back to the group
	if r, _ := m.table.selected(); r.Key != "g:chrome" || !r.Expanded {
		t.Errorf("h on a child should go to its group: %+v", r)
	}
	m = drive(t, m, key("h"))
	if r, _ := m.table.selected(); r.Expanded {
		t.Error("h on an open group should close it")
	}
	m = drive(t, m, key("L"))
	if len(m.table.rows) != 8 {
		t.Errorf("L should open every group: %d rows", len(m.table.rows))
	}
	m = drive(t, m, key("H"))
	if len(m.table.rows) != 6 {
		t.Errorf("H should close every group: %d rows", len(m.table.rows))
	}
}

func TestTreeRendersNesting(t *testing.T) {
	m := sampled(t, &fakeSampler{procs: goldenProcs()})
	v := drive(t, m, key("t"), key("s"), key("s"), key("s"), key("s")).View() // sort by PID
	for _, want := range []string{"▾ System", "    ollama.exe", "▾ chrome.exe", "    chrome.exe"} {
		if !strings.Contains(v, want) {
			t.Errorf("tree view missing %q:\n%s", want, v)
		}
	}
}
