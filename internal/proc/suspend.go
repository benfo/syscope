package proc

// Suspend pauses every thread of a process (SIGSTOP on Unix,
// NtSuspendProcess on Windows) until Resume. The process keeps its memory.
func Suspend(pid int32) error {
	if pid <= 0 {
		return ErrNotFound
	}
	return suspend(pid, true)
}

// Resume continues a suspended process.
func Resume(pid int32) error {
	if pid <= 0 {
		return ErrNotFound
	}
	return suspend(pid, false)
}
