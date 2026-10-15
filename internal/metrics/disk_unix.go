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
	volIO := map[string]map[string]volumeIO{} // disk → mount → counters
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			part := filepath.Base(p.Device)
			d := parentDisk(part, names)
			if d == "" {
				continue
			}
			mounts[d] = append(mounts[d], p.Mountpoint)
			if c, ok := ctrs[part]; ok { // the partition's own counters
				if volIO[d] == nil {
					volIO[d] = map[string]volumeIO{}
				}
				volIO[d][p.Mountpoint] = volumeIO{c.ReadBytes, c.WriteBytes}
			}
		}
	}

	out := make([]diskRaw, 0, len(names))
	for _, name := range names {
		c := ctrs[name]
		out = append(out, diskRaw{
			ID: name, Name: name, Model: diskModel(name), Mounts: mounts[name], VolumeIO: volIO[name],
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

// readVirtualDrives: elsewhere virtual file systems (FUSE mounts such as
// rclone) don't map to a physical disk in the first place.
func readVirtualDrives() []VirtualDrive { return nil }
