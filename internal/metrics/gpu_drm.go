package metrics

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Linux DRM client accounting: AMD (amdgpu) and Intel (i915, xe) drivers
// report, per open GPU file, cumulative busy time per engine and memory use
// in /proc/<pid>/fdinfo/<fd> (Documentation/gpu/drm-usage-stats.rst). This
// file has no build tag so tests can run it against a fake /proc on any OS.

// drmClient is one GPU client (a process can have several).
type drmClient struct {
	pid    int32
	engine map[string]uint64 // cumulative busy ns per engine
	vram   uint64            // bytes in device-local memory
}

// readDRMClients scans every process's GPU file descriptors under procRoot
// (normally "/proc"). Clients are keyed by driver, device and client ID, so
// several fds of one client are counted once.
func readDRMClients(procRoot string) map[string]drmClient {
	out := map[string]drmClient{}
	fds, _ := filepath.Glob(filepath.Join(procRoot, "[0-9]*", "fd", "*"))
	for _, fd := range fds {
		if target, err := os.Readlink(fd); err != nil || !strings.HasPrefix(target, "/dev/dri/") {
			continue
		}
		pidDir := filepath.Dir(filepath.Dir(fd))
		pid, err := strconv.ParseInt(filepath.Base(pidDir), 10, 32)
		if err != nil {
			continue
		}
		key, c, ok := parseDRMFdinfo(filepath.Join(pidDir, "fdinfo", filepath.Base(fd)))
		if ok {
			c.pid = int32(pid)
			out[key] = c
		}
	}
	return out
}

func parseDRMFdinfo(path string) (string, drmClient, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", drmClient{}, false
	}
	defer f.Close()

	c := drmClient{engine: map[string]uint64{}}
	var driver, pdev, id string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch {
		case k == "drm-driver":
			driver = v
		case k == "drm-pdev":
			pdev = v
		case k == "drm-client-id":
			id = v
		case strings.HasPrefix(k, "drm-engine-") && !strings.HasPrefix(k, "drm-engine-capacity-"):
			if ns, ok := parseDRMValue(v); ok {
				c.engine[strings.TrimPrefix(k, "drm-engine-")] = ns
			}
		case isVRAMKey(k):
			if b, ok := parseDRMValue(v); ok {
				c.vram += b
			}
		}
	}
	if driver == "" || id == "" {
		return "", drmClient{}, false
	}
	return driver + "/" + pdev + "/" + id, c, true
}

// isVRAMKey matches device-local memory keys across drivers: amdgpu's
// drm-memory-vram, and the generic drm-resident-vram0 / drm-resident-local0.
func isVRAMKey(k string) bool {
	return k == "drm-memory-vram" || strings.HasPrefix(k, "drm-resident-vram") || strings.HasPrefix(k, "drm-resident-local")
}

// parseDRMValue parses "1234 ns", "512 KiB", "3 MiB" or a bare number, as
// nanoseconds or bytes.
func parseDRMValue(v string) (uint64, bool) {
	num, unit, _ := strings.Cut(v, " ")
	n, err := strconv.ParseUint(num, 10, 64)
	if err != nil {
		return 0, false
	}
	switch unit {
	case "KiB":
		n <<= 10
	case "MiB":
		n <<= 20
	case "GiB":
		n <<= 30
	}
	return n, true
}

// drmUsage turns two client scans taken wall apart into per-process GPU use:
// a client's utilization is its busiest engine; a process takes its busiest
// client and the sum of its clients' memory.
func drmUsage(prev, cur map[string]drmClient, wall time.Duration) gpuUsage {
	out := gpuUsage{}
	for key, c := range cur {
		u := out[c.pid]
		u.vram, u.vramKnown = u.vram+c.vram, true
		if p, ok := prev[key]; ok && wall > 0 {
			for eng, ns := range c.engine {
				busy := float64(sub(ns, p.engine[eng])) / float64(wall.Nanoseconds()) * 100
				u.util = max(u.util, min(busy, 100))
			}
			u.utilKnown = true
		}
		out[c.pid] = u
	}
	return out
}

// mergeUsage combines per-process GPU use from several sources (e.g. NVIDIA
// via NVML and AMD/Intel via DRM): busiest utilization, summed memory.
func mergeUsage(a, b gpuUsage) gpuUsage {
	out := gpuUsage{}
	for _, src := range []gpuUsage{a, b} {
		for pid, u := range src {
			o := out[pid]
			o.util, o.utilKnown = max(o.util, u.util), o.utilKnown || u.utilKnown
			o.vram, o.vramKnown = o.vram+u.vram, o.vramKnown || u.vramKnown
			out[pid] = o
		}
	}
	return out
}
