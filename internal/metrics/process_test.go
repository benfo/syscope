package metrics

import (
	"context"
	"math"
	"os"
	"testing"
	"time"
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

func TestParentPIDs(t *testing.T) {
	s := NewGopsutilSampler()
	_, procs, err := s.Sample(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	self, parent := int32(os.Getpid()), int32(os.Getppid())
	for _, p := range procs {
		if p.PID == self {
			if p.PPID != parent {
				t.Errorf("own PPID %d, want %d", p.PPID, parent)
			}
			return
		}
	}
	t.Error("own process not found")
}

func TestReadProcDetailsSelf(t *testing.T) {
	ctx := context.Background()
	d, err := ReadProcDetails(ctx, int32(os.Getpid()))
	if err != nil {
		t.Fatal(err)
	}
	if d.Cmdline == "" || !d.ThreadsKnown || d.Threads < 1 || !d.IOKnown {
		t.Errorf("own details incomplete: %+v", d)
	}
	t.Logf("threads=%d handles=%d(%v) io r=%d w=%d cwd=%q", d.Threads, d.Handles, d.HandlesKnown, d.IORead, d.IOWrite, d.Cwd)
	if _, err := ReadProcDetails(ctx, 0x7ffffff1); err == nil {
		t.Error("missing process should fail")
	}
}

func TestIORates(t *testing.T) {
	t0 := time.Unix(100, 0)
	prev := ProcDetails{PID: 1, ReadAt: t0, IORead: 1000, IOWrite: 0, IOKnown: true}
	cur := ProcDetails{PID: 1, ReadAt: t0.Add(2 * time.Second), IORead: 5000, IOWrite: 600, IOKnown: true}
	if r, w, ok := IORates(prev, cur); !ok || r != 2000 || w != 300 {
		t.Errorf("rates: %v %v %v", r, w, ok)
	}
	cur.PID = 2
	if _, _, ok := IORates(prev, cur); ok {
		t.Error("different processes must not produce a rate")
	}
}
