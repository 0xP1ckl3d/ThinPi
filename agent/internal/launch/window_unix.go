//go:build !windows

package launch

import (
	"errors"
	"os/exec"
	"strings"
)

type x11WindowController struct{}

func newWindowController() WindowController { return x11WindowController{} }

func (x11WindowController) Minimize() (string, error) {
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		return "", errors.New("xdotool is unavailable")
	}
	output, err := exec.Command(xdotool, "getactivewindow").Output()
	activeWindowID := strings.TrimSpace(string(output))
	if err != nil || activeWindowID == "" {
		return "", errors.New("active remote window is unavailable")
	}
	name, _ := exec.Command(xdotool, "getwindowname", activeWindowID).Output()
	if strings.TrimSpace(string(name)) == "ThinPi" {
		return "", errors.New("the launcher cannot be minimized as a remote session")
	}
	windowIDs := []string{activeWindowID}
	if pidOutput, pidErr := exec.Command(xdotool, "getwindowpid", activeWindowID).Output(); pidErr == nil {
		pid := strings.TrimSpace(string(pidOutput))
		if pid != "" {
			if searchOutput, searchErr := exec.Command(xdotool, "search", "--pid", pid).Output(); searchErr == nil {
				windowIDs = strings.Fields(string(searchOutput))
			}
		}
	}
	activeIncluded := false
	for _, windowID := range windowIDs {
		activeIncluded = activeIncluded || windowID == activeWindowID
	}
	if !activeIncluded {
		windowIDs = append(windowIDs, activeWindowID)
	}
	for _, windowID := range windowIDs {
		if err = exec.Command(xdotool, "windowminimize", windowID).Run(); err != nil {
			return "", errors.New("remote window could not be minimized")
		}
	}
	return activeWindowID + "|" + strings.Join(windowIDs, ","), nil
}

func (x11WindowController) Resume(windowState string) error {
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		return errors.New("xdotool is unavailable")
	}
	parts := strings.SplitN(windowState, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("minimized remote window state is invalid")
	}
	for _, windowID := range strings.Split(parts[1], ",") {
		if windowID == "" {
			continue
		}
		if err = exec.Command(xdotool, "windowmap", windowID).Run(); err != nil {
			return errors.New("minimized remote window no longer exists")
		}
	}
	if err = exec.Command(xdotool, "windowactivate", "--sync", parts[0]).Run(); err != nil {
		return errors.New("minimized remote window could not be restored")
	}
	return nil
}
