// Package metrics collects system-wide and per-process performance data.
package metrics

import (
	"context"
	"strings"
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

	Disks         []DiskStat
	VirtualDrives []VirtualDrive // drive letters that aren't disk partitions (Windows)
	Nets          []NetStat
	GPUs          []GPUStat
	Sockets       []Socket
}

// Socket is one TCP or UDP socket and the process that owns it.
type Socket struct {
	Proto      string // "TCP" or "UDP"
	LocalIP    string
	LocalPort  uint32
	RemoteIP   string // "" when not connected
	RemotePort uint32
	State      string // TCP state such as "LISTEN" or "ESTABLISHED"; "" for UDP
	PID        int32  // 0 when unknown (e.g. another user's socket without root on Linux)
}

// Listening reports whether the socket waits for others to connect: a TCP
// socket in LISTEN or a bound, unconnected UDP socket.
func (s Socket) Listening() bool {
	if s.Proto == "UDP" {
		return s.RemotePort == 0
	}
	return s.State == "LISTEN"
}

// ProcInfo is a point-in-time view of one process.
type ProcInfo struct {
	PID        int32
	PPID       int32 // parent; 0 when unknown
	Name       string
	Exe        string
	User       string
	Status     string // e.g. "running", "sleep", "zombie"; "" where not reported (Windows)
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

// Status values the dashboard acts on (gopsutil's names).
const (
	statusZombie  = "zombie"
	statusStopped = "stop"
)

// Zombie reports whether the process has exited but is still listed because
// its parent hasn't collected its exit status yet (Unix).
func (p ProcInfo) Zombie() bool { return strings.Contains(p.Status, statusZombie) }

// Suspended reports whether the process is paused: stopped by a signal
// (Unix) or with every thread suspended (Windows).
func (p ProcInfo) Suspended() bool { return strings.Contains(p.Status, statusStopped) }

// Sampler produces snapshots. Rate-based fields (CPU) are computed from the
// delta against the previous call, so the first call primes the sampler.
type Sampler interface {
	Sample(ctx context.Context) (SystemSnapshot, []ProcInfo, error)
}
