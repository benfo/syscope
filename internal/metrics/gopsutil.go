package metrics

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

// entry caches per-process state between samples: static fields that are
// expensive to look up, and the previous CPU time for delta calculation.
type entry struct {
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

// GopsutilSampler is the real Sampler backed by gopsutil. Safe for concurrent
// use; calls are serialized.
type GopsutilSampler struct {
	mu      sync.Mutex
	numCPU  int
	procs   map[int32]*entry
	gen     uint64
	prevTot *cpu.TimesStat
	prevPer []cpu.TimesStat
}

func NewGopsutilSampler() *GopsutilSampler {
	n, err := cpu.Counts(true)
	if err != nil || n < 1 {
		n = runtime.NumCPU()
	}
	return &GopsutilSampler{numCPU: n, procs: map[int32]*entry{}}
}

func (s *GopsutilSampler) Sample(ctx context.Context) (SystemSnapshot, []ProcInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	snap, err := s.system(ctx, now)
	if err != nil {
		return snap, nil, err
	}

	pids, err := process.PidsWithContext(ctx)
	if err != nil {
		return snap, nil, err
	}
	names := snapshotNames()

	s.gen++
	out := make([]ProcInfo, 0, len(pids))
	for _, pid := range pids {
		if err := ctx.Err(); err != nil {
			return snap, nil, err
		}
		out = append(out, s.sampleProc(ctx, pid, names[pid], now))
	}
	for pid, e := range s.procs {
		if e.gen != s.gen {
			delete(s.procs, pid)
		}
	}
	snap.ProcCount = len(out)
	return snap, out, nil
}

func (s *GopsutilSampler) system(ctx context.Context, now time.Time) (SystemSnapshot, error) {
	snap := SystemSnapshot{Time: now, NumCPU: s.numCPU}

	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return snap, err
	}
	snap.MemTotal, snap.MemUsed, snap.MemAvailable = vm.Total, vm.Used, vm.Available
	if sw, err := mem.SwapMemoryWithContext(ctx); err == nil {
		snap.SwapTotal, snap.SwapUsed = sw.Total, sw.Used
	}
	if up, err := host.UptimeWithContext(ctx); err == nil {
		snap.Uptime = time.Duration(up) * time.Second
	}

	tot, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return snap, err
	}
	per, _ := cpu.TimesWithContext(ctx, true)
	if s.prevTot != nil && len(tot) > 0 {
		snap.CPUTotal = systemCPUPercent(*s.prevTot, tot[0])
		snap.CPUKnown = true
		if len(per) == len(s.prevPer) {
			snap.CPUPerCore = make([]float64, len(per))
			for i := range per {
				snap.CPUPerCore[i] = systemCPUPercent(s.prevPer[i], per[i])
			}
		}
	}
	if len(tot) > 0 {
		s.prevTot = &tot[0]
	}
	s.prevPer = per
	return snap, nil
}

func (s *GopsutilSampler) sampleProc(ctx context.Context, pid int32, snapName string, now time.Time) ProcInfo {
	fresh := &process.Process{Pid: pid}
	create, _ := fresh.CreateTimeWithContext(ctx)

	e := s.procs[pid]
	if e == nil || staleEntry(e.create, create) {
		e = &entry{proc: fresh, create: create}
		e.exe, _ = fresh.ExeWithContext(ctx)
		e.name = snapName
		if e.name == "" {
			e.name, _ = fresh.NameWithContext(ctx)
		}
		e.user, _ = fresh.UsernameWithContext(ctx)
		s.procs[pid] = e
	}
	e.gen = s.gen

	info := ProcInfo{PID: pid, Name: e.name, Exe: e.exe, User: e.user}
	if e.create != 0 {
		info.CreateTime = time.UnixMilli(e.create)
	}
	if t, err := e.proc.TimesWithContext(ctx); err == nil {
		total := t.User + t.System
		if e.hasPrev {
			info.CPU = procCPUPercent(e.prevCPU, total, now.Sub(e.prevAt), s.numCPU)
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
