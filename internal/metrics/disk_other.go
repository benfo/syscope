//go:build !windows && !linux

package metrics

import "regexp"

var wholeDiskRE = regexp.MustCompile(`^disk\d+$`)

// isWholeDisk matches macOS whole-disk names (disk0, not disk0s1).
func isWholeDisk(name string) bool { return wholeDiskRE.MatchString(name) }

func diskModel(string) string { return "" }
