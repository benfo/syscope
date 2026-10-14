package metrics

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// collector fills one area of a SystemSnapshot. Each collector keeps its own
// previous-sample state, so rate calculations stay independent. Collectors
// only return an error when the snapshot would be meaningless without them;
// optional data (disks, networks, GPUs) is simply left empty on failure.
type collector interface {
	collect(ctx context.Context, now time.Time, snap *SystemSnapshot) error
}

// GopsutilSampler is the real Sampler: it runs the system-wide collectors,
// then the process collector. Safe for concurrent use; calls are serialized.
type GopsutilSampler struct {
	mu         sync.Mutex
	numCPU     int
	collectors []collector
	procs      *processCollector
}

func NewGopsutilSampler() *GopsutilSampler {
	n, err := cpu.Counts(true)
	if err != nil || n < 1 {
		n = runtime.NumCPU()
	}
	return &GopsutilSampler{
		numCPU: n,
		collectors: []collector{
			&cpuCollector{}, memoryCollector{}, &diskCollector{}, newNetworkCollector(), &gpuCollector{},
		},
		procs: newProcessCollector(n),
	}
}

func (s *GopsutilSampler) Sample(ctx context.Context) (SystemSnapshot, []ProcInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	snap := SystemSnapshot{Time: now, NumCPU: s.numCPU}
	for _, c := range s.collectors {
		if err := c.collect(ctx, now, &snap); err != nil {
			return snap, nil, err
		}
	}
	procs, err := s.procs.collect(ctx, now)
	if err != nil {
		return snap, nil, err
	}
	snap.ProcCount = len(procs)
	return snap, procs, nil
}
