package launch

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	alsaCardPattern = regexp.MustCompile(`(?m)^\s*([0-9]+)\s+\[([^]]+)\]`)
	alsaPCMPattern  = regexp.MustCompile(`(?m)^\s*([0-9]+)-([0-9]+):.*:\s+playback\s`)
)

// piALSAAudioCandidates returns physical playback devices in preference order:
// non-HDMI devices that are actually present (USB speakers, DACs, analogue
// outputs), the connected HDMI port, then any remaining HDMI playback device.
// The configured ALSA "default" device is tried separately after this list.
func piALSAAudioCandidates(asoundRoot, drmRoot string) []string {
	cards, _ := os.ReadFile(filepath.Join(asoundRoot, "cards"))
	pcms, _ := os.ReadFile(filepath.Join(asoundRoot, "pcm"))
	cardIDs := make(map[int]string)
	for _, match := range alsaCardPattern.FindAllStringSubmatch(string(cards), -1) {
		number, err := strconv.Atoi(match[1])
		if err == nil {
			cardIDs[number] = strings.TrimSpace(match[2])
		}
	}

	connectedHDMI := piConnectedHDMICard(drmRoot)
	var physical, connected, remaining []string
	seen := make(map[string]bool)
	for _, match := range alsaPCMPattern.FindAllStringSubmatch(string(pcms), -1) {
		cardNumber, cardErr := strconv.Atoi(match[1])
		deviceNumber, deviceErr := strconv.Atoi(match[2])
		cardID := cardIDs[cardNumber]
		if cardErr != nil || deviceErr != nil || cardID == "" {
			continue
		}
		device := "plughw:CARD=" + cardID + ",DEV=" + strconv.Itoa(deviceNumber)
		if seen[device] {
			continue
		}
		seen[device] = true
		if !strings.HasPrefix(strings.ToLower(cardID), "vc4hdmi") {
			physical = append(physical, device)
		} else if cardID == connectedHDMI {
			connected = append(connected, device)
		} else {
			remaining = append(remaining, device)
		}
	}
	return append(append(physical, connected...), remaining...)
}

func piConnectedHDMICard(drmRoot string) string {
	statuses, _ := filepath.Glob(filepath.Join(drmRoot, "card*-HDMI-A-*", "status"))
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
			return "vc4hdmi" + strconv.Itoa(port-1)
		}
	}
	return ""
}
