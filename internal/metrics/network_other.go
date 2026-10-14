//go:build !windows && !linux

package metrics

import "strings"

// isPhysicalNIC treats macOS en* interfaces (Wi-Fi, Ethernet) as physical.
func isPhysicalNIC(name string) bool { return strings.HasPrefix(name, "en") }

func nicSpeedBps(string) uint64 { return 0 }
