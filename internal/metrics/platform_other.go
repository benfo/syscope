//go:build !windows && !linux

package metrics

import "regexp"

// coreKinds is not implemented on this platform yet (macOS exposes P/E
// counts via sysctl but not which CPU index is which).
func coreKinds(int) []CoreKind { return nil }

// commitCharge is a Windows concept.
func commitCharge() (used, limit uint64) { return 0, 0 }

var wholeDiskRE = regexp.MustCompile(`^disk\d+$`)

// isWholeDisk matches macOS whole-disk names (disk0, not disk0s1).
func isWholeDisk(name string) bool { return wholeDiskRE.MatchString(name) }

func diskModel(string) string { return "" }
