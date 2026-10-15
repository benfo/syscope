package metrics

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processSnapshot returns the image name, parent and suspended state of every
// process from one Toolhelp snapshot (plus one NtQuerySystemInformation for
// thread states). Unlike per-process lookups it works for protected processes.
func processSnapshot() map[int32]snapshotEntry {
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(h)

	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	out := map[int32]snapshotEntry{}
	suspended := suspendedPIDs()
	for err = windows.Process32First(h, &pe); err == nil; err = windows.Process32Next(h, &pe) {
		out[int32(pe.ProcessID)] = snapshotEntry{
			name:      windows.UTF16ToString(pe.ExeFile[:]),
			ppid:      int32(pe.ParentProcessID),
			suspended: suspended[int32(pe.ProcessID)],
		}
	}
	return out
}

// systemThreadInformation is SYSTEM_THREAD_INFORMATION, which follows each
// SYSTEM_PROCESS_INFORMATION in NtQuerySystemInformation's output.
type systemThreadInformation struct {
	KernelTime, UserTime, CreateTime int64
	WaitTime                         uint32
	StartAddress                     uintptr
	ClientID                         [2]uintptr
	Priority, BasePriority           int32
	ContextSwitches                  uint32
	ThreadState                      uint32
	WaitReason                       uint32
}

const (
	threadStateWaiting   = 5
	waitReasonSuspended  = 5
	processInfoBufferMin = 1 << 20
)

// suspendedPIDs lists processes whose threads are all suspended, which is
// how Task Manager decides a process is "Suspended" (including Store apps
// Windows suspends in the background).
func suspendedPIDs() map[int32]bool {
	buf := make([]byte, processInfoBufferMin)
	for {
		var need uint32
		err := windows.NtQuerySystemInformation(windows.SystemProcessInformation,
			unsafe.Pointer(&buf[0]), uint32(len(buf)), &need)
		if err == nil {
			break
		}
		if err != windows.STATUS_INFO_LENGTH_MISMATCH || need == 0 {
			return nil
		}
		buf = make([]byte, need+need/4) // processes may start meanwhile
	}

	out := map[int32]bool{}
	for off := uintptr(0); ; {
		p := (*windows.SYSTEM_PROCESS_INFORMATION)(unsafe.Pointer(&buf[off]))
		threads := unsafe.Slice((*systemThreadInformation)(unsafe.Add(unsafe.Pointer(p), unsafe.Sizeof(*p))),
			p.NumberOfThreads)
		if len(threads) > 0 && allSuspended(threads) {
			out[int32(p.UniqueProcessID)] = true
		}
		if p.NextEntryOffset == 0 {
			return out
		}
		off += uintptr(p.NextEntryOffset)
	}
}

func allSuspended(threads []systemThreadInformation) bool {
	for _, t := range threads {
		if t.ThreadState != threadStateWaiting || t.WaitReason != waitReasonSuspended {
			return false
		}
	}
	return true
}
