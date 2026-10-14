package metrics

import (
	"os"
	"strings"
)

// isWholeDisk keeps real block devices (sda, nvme0n1, vda, mmcblk0) and
// drops partitions and virtual devices (loop, ram, zram, device-mapper).
func isWholeDisk(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "dm-", "sr", "fd"} {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	_, err := os.Stat("/sys/block/" + name)
	return err == nil
}

func diskModel(name string) string {
	b, err := os.ReadFile("/sys/block/" + name + "/device/model")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
