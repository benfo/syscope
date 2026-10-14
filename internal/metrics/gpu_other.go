//go:build !windows && !linux

package metrics

// gpuState: GPU metrics aren't implemented on this platform yet (macOS
// Apple Silicon via IOKit is on the roadmap).
type gpuState struct{}

func (g *gpuState) read() ([]GPUStat, procGPUReport) { return nil, procGPUReport{} }
