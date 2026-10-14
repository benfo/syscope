package metrics

import (
	"context"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

func TestProcCPUPercent(t *testing.T) {
	tests := []struct {
		name      string
		prev, cur float64
		wall      time.Duration
		numCPU    int
		want      float64
	}{
		{"two of eight cores saturated", 10, 12, time.Second, 8, 25},
		{"one core on one-core box", 0, 1, time.Second, 1, 100},
		{"idle", 5, 5, time.Second, 4, 0},
		{"clock went backwards", 5, 4, time.Second, 4, 0},
		{"zero window", 0, 1, 0, 4, 0},
		{"clamped", 0, 100, time.Second, 2, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := procCPUPercent(tt.prev, tt.cur, tt.wall, tt.numCPU)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSystemCPUPercent(t *testing.T) {
	prev := cpu.TimesStat{User: 10, System: 10, Idle: 80}
	cur := cpu.TimesStat{User: 20, System: 15, Idle: 165, Iowait: 0}
	// delta total 100, delta idle 85 -> 15% busy
	if got := systemCPUPercent(prev, cur); math.Abs(got-15) > 1e-9 {
		t.Errorf("got %v, want 15", got)
	}
	if got := systemCPUPercent(cur, cur); got != 0 {
		t.Errorf("no elapsed time: got %v, want 0", got)
	}
}

func TestStaleEntry(t *testing.T) {
	if !staleEntry(1000, 2000) {
		t.Error("different create times should be stale (PID reused)")
	}
	if staleEntry(1000, 1000) {
		t.Error("same create time should not be stale")
	}
	if staleEntry(0, 2000) || staleEntry(1000, 0) {
		t.Error("unknown create time should not force a refresh")
	}
}

func TestGopsutilSamplerFindsSelf(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	if _, _, err := s.Sample(ctx); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	snap, procs, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.CPUKnown || snap.MemTotal == 0 || snap.NumCPU < 1 {
		t.Errorf("bad system snapshot: %+v", snap)
	}
	self := int32(os.Getpid())
	for _, p := range procs {
		if p.PID == self {
			if p.Name == "" || !p.CPUKnown || !p.RSSKnown || p.RSS == 0 {
				t.Errorf("own process incomplete: %+v", p)
			}
			return
		}
	}
	t.Errorf("own PID %d not found among %d processes", self, len(procs))
}

func TestKindsFromClasses(t *testing.T) {
	got := kindsFromClasses([]int{1, 1, 0, 0, -1})
	want := []CoreKind{CorePerformance, CorePerformance, CoreEfficiency, CoreEfficiency, CoreUnknown}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hybrid: got %v, want %v", got, want)
	}
	if got := kindsFromClasses([]int{0, 0, 0}); got != nil {
		t.Errorf("uniform CPU should not be hybrid: %v", got)
	}
	if got := kindsFromClasses([]int{-1, -1}); got != nil {
		t.Errorf("unknown classes: %v", got)
	}
}

func TestParseCPUList(t *testing.T) {
	got, err := parseCPUList("0-3,8,10-11\n")
	if want := []int{0, 1, 2, 3, 8, 10, 11}; err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, bad := range []string{"x", "3-1", "1-y"} {
		if _, err := parseCPUList(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestReadCPUInfo(t *testing.T) {
	info := ReadCPUInfo(context.Background())
	if info.Logical < 1 || info.Model == "" {
		t.Errorf("incomplete: %+v", info)
	}
	if info.Kinds != nil && len(info.Kinds) != info.Logical {
		t.Errorf("kinds length %d != logical %d", len(info.Kinds), info.Logical)
	}
	t.Logf("%s, %d logical, P=%d E=%d", info.Model, info.Logical,
		info.Count(CorePerformance), info.Count(CoreEfficiency))
}

func TestDiskDelta(t *testing.T) {
	// Windows style: 100ns ticks, busy measured against the query clock.
	prev := diskRaw{ReadBytes: 1000, WriteBytes: 0, Reads: 10, Writes: 0,
		TickUnit: 100 * time.Nanosecond, IOTime: 0, Busy: 0, Clock: 1e7, HasBusy: true, HasIO: true}
	cur := prev
	cur.ReadBytes, cur.WriteBytes = 1000+4_000_000, 2_000_000
	cur.Reads, cur.Writes = 30, 10            // 30 I/Os
	cur.IOTime = 30 * 5_000                   // 0.5 ms each
	cur.Clock, cur.Busy = 2e7, 2_500_000      // 0.25s busy of 1s
	st := diskDelta(prev, cur, 2*time.Second) // wall ignored for active when Clock is set
	if st.ReadBps != 2_000_000 || st.WriteBps != 1_000_000 {
		t.Errorf("rates: %+v", st)
	}
	if !st.ActiveKnown || math.Abs(st.ActivePct-25) > 1e-9 {
		t.Errorf("active: %v %v", st.ActivePct, st.ActiveKnown)
	}
	if !st.RespKnown || math.Abs(st.RespMs-0.5) > 1e-9 {
		t.Errorf("response: %v", st.RespMs)
	}

	// Linux style: ms ticks against wall time; counter reset reads as zero.
	lp := diskRaw{TickUnit: time.Millisecond, Busy: 1000, ReadBytes: 500, HasBusy: true}
	lc := lp
	lc.Busy, lc.ReadBytes = 1500, 100
	st = diskDelta(lp, lc, time.Second)
	if math.Abs(st.ActivePct-50) > 1e-9 || st.ReadBps != 0 || st.RespKnown {
		t.Errorf("linux style: %+v", st)
	}
}

func TestParentDisk(t *testing.T) {
	disks := []string{"nvme0n1", "nvme0n10", "sda"}
	for part, want := range map[string]string{
		"nvme0n1p2": "nvme0n1", "nvme0n10p1": "nvme0n10", "sda1": "sda", "sdb1": "", "sda": "sda",
	} {
		if got := parentDisk(part, disks); got != want {
			t.Errorf("parentDisk(%q) = %q, want %q", part, got, want)
		}
	}
}

func TestReadDisks(t *testing.T) {
	s := NewGopsutilSampler()
	ctx := context.Background()
	s.Sample(ctx)
	time.Sleep(300 * time.Millisecond)
	snap, _, err := s.Sample(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Disks) == 0 {
		t.Skip("no physical disks visible (container?)")
	}
	for _, d := range snap.Disks {
		if !d.RatesKnown {
			t.Errorf("%s: rates unknown on second sample", d.Name)
		}
		t.Logf("%s %q %s active=%.1f%%(%v) r=%.0f w=%.0f resp=%.2fms vols=%+v",
			d.Name, d.Model, d.Bus, d.ActivePct, d.ActiveKnown, d.ReadBps, d.WriteBps, d.RespMs, d.Volumes)
	}
}
