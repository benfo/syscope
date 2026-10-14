package metrics

import (
	"os"
	"strconv"
	"time"
)

// gpuState keeps the previous DRM client scan for per-process utilization.
type gpuState struct {
	drmPrev map[string]drmClient
	drmAt   time.Time
}

// read lists NVIDIA GPUs via NVML and AMD GPUs via amdgpu sysfs, plus
// per-process use from NVML and from DRM fdinfo (AMD and Intel). System-wide
// Intel (i915/xe) usage needs root and isn't supported yet.
func (g *gpuState) read() ([]GPUStat, procGPUReport) {
	var out []GPUStat
	var nvProcs gpuUsage
	if n := openNVML(); n != nil {
		for _, d := range n.read() {
			st := d.stat
			st.Name = d.name
			out = append(out, st)
		}
		nvProcs = n.processes()
	}
	out = append(out, readAMD("/sys")...)
	for i := range out {
		out[i].ID = strconv.Itoa(i)
	}

	now := time.Now()
	clients := readDRMClients("/proc")
	drm := drmUsage(g.drmPrev, clients, now.Sub(g.drmAt))
	g.drmPrev, g.drmAt = clients, now
	// NVML lists every GPU process; DRM fdinfo only covers other users'
	// processes when running as root.
	complete := os.Geteuid() == 0 || (nvProcs != nil && !hasNonNVIDIAGPU("/sys"))
	return out, procGPUReport{usage: mergeUsage(nvProcs, drm), complete: complete}
}
