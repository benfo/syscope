package metrics

import (
	"strings"
	"time"
)

// VolumeStat is a mounted volume on a disk.
type VolumeStat struct {
	Mount string // "C:" on Windows, mount point elsewhere
	Total uint64
	Used  uint64
}

// DiskStat is one physical disk.
type DiskStat struct {
	ID      string
	Name    string // "Disk 0" on Windows, kernel name elsewhere
	Model   string
	Bus     string // "NVMe", "SATA", "USB", …
	Volumes []VolumeStat

	RatesKnown bool // false on the first sample
	ReadBps    float64
	WriteBps   float64

	ActivePct   float64 // share of time the disk was busy (Task Manager "Active time")
	ActiveKnown bool
	RespMs      float64 // average time per completed I/O
	RespKnown   bool
}

// diskRaw holds cumulative counters for one disk. Time counters are in
// TickUnit because Windows' are 100ns system-clock values that overflow
// time.Duration.
type diskRaw struct {
	ID, Name, Model, Bus string
	Mounts               []string // paths usable with disk.Usage

	ReadBytes, WriteBytes uint64
	Reads, Writes         uint64

	TickUnit time.Duration
	IOTime   uint64 // cumulative time spent on I/Os (sum over requests)
	Busy     uint64 // cumulative busy time
	Clock    uint64 // clock Busy is measured against; 0 = use wall time
	HasBusy  bool
	HasIO    bool
}

// diskDelta computes rates between two samples taken wall apart.
func diskDelta(prev, cur diskRaw, wall time.Duration) DiskStat {
	st := DiskStat{ID: cur.ID, Name: cur.Name, Model: cur.Model, Bus: cur.Bus}
	if wall <= 0 {
		return st
	}
	secs := wall.Seconds()
	st.RatesKnown = true
	st.ReadBps = float64(sub(cur.ReadBytes, prev.ReadBytes)) / secs
	st.WriteBps = float64(sub(cur.WriteBytes, prev.WriteBytes)) / secs

	unit := cur.TickUnit.Seconds()
	if cur.HasBusy && unit > 0 {
		denom := secs
		if cur.Clock > 0 {
			denom = float64(sub(cur.Clock, prev.Clock)) * unit
		}
		if denom > 0 {
			st.ActivePct = clampPct(float64(sub(cur.Busy, prev.Busy)) * unit / denom * 100)
			st.ActiveKnown = true
		}
	}
	if cur.HasIO && unit > 0 {
		st.RespKnown = true
		if ios := sub(cur.Reads, prev.Reads) + sub(cur.Writes, prev.Writes); ios > 0 {
			st.RespMs = float64(sub(cur.IOTime, prev.IOTime)) * unit * 1000 / float64(ios)
		}
	}
	return st
}

// sub is a counter delta that treats a reset or wrap as zero.
func sub(cur, prev uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// parentDisk returns the disk a partition device belongs to (longest
// prefix match, e.g. "nvme0n1p2" → "nvme0n1", "sda1" → "sda").
func parentDisk(part string, disks []string) string {
	best := ""
	for _, d := range disks {
		if strings.HasPrefix(part, d) && len(d) > len(best) {
			best = d
		}
	}
	return best
}
