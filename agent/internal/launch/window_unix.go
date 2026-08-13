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
	primaryWindowID := ""
	if err == nil && len(windowIDs) > 0 {
		primaryWindowID = windowIDs[len(windowIDs)-1]
	} else {
		active, activeErr := exec.Command(xdotool, "getactivewindow").Output()
		primaryWindowID = strings.TrimSpace(string(active))
		if activeErr != nil || primaryWindowID == "" {
			return "", errors.New("remote window is unavailable")
		}
		name, _ := exec.Command(xdotool, "getwindowname", primaryWindowID).Output()
		if strings.TrimSpace(string(name)) == "ThinPi" {
			return "", errors.New("the launcher cannot be minimized as a remote session")
		}
		windowIDs = []string{primaryWindowID}
		if activePID, pidErr := exec.Command(xdotool, "getwindowpid", primaryWindowID).Output(); pidErr == nil {
			activePID = []byte(strings.TrimSpace(string(activePID)))
			if len(activePID) > 0 {
				if related, searchErr := exec.Command(xdotool, "search", "--onlyvisible", "--pid", string(activePID)).Output(); searchErr == nil {
					if found := strings.Fields(string(related)); len(found) > 0 {
						windowIDs = found
					}
				}
			}
		}
	}
	primaryIncluded := false
	for _, windowID := range windowIDs {
		primaryIncluded = primaryIncluded || windowID == primaryWindowID
	}
	if !primaryIncluded {
		windowIDs = append(windowIDs, primaryWindowID)
	}
	for _, windowID := range windowIDs {
		if err = exec.Command(xdotool, "windowminimize", windowID).Run(); err != nil {
			return "", errors.New("remote window could not be minimized")
		}
	}
	return primaryWindowID + "|" + strings.Join(windowIDs, ","), nil
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
