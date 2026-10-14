// Package proc acts on processes.
package proc

import "errors"

var (
	ErrNotFound     = errors.New("no such process")
	ErrAccessDenied = errors.New("access denied: re-run as admin/sudo")
	// ErrNoWindow means a graceful close was requested on Windows for a
	// process without a visible window (console or background process).
	ErrNoWindow = errors.New("process has no window to close gracefully: use force")
)

// Kill stops a process. Graceful sends SIGTERM (Unix) or WM_CLOSE to the
// process's windows (Windows) and returns without waiting for it to exit.
// Force sends SIGKILL / TerminateProcess.
func Kill(pid int32, force bool) error {
	if pid <= 0 {
		return ErrNotFound
	}
	return kill(pid, force)
}
