package metrics

import (
	"context"
	"math"
	"os"
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
