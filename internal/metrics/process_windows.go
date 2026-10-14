package metrics

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processSnapshot returns the image name and parent of every process from
// one Toolhelp snapshot. Unlike per-process lookups it works for protected
// processes.
func processSnapshot() map[int32]snapshotEntry {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	out := map[int32]snapshotEntry{}
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		out[int32(pe.ProcessID)] = snapshotEntry{
			name: windows.UTF16ToString(pe.ExeFile[:]),
			ppid: int32(pe.ParentProcessID),
		}
	}
	return out
}
