package proc

import (
	"errors"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32          = windows.NewLazySystemDLL("user32.dll")
	procPostMessage = user32.NewProc("PostMessageW")
	procIsVisible   = user32.NewProc("IsWindowVisible")
)

const wmClose = 0x0010

// Windows callbacks are never freed, so one shared callback is created and its
// per-call state lives in package variables guarded by enumMu.
var (
	enumMu     sync.Mutex
	enumPID    uint32
	enumHWNDs  []windows.HWND
	enumWindow = windows.NewCallback(func(hwnd windows.HWND, _ uintptr) uintptr {
		var owner uint32
		windows.GetWindowThreadProcessId(hwnd, &owner)
		if owner == enumPID {
			if visible, _, _ := procIsVisible.Call(uintptr(hwnd)); visible != 0 {
				enumHWNDs = append(enumHWNDs, hwnd)
			}
		}
		return 1 // continue enumeration
	})
)

func kill(pid int32, force bool) error {
	if force {
		return terminate(uint32(pid))
	}
	return closeWindows(uint32(pid))
}

func terminate(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return mapErr(err)
	}
	defer windows.CloseHandle(h)
	return mapErr(windows.TerminateProcess(h, 1))
}

// closeWindows posts WM_CLOSE to every visible top-level window owned by pid,
// which is what Task Manager's "End task" and taskkill without /F do.
func closeWindows(pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return mapErr(err)
	}
	windows.CloseHandle(h)

	hwnds, err := windowsOf(pid)
	if err != nil {
		return err
	}
	if len(hwnds) == 0 {
		return ErrNoWindow
	}

	var lastErr error
	posted := 0
	for _, hwnd := range hwnds {
		if r, _, err := procPostMessage.Call(uintptr(hwnd), wmClose, 0, 0); r == 0 {
			lastErr = mapErr(err)
			continue
		}
		posted++
	}
	if posted == 0 {
		return lastErr
	}
	return nil
}

func windowsOf(pid uint32) ([]windows.HWND, error) {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID, enumHWNDs = pid, nil
	err := windows.EnumWindows(enumWindow, unsafe.Pointer(nil))
	return enumHWNDs, err
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, windows.ERROR_ACCESS_DENIED):
		return ErrAccessDenied
	case errors.Is(err, windows.ERROR_INVALID_PARAMETER):
		// OpenProcess reports a nonexistent PID as an invalid parameter.
		return ErrNotFound
	}
	return err
}
