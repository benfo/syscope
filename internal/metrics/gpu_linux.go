package metrics

import "strconv"

// gpuState has nothing to keep between samples on Linux.
type gpuState struct{}

// read lists NVIDIA GPUs via NVML and AMD GPUs via amdgpu sysfs. Intel
// (i915/xe) needs root for usage and isn't supported yet.
func (g *gpuState) read() []GPUStat {
	var out []GPUStat
	if n := openNVML(); n != nil {
		for _, d := range n.read() {
			st := d.stat
			st.Name = d.name
			out = append(out, st)
		}
	}
	out = append(out, readAMD("/sys")...)
	for i := range out {
		out[i].ID = strconv.Itoa(i)
	}
	return out
}
