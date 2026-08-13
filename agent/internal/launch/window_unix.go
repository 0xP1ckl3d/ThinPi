//go:build !windows

package launch

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

type x11WindowController struct{}

func newWindowController() WindowController { return x11WindowController{} }

func (x11WindowController) Minimize(pid int) (string, error) {
	xdotool, err := exec.LookPath("xdotool")
	if err != nil {
		return "", errors.New("xdotool is unavailable")
	}
	output, err := exec.Command(xdotool, "search", "--onlyvisible", "--pid", strconv.Itoa(pid)).Output()
	windowIDs := strings.Fields(string(output))
	if err != nil || len(windowIDs) == 0 {
		return "", errors.New("remote window is unavailable")
	}
	for _, windowID := range windowIDs {
		if err = exec.Command(xdotool, "windowminimize", windowID).Run(); err != nil {
			return "", errors.New("remote window could not be minimized")
		}
	}
	return windowIDs[len(windowIDs)-1] + "|" + strings.Join(windowIDs, ","), nil
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
