//go:build !windows && !linux

package metrics

// coreKinds is not implemented on this platform yet (macOS exposes P/E
// counts via sysctl but not which CPU index is which).
func coreKinds(int) []CoreKind { return nil }
