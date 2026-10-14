// Package metrics collects system-wide and per-process performance data.
package metrics

import (
	"context"
	"time"
)

// SystemSnapshot is a point-in-time view of system-wide resource usage.
type SystemSnapshot struct {
	Time       time.Time
	NumCPU     int
	CPUTotal   float64   // 0–100, busy share of all logical cores
	CPUPerCore []float64 // 0–100 per logical core
	CPUKnown   bool      // false on the first sample (no delta yet)

	MemTotal     uint64
	MemUsed      uint64
	MemAvailable uint64
	MemCached    uint64 // page cache (Linux/macOS); 0 where not reported
	CommitUsed   uint64 // Windows commit charge; 0 elsewhere
	CommitLimit  uint64
	SwapTotal    uint64
	SwapUsed     uint64

	ProcCount int
	Uptime    time.Duration

	Disks []DiskStat
	Nets  []NetStat
	GPUs  []GPUStat
}

// ProcInfo is a point-in-time view of one process.
type ProcInfo struct {
	PID        int32
	PPID       int32 // parent; 0 when unknown
	Name       string
	Exe        string
	User       string
	Status     string
	CreateTime time.Time

	// CPU is normalized: 100 means every logical core is saturated
	// (Task Manager style). Multiply by NumCPU for per-core % (top style).
	CPU      float64
	CPUKnown bool // false when access is denied or there is no previous sample

	RSS      uint64 // resident set size; working set on Windows
	RSSKnown bool

	GPU       float64 // busiest GPU engine this process uses, 0–100
	GPUKnown  bool
	GPUEngine string // where that is, e.g. "GPU 0 · 3D"
	VRAM      uint64 // dedicated GPU memory
	VRAMKnown bool
}

// Sampler produces snapshots. Rate-based fields (CPU) are computed from the
// delta against the previous call, so the first call primes the sampler.
type Sampler interface {
	Sample(ctx context.Context) (SystemSnapshot, []ProcInfo, error)
}
