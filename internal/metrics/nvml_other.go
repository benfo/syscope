//go:build !windows && !linux

package metrics

// loadNVML: NVIDIA's management library isn't available on this platform.
func loadNVML() (func(string) (nvmlFunc, bool), bool) { return nil, false }
