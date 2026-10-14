package tui

import (
	"strings"
	"testing"

	"taskman/internal/metrics"
	"taskman/internal/view"
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
	cols := []column{{width: 7}, {}, {width: 10}}
	if got := columnWidths(cols, 40); got[1] != 40-7-10-2 {
		t.Errorf("flex width: %v", got)
	}
	if got := columnWidths(cols, 10); got[1] != minFlexW {
		t.Errorf("flex width floor: %v", got)
	}
}
