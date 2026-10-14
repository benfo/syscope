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
	err := syscall.Kill(int(pid), sig)
	switch {
	case errors.Is(err, syscall.ESRCH):
		return ErrNotFound
	case errors.Is(err, syscall.EPERM):
		return ErrAccessDenied
	}
	return err
}
