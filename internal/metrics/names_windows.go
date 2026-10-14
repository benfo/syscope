package metrics

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// snapshotNames returns image names for all processes from one Toolhelp
// snapshot. Unlike per-process lookups it works for protected processes.
func snapshotNames() map[int32]string {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	names := map[int32]string{}
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		names[int32(pe.ProcessID)] = windows.UTF16ToString(pe.ExeFile[:])
	}
	return names
}
