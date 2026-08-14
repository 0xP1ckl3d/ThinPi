//go:build windows

package launch

import "os"

func terminateProcessGroup(pid int) error {
	if pid <= 0 {
		return os.ErrInvalid
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Kill()
}
