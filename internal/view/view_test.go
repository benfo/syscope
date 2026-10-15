package view

import (
	"reflect"
	"testing"
	"time"

	"taskman/internal/metrics"
)

func procs() []metrics.ProcInfo {
	return []metrics.ProcInfo{
		{PID: 10, Name: "chrome.exe", User: "sam", CPU: 3, CPUKnown: true, RSS: 300, RSSKnown: true},
		{PID: 4, Name: "System", User: "SYSTEM", CPU: 1, CPUKnown: true},
		{PID: 22, Name: "Chrome.exe", User: "sam", CPU: 2, CPUKnown: true, RSS: 200, RSSKnown: true},
		{PID: 7, Name: "code.exe", User: "sam", CPU: 3, CPUKnown: true, RSS: 900, RSSKnown: true},
	}
}

func pids(rows []Row) []int32 {
	out := make([]int32, len(rows))
	for i, r := range rows {
		out[i] = r.PID
	}
	return out
}

func TestSort(t *testing.T) {
	tests := []struct {
		key  SortKey
		desc bool
		want []int32
	}{
		{SortCPU, true, []int32{7, 10, 22, 4}}, // tie 7/10 broken by PID asc
		{SortMem, true, []int32{7, 10, 22, 4}},
		{SortPID, false, []int32{4, 7, 10, 22}},
		{SortPID, true, []int32{22, 10, 7, 4}},
		{SortName, false, []int32{10, 22, 7, 4}},
	}
	for _, tt := range tests {
		rows := Rows(procs(), false)
		Sort(rows, tt.key, tt.desc)
		if got := pids(rows); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%v desc=%v: got %v, want %v", tt.key, tt.desc, got, tt.want)
		}
	}
}

func TestFilter(t *testing.T) {
	if got := len(Filter(procs(), "CHROME")); got != 2 {
		t.Errorf("name filter: got %d, want 2", got)
	}
	if got := Filter(procs(), "2"); len(got) != 1 || got[0].PID != 22 {
		t.Errorf("pid prefix filter: got %+v", got)
	}
	if got := len(Filter(procs(), "  ")); got != 4 {
		t.Errorf("blank filter: got %d, want 4", got)
	}
}

func TestGroup(t *testing.T) {
	rows := Rows(procs(), true)
	if len(rows) != 3 {
		t.Fatalf("got %d groups, want 3", len(rows))
	}
	var chrome Row
	for _, r := range rows {
		if r.Key == "g:chrome" {
			chrome = r
		}
	}
	if chrome.Count != 2 || chrome.CPU != 5 || chrome.RSS != 500 || chrome.Name != "chrome" ||
		!reflect.DeepEqual(chrome.PIDs, []int32{10, 22}) || chrome.PID != 10 {
		t.Errorf("chrome group wrong: %+v", chrome)
	}
	for _, r := range rows {
		if r.Name == "System" && r.RSSKnown {
			t.Error("group with no known RSS should stay unknown")
		}
	}
}

func TestMatchesName(t *testing.T) {
	for _, c := range []struct {
		proc, want string
		ok         bool
	}{
		{"notepad.exe", "notepad", true},
		{"Notepad.exe", "NOTEPAD.EXE", true},
		{"sleep", "sleep", true},
		{"notepad++.exe", "notepad", false},
	} {
		if got := MatchesName(c.proc, c.want); got != c.ok {
			t.Errorf("MatchesName(%q, %q) = %v", c.proc, c.want, got)
		}
	}
}

func TestShortUser(t *testing.T) {
	if got := ShortUser(`CORP\sam`); got != "sam" {
		t.Errorf("got %q", got)
	}
	if got := ShortUser("root"); got != "root" {
		t.Errorf("got %q", got)
	}
}

func TestParseSortKey(t *testing.T) {
	if k, err := ParseSortKey("MEM"); err != nil || k != SortMem {
		t.Errorf("got %v, %v", k, err)
	}
	if _, err := ParseSortKey("disk"); err == nil {
		t.Error("expected error")
	}
}

func TestFormat(t *testing.T) {
	cases := [][2]string{
		{FormatBytes(512), "512 B"},
		{FormatBytes(1536), "1.5 KiB"},
		{FormatBytes(3 << 30), "3.0 GiB"},
		{FormatMem(0, false), "--"},
		{FormatCPU(25, true, 8, false), "25.0"},
		{FormatCPU(25, true, 8, true), "200.0"},
		{FormatCPU(0, false, 8, true), "--"},
		{FormatUptime(50 * time.Minute), "0h 50m"},
		{FormatUptime(76*time.Hour + time.Hour), "3d 5h 0m"},
		{FormatByteRate(0), "0 B/s"},
		{FormatByteRate(412_000), "412 KB/s"},
		{FormatByteRate(1_250_000), "1.2 MB/s"},
		{FormatBitRate(1_050_000), "8.4 Mbps"},
		{FormatBitRate(125), "1.0 Kbps"},
		{FormatBitRate(100), "800 bps"},
		{FormatBytesCompact(3 << 30), "3.0GiB"},
		{FormatUsedTotal(9<<30, 12<<30), "9.0/12.0G"},
		{FormatUsedTotal(300<<20, 512<<20), "300/512M"},
	}
	for _, c := range cases {
		if got, want := c[0], c[1]; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestPercent(t *testing.T) {
	if Percent(1, 4) != 25 || Percent(5, 0) != 0 {
		t.Error("Percent")
	}
}

func TestGPUSortAndGroup(t *testing.T) {
	ps := []metrics.ProcInfo{
		{PID: 1, Name: "ollama", GPU: 80, GPUKnown: true, VRAM: 7 << 30, VRAMKnown: true},
		{PID: 2, Name: "chrome.exe", GPU: 30, GPUKnown: true, VRAM: 100 << 20, VRAMKnown: true},
		{PID: 3, Name: "chrome.exe", GPU: 90, GPUKnown: true, VRAM: 200 << 20, VRAMKnown: true},
		{PID: 4, Name: "idle"},
	}
	rows := Rows(ps, false)
	Sort(rows, SortVRAM, true)
	if got := pids(rows); !reflect.DeepEqual(got, []int32{1, 3, 2, 4}) {
		t.Errorf("vram sort: %v", got)
	}
	Sort(rows, SortGPU, true)
	if got := pids(rows); !reflect.DeepEqual(got, []int32{3, 1, 2, 4}) {
		t.Errorf("gpu sort: %v", got)
	}
	for _, r := range Rows(ps, true) {
		if r.Name == "chrome" && (r.GPU != 100 || r.VRAM != 300<<20) {
			t.Errorf("chrome group: GPU %v (capped at 100), VRAM %d", r.GPU, r.VRAM)
		}
		if r.Name == "idle" && (r.GPUKnown || r.VRAMKnown) {
			t.Error("group without GPU data should stay unknown")
		}
	}
	if !SortVRAM.IsGPU() || SortMem.IsGPU() || !SortGPU.DefaultDesc() || SortName.DefaultDesc() {
		t.Error("GPU sort key properties")
	}
	if FormatGPU(12.34, true) != "12.3" || FormatGPU(0, false) != "--" {
		t.Error("FormatGPU")
	}
}

func TestExitedRows(t *testing.T) {
	procs := []metrics.ProcInfo{
		{PID: 1, Name: "worker", Status: "zombie"},
		{PID: 2, Name: "worker", Status: "zombie"},
		{PID: 3, Name: "server", Status: "zombie"},
		{PID: 4, Name: "server", Status: "running"},
	}
	flat := Rows(procs, false)
	if !flat[0].Exited || flat[3].Exited {
		t.Errorf("flat: %+v", flat)
	}
	for _, g := range Rows(procs, true) {
		if want := g.Name == "worker"; g.Exited != want {
			t.Errorf("group %s: exited %v, want %v (only when every member has)", g.Name, g.Exited, want)
		}
	}
}
