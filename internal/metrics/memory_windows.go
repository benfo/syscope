package metrics

import "github.com/shirou/gopsutil/v4/mem"

// commitCharge returns Windows commit charge (used, limit).
func commitCharge() (used, limit uint64) {
	ex, err := mem.NewExWindows().VirtualMemory()
	if err != nil {
		return 0, 0
	}
	return ex.CommitTotal, ex.CommitLimit
}
