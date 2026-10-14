package metrics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/mem"
)

// memoryCollector reads RAM, cache, commit charge (Windows) and swap.
type memoryCollector struct{}

func (memoryCollector) collect(ctx context.Context, _ time.Time, snap *SystemSnapshot) error {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}
	snap.MemTotal, snap.MemUsed, snap.MemAvailable = vm.Total, vm.Used, vm.Available
	snap.MemCached = vm.Cached
	snap.CommitUsed, snap.CommitLimit = commitCharge()
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		snap.SwapTotal, snap.SwapUsed = sw.Total, sw.Used
	}
	return nil
}
