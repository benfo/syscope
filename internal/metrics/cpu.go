package metrics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
)

// cpuCollector computes system-wide and per-core utilization from the CPU
// time deltas between samples, plus uptime.
type cpuCollector struct {
	prevTotal *cpu.TimesStat
	prevCores []cpu.TimesStat
}

func (c *cpuCollector) collect(ctx context.Context, _ time.Time, snap *SystemSnapshot) error {
	if up, err := host.UptimeWithContext(ctx); err == nil {
		snap.Uptime = time.Duration(up) * time.Second
	}
	total, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return err
	}
	cores, _ := cpu.TimesWithContext(ctx, true)
	if c.prevTotal != nil && len(total) > 0 {
		snap.CPUTotal = systemCPUPercent(*c.prevTotal, total[0])
		snap.CPUKnown = true
		if len(cores) == len(c.prevCores) {
			snap.CPUPerCore = make([]float64, len(cores))
			for i := range cores {
				snap.CPUPerCore[i] = systemCPUPercent(c.prevCores[i], cores[i])
			}
		}
	}
	if len(total) > 0 {
		c.prevTotal = &total[0]
	}
	c.prevCores = cores
	return nil
}

// systemCPUPercent returns the busy share (0–100) between two CPU time samples.
func systemCPUPercent(prev, cur cpu.TimesStat) float64 {
	total := cur.Total() - prev.Total()
	if total <= 0 {
		return 0
	}
	idle := (cur.Idle + cur.Iowait) - (prev.Idle + prev.Iowait)
	return clampPct((total - idle) / total * 100)
}

func clampPct(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 100:
		return 100
	}
	return v
}
