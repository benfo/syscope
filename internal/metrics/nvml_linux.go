package metrics

import "github.com/ebitengine/purego"

// loadNVML opens libnvidia-ml without cgo.
func loadNVML() (func(string) (nvmlFunc, bool), bool) {
	lib, err := purego.Dlopen("libnvidia-ml.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, false
	}
	return func(name string) (nvmlFunc, bool) {
		fn, err := purego.Dlsym(lib, name)
		if err != nil {
			return nil, false
		}
		return func(args ...uintptr) uintptr {
			r, _, _ := purego.SyscallN(fn, args...)
			return r
		}, true
	}, true
}
