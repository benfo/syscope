package metrics

import (
	"context"
	"os"
	"testing"
	"time"
)

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
