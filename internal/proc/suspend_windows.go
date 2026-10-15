package proc

import "golang.org/x/sys/windows"

var (
	ntdll             = windows.NewLazySystemDLL("ntdll.dll")
	procNtSuspendProc = ntdll.NewProc("NtSuspendProcess")
	procNtResumeProc  = ntdll.NewProc("NtResumeProcess")
)

// suspend uses the undocumented but long-stable NtSuspendProcess /
// NtResumeProcess, as Task Manager's Resource Monitor and Process Explorer do.
func suspend(pid int32, pause bool) error {
	h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(pid))
	if err != nil {
		return mapErr(err)
	}
	defer windows.CloseHandle(h)
	call := procNtResumeProc
	if pause {
		call = procNtSuspendProc
	}
	if status, _, _ := call.Call(uintptr(h)); status != 0 {
		return mapErr(windows.NTStatus(status).Errno())
	}
	return nil
}
