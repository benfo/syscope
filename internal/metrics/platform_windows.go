package metrics

import (
	"encoding/binary"
	"unsafe"

	"github.com/shirou/gopsutil/v4/mem"
	"golang.org/x/sys/windows"
)

var procGetLogicalProcessorInformationEx = windows.NewLazySystemDLL("kernel32.dll").
	NewProc("GetLogicalProcessorInformationEx")

const relationProcessorCore = 0

// coreKinds reads each core's EfficiencyClass (higher = faster) from
// GetLogicalProcessorInformationEx.
func coreKinds(logical int) []CoreKind {
	if logical < 1 {
		return nil
	}
	var size uint32
	procGetLogicalProcessorInformationEx.Call(relationProcessorCore, 0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return nil
	}
	buf := make([]byte, size)
	r, _, _ := procGetLogicalProcessorInformationEx.Call(relationProcessorCore,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return nil
	}

	classes := make([]int, logical)
	for i := range classes {
		classes[i] = -1
	}
	le := binary.LittleEndian
	// Each record: Relationship uint32, Size uint32, then PROCESSOR_RELATIONSHIP:
	// Flags byte, EfficiencyClass byte, Reserved [20]byte, GroupCount uint16,
	// GroupMask [GroupCount]GROUP_AFFINITY{Mask uint64, Group uint16, Reserved [3]uint16}.
	for off := 0; off+8 <= int(size); {
		recSize := int(le.Uint32(buf[off+4:]))
		if recSize <= 0 || off+recSize > int(size) {
			break
		}
		if le.Uint32(buf[off:]) == relationProcessorCore {
			p := off + 8
			eff := int(buf[p+1])
			groups := int(le.Uint16(buf[p+22:]))
			for g := 0; g < groups; g++ {
				ga := p + 24 + g*16
				if ga+16 > off+recSize {
					break
				}
				mask := le.Uint64(buf[ga:])
				group := int(le.Uint16(buf[ga+8:]))
				for bit := 0; bit < 64; bit++ {
					if mask&(1<<bit) != 0 {
						if idx := group*64 + bit; idx < logical {
							classes[idx] = eff
						}
					}
				}
			}
		}
		off += recSize
	}
	return kindsFromClasses(classes)
}

// commitCharge returns Windows commit charge (used, limit).
func commitCharge() (used, limit uint64) {
	ex, err := mem.NewExWindows().VirtualMemory()
	if err != nil {
		return 0, 0
	}
	return ex.CommitTotal, ex.CommitLimit
}
