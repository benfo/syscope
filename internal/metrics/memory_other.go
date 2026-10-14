//go:build !windows

package metrics

// commitCharge is a Windows concept.
func commitCharge() (used, limit uint64) { return 0, 0 }
