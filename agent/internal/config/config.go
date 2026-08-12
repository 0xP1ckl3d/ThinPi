package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Config struct {
	ControllerURL       string `json:"controller_url"`
	DeviceFile          string `json:"device_file"`
	Socket              string `json:"socket"`
	CACertificate       string `json:"ca_certificate,omitempty"`
	MockClients         bool   `json:"mock_clients,omitempty"`
	MockDurationSeconds int    `json:"mock_duration_seconds,omitempty"`
	FreeRDPBinary       string `json:"freerdp_binary,omitempty"`
	MoonlightBinary     string `json:"moonlight_binary,omitempty"`
	VNCBinary           string `json:"vnc_binary,omitempty"`
	SSHBinary           string `json:"ssh_binary,omitempty"`
	TerminalBinary      string `json:"terminal_binary,omitempty"`
	SSHPassBinary       string `json:"sshpass_binary,omitempty"`
	MaintenanceUser     string `json:"maintenance_user,omitempty"`
}

type DeviceFile struct {
	DeviceIdentifier string `json:"device_identifier"`
	DeviceToken      string `json:"device_token"`
	Name             string `json:"name"`
}

func Defaults() Config {
	return Config{DeviceFile: "/etc/thinpi/device.json", Socket: "/run/thinpi/agent.sock", MockDurationSeconds: 3, FreeRDPBinary: "auto", MoonlightBinary: "auto", VNCBinary: "auto", SSHBinary: "auto", TerminalBinary: "auto", SSHPassBinary: "auto"}
}
func Load(path string) (Config, error) {
	c := Defaults()
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.ControllerURL == "" || c.DeviceFile == "" || c.Socket == "" {
		return c, errors.New("controller_url, device_file and socket are required")
	}
	return c, nil
}
func LoadDevice(path string) (DeviceFile, error) {
	var d DeviceFile
	b, err := os.ReadFile(path)
	if err != nil {
		return d, err
	}
	err = json.Unmarshal(b, &d)
	if err == nil && (d.DeviceIdentifier == "" || d.DeviceToken == "") {
		err = errors.New("invalid device file")
	}
	return d, err
}
func SaveDevice(path string, d DeviceFile) error {
	if d.DeviceIdentifier == "" || d.DeviceToken == "" {
		return errors.New("invalid device")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0600)
}
