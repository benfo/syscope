package metrics

import (
	"os"
	"strconv"
	"strings"
)

// isPhysicalNIC reports whether an interface is backed by a device (not a
// bridge, veth, tun or docker interface).
func isPhysicalNIC(name string) bool {
	_, err := os.Stat("/sys/class/net/" + name + "/device")
	return err == nil
}

func nicSpeedBps(name string) uint64 {
	b, err := os.ReadFile("/sys/class/net/" + name + "/speed")
	if err != nil {
		return 0
	}
	mbps, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || mbps <= 0 {
		return 0
	}
	return uint64(mbps) * 1_000_000
}
