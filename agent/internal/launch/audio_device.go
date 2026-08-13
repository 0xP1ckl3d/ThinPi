package launch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func piHDMIAudioDevice(sysfsRoot string) string {
	statuses, _ := filepath.Glob(filepath.Join(sysfsRoot, "card*-HDMI-A-*", "status"))
	for _, statusPath := range statuses {
		status, err := os.ReadFile(statusPath)
		if err != nil || strings.TrimSpace(string(status)) != "connected" {
			continue
		}
		connector := filepath.Base(filepath.Dir(statusPath))
		const marker = "-HDMI-A-"
		markerAt := strings.LastIndex(connector, marker)
		if markerAt < 0 {
			continue
		}
		port, err := strconv.Atoi(connector[markerAt+len(marker):])
		if err == nil && port > 0 {
			return "plughw:CARD=vc4hdmi" + strconv.Itoa(port-1) + ",DEV=0"
		}
	}
	return ""
}
