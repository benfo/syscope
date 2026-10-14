package metrics

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// coreKinds uses the Intel hybrid PMU lists when present, otherwise the
// per-CPU capacity that ARM big.LITTLE kernels expose.
func coreKinds(logical int) []CoreKind {
	if logical < 1 {
		return nil
	}
	pcore, err1 := os.ReadFile("/sys/devices/cpu_core/cpus")
	ecore, err2 := os.ReadFile("/sys/devices/cpu_atom/cpus")
	if err1 == nil && err2 == nil {
		p, errP := parseCPUList(string(pcore))
		e, errE := parseCPUList(string(ecore))
		if errP == nil && errE == nil {
			kinds := make([]CoreKind, logical)
			for _, i := range p {
				if i < logical {
					kinds[i] = CorePerformance
				}
			}
			for _, i := range e {
				if i < logical {
					kinds[i] = CoreEfficiency
				}
			}
			return kinds
		}
	}

	caps := make([]int, logical)
	for i := range caps {
		caps[i] = -1
		b, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpu_capacity", i))
		if err != nil {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
			caps[i] = v
		}
	}
	return kindsFromClasses(caps)
}

// commitCharge is a Windows concept.
func commitCharge() (used, limit uint64) { return 0, 0 }

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
