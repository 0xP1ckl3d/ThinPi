//go:build !windows

package launch

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const nativeClientStopGracePeriod = 1500 * time.Millisecond

func terminateProcessGroup(pid int) error {
	credential, _ := nativeSessionIdentity()
	return terminateProcessGroupAs(pid, credential, nativeClientStopGracePeriod)
}

// terminateProcessGroupAs deliberately sends signals from a short-lived
// process running as the kiosk user. Native clients are owned by that user,
// so the kernel's normal UID checks constrain termination to kiosk-owned
// processes and the root agent does not need CAP_KILL.
func terminateProcessGroupAs(pid int, credential *syscall.Credential, gracePeriod time.Duration) error {
	if pid <= 0 {
		return errors.New("client process has not started")
	}
	if !processGroupExists(pid) {
		return os.ErrProcessDone
	}
	if err := signalProcessGroupAs(pid, syscall.SIGTERM, credential); err != nil && processGroupExists(pid) {
		return err
	}
	deadline := time.Now().Add(gracePeriod)
	for processGroupExists(pid) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if !processGroupExists(pid) {
		return nil
	}
	if err := signalProcessGroupAs(pid, syscall.SIGKILL, credential); err != nil && processGroupExists(pid) {
		return err
	}
	return nil
}

func signalProcessGroupAs(pid int, signal syscall.Signal, credential *syscall.Credential) error {
	if credential == nil {
		return syscall.Kill(-pid, signal)
	}
	command := nativeSignalCommand(pid, signal, credential)
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	diagnostic := strings.TrimSpace(string(output))
	if diagnostic == "" {
		diagnostic = err.Error()
	}
	return fmt.Errorf("signal kiosk process group %d: %s", pid, diagnostic)
}

func nativeSignalCommand(pid int, signal syscall.Signal, credential *syscall.Credential) *exec.Cmd {
	binary := strings.TrimSpace(os.Getenv("THINPI_SIGNAL_HELPER"))
	if binary == "" {
		binary = "/bin/kill"
	}
	command := exec.Command(binary, "-s", signalName(signal), "--", "-"+strconv.Itoa(pid))
	command.SysProcAttr = &syscall.SysProcAttr{Credential: credential}
	return command
}

func signalName(signal syscall.Signal) string {
	switch signal {
	case syscall.SIGTERM:
		return "TERM"
	case syscall.SIGKILL:
		return "KILL"
	default:
		return strconv.Itoa(int(signal))
	}
}

func processGroupExists(pid int) bool {
	err := syscall.Kill(-pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
