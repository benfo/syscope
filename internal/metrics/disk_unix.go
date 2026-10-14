//go:build !windows

package metrics

import (
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
)

// readDisks uses gopsutil's per-device counters, keeping whole disks only
// and attaching each mounted partition to its parent disk.
func readDisks() ([]diskRaw, error) {
	ctrs, err := disk.IOCounters()
	if err != nil {
		return nil, err
	}
	var names []string
	for name := range ctrs {
		if isWholeDisk(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	mounts := map[string][]string{}
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			if d := parentDisk(filepath.Base(p.Device), names); d != "" {
				mounts[d] = append(mounts[d], p.Mountpoint)
			}
		}
	}

	out := make([]diskRaw, 0, len(names))
	for _, name := range names {
		c := ctrs[name]
		out = append(out, diskRaw{
			ID: name, Name: name, Model: diskModel(name), Mounts: mounts[name],
			ReadBytes: c.ReadBytes, WriteBytes: c.WriteBytes, Reads: c.ReadCount, Writes: c.WriteCount,
			TickUnit: time.Millisecond,
			IOTime:   c.ReadTime + c.WriteTime,
			Busy:     c.IoTime,
			HasBusy:  runtime.GOOS == "linux", // io_ticks; macOS has no busy counter
			HasIO:    runtime.GOOS == "linux",
		})
	}
	return out, nil
}

func mountLabel(m string) string { return m }
