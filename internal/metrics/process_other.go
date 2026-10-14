//go:build !windows

package metrics

// processSnapshot is only needed on Windows; elsewhere per-process lookups
// work for every process.
func processSnapshot() map[int32]snapshotEntry { return nil }
