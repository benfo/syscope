package metrics

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// loadNVML finds nvml.dll in System32 (current drivers) or the legacy NVSMI
// folder (older drivers).
func loadNVML() (func(string) (nvmlFunc, bool), bool) {
	dll := windows.NewLazySystemDLL("nvml.dll")
	if dll.Load() != nil {
		legacy := filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvml.dll")
		dll = windows.NewLazyDLL(legacy)
		if dll.Load() != nil {
			return nil, false
		}
	}
	return func(name string) (nvmlFunc, bool) {
		p := dll.NewProc(name)
		if p.Find() != nil {
			return nil, false
		}
		return func(args ...uintptr) uintptr {
			r, _, _ := p.Call(args...)
			return r
		}, true
	}, true
}
