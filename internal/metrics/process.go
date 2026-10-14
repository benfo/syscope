package metrics

import (
	"context"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// processCollector samples every process, caching per-process state between
// samples: static fields that are expensive to look up, and the previous CPU
// time for the delta.
type processCollector struct {
	numCPU  int
	entries map[int32]*procEntry
	gen     uint64 // sample generation; entries not seen this generation are dropped
}

type procEntry struct {
	proc    *process.Process
	create  int64
	name    string
	exe     string
	user    string
	prevCPU float64
	prevAt  time.Time
	hasPrev bool
	gen     uint64
}

func newProcessCollector(numCPU int) *processCollector {
	return &processCollector{numCPU: numCPU, entries: map[int32]*procEntry{}}
}

func (c *processCollector) collect(ctx context.Context, now time.Time) ([]ProcInfo, error) {
	pids, err := process.PidsWithContext(ctx)
	if err != nil {
		return nil, err
	}
	names := snapshotNames()

	c.gen++
	out := make([]ProcInfo, 0, len(pids))
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, c.sample(ctx, pid, names[pid], now))
	}
	for pid, e := range c.entries {
		if e.gen != c.gen {
			delete(c.entries, pid)
		}
	}
	return out, nil
}

func (c *processCollector) sample(ctx context.Context, pid int32, snapName string, now time.Time) ProcInfo {
	fresh := &process.Process{Pid: pid}
	create, _ := fresh.CreateTimeWithContext(ctx)

	e := c.entries[pid]
	if e == nil || staleEntry(e.create, create) {
		e = &procEntry{proc: fresh, create: create}
		e.exe, _ = fresh.ExeWithContext(ctx)
		e.name = snapName
		if e.name == "" {
			e.name, _ = fresh.NameWithContext(ctx)
		}
		e.user, _ = fresh.UsernameWithContext(ctx)
		c.entries[pid] = e
	}
	e.gen = c.gen

	info := ProcInfo{PID: pid, Name: e.name, Exe: e.exe, User: e.user}
	if e.create != 0 {
		info.CreateTime = time.UnixMilli(e.create)
	}
	if t, err := e.proc.TimesWithContext(ctx); err == nil {
		total := t.User + t.System
		if e.hasPrev {
			info.CPU = procCPUPercent(e.prevCPU, total, now.Sub(e.prevAt), c.numCPU)
			info.CPUKnown = true
		}
		e.prevCPU, e.prevAt, e.hasPrev = total, now, true
	}
	if m, err := e.proc.MemoryInfoWithContext(ctx); err == nil {
		info.RSS, info.RSSKnown = m.RSS, true
	}
	if st, err := e.proc.StatusWithContext(ctx); err == nil && len(st) > 0 {
		info.Status = strings.Join(st, ",")
	}
	return info
}

// procCPUPercent converts the CPU seconds a process consumed over a wall-clock
// window into a share of total machine capacity (0–100).
func procCPUPercent(prevSec, curSec float64, wall time.Duration, numCPU int) float64 {
	if wall <= 0 || numCPU < 1 || curSec < prevSec {
		return 0
	}
	pct := (curSec - prevSec) / wall.Seconds() / float64(numCPU) * 100
	return clampPct(pct)
}

// staleEntry reports whether a cached process entry belongs to a different
// process than the one now holding its PID (PID reuse).
func staleEntry(cachedCreate, currentCreate int64) bool {
	return cachedCreate != 0 && currentCreate != 0 && cachedCreate != currentCreate
}
