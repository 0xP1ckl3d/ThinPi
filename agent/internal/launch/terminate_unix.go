//go:build !windows

package launch

import (
	"errors"
	"syscall"
)

func terminateProcessGroup(pid int) error {
	if pid <= 0 {
		return errors.New("client process has not started")
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}
