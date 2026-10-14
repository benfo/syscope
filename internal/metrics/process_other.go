//go:build !windows

package metrics

// snapshotNames is only needed on Windows; elsewhere per-process name
// lookups work for every process.
func snapshotNames() map[int32]string { return nil }
