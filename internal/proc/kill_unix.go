//go:build !windows

package proc

import (
	"errors"
	"syscall"
)

func kill(pid int32, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return signal(pid, sig)
}

func suspend(pid int32, pause bool) error {
	if pause {
		return signal(pid, syscall.SIGSTOP)
	}
	return signal(pid, syscall.SIGCONT)
}

func signal(pid int32, sig syscall.Signal) error {
	err := syscall.Kill(int(pid), sig)
	switch {
	case errors.Is(err, syscall.ESRCH):
		return ErrNotFound
	case errors.Is(err, syscall.EPERM):
		return ErrAccessDenied
	}
	return err
}
