package metrics

import (
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
)

// procCPUPercent converts the CPU seconds a process consumed over a wall-clock
// window into a share of total machine capacity (0–100).
func procCPUPercent(prevSec, curSec float64, wall time.Duration, numCPU int) float64 {
	if wall <= 0 || numCPU < 1 || curSec < prevSec {
		return 0
	}
	pct := (curSec - prevSec) / wall.Seconds() / float64(numCPU) * 100
	return clampPct(pct)
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

// staleEntry reports whether a cached process entry belongs to a different
// process than the one now holding its PID (PID reuse).
func staleEntry(cachedCreate, currentCreate int64) bool {
	return cachedCreate != 0 && currentCreate != 0 && cachedCreate != currentCreate
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
