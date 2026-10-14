package metrics

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// ProcDetails is extra information about one process, read on demand for the
// details pane: too costly to read for every process on every sample.
// Fields the OS won't reveal (often for other users' processes) stay empty
// or have Known false.
type ProcDetails struct {
	PID     int32
	ReadAt  time.Time
	Cmdline string
	Cwd     string

	Threads      int32
	ThreadsKnown bool
	Handles      int32 // open handles on Windows, file descriptors elsewhere
	HandlesKnown bool

	// Cumulative bytes read and written. On Windows this counts all I/O
	// (files, network, devices), not only disk.
	IORead  uint64
	IOWrite uint64
	IOKnown bool
}

// ReadProcDetails reads details for one process; it fails only when the
// process no longer exists.
func ReadProcDetails(ctx context.Context, pid int32) (ProcDetails, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return ProcDetails{}, err
	}
	d := ProcDetails{PID: pid, ReadAt: time.Now()}
	d.Cmdline, _ = p.CmdlineWithContext(ctx)
	d.Cwd, _ = p.CwdWithContext(ctx)
	if n, err := p.NumThreadsWithContext(ctx); err == nil {
		d.Threads, d.ThreadsKnown = n, true
	}
	if n, err := p.NumFDsWithContext(ctx); err == nil {
		d.Handles, d.HandlesKnown = n, true
	}
	if io, err := p.IOCountersWithContext(ctx); err == nil {
		d.IORead, d.IOWrite, d.IOKnown = io.ReadBytes, io.WriteBytes, true
	}
	return d, nil
}

// IORates returns read and write bytes per second between two detail reads
// of the same process.
func IORates(prev, cur ProcDetails) (read, write float64, ok bool) {
	wall := cur.ReadAt.Sub(prev.ReadAt).Seconds()
	if prev.PID != cur.PID || !prev.IOKnown || !cur.IOKnown || wall <= 0 {
		return 0, 0, false
	}
	return float64(sub(cur.IORead, prev.IORead)) / wall, float64(sub(cur.IOWrite, prev.IOWrite)) / wall, true
}
