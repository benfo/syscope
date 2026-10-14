//go:build !windows && !linux

package metrics

// coreKinds is not implemented on this platform yet (macOS exposes P/E
// counts via sysctl but not which CPU index is which).
func coreKinds(int) []CoreKind { return nil }

// commitCharge is a Windows concept.
func commitCharge() (used, limit uint64) { return 0, 0 }
