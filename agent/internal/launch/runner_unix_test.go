//go:build !windows

package launch

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

func TestClassifyClientFailure(t *testing.T) {
	cases := map[string]string{
		"failed to open display: :0":                      "display",
		"ERRCONNECT_LOGON_FAILURE":                        "username or password",
		"ERRCONNECT_CONNECT_TRANSPORT_FAILED":             "host and port",
		"certificate mismatch for remote system":          "certificate changed",
		"freerdp command line parsing failed at argument": "configuration",
	}
	for output, want := range cases {
		if got := classifyClientFailure(output).Error(); !strings.Contains(strings.ToLower(got), want) {
			t.Fatalf("%q: got %q, want %q", output, got, want)
		}
	}
}

func TestUserInitiatedRDPLogoffIsSuccessful(t *testing.T) {
	output := "certificate mismatch warning from connection startup\nERRINFO_LOGOFF_BY_USER (0x0000000C): The disconnection was initiated by the user logging off"
	if !clientExitedNormally(output) {
		t.Fatal("user-initiated RDP logoff was classified as a client failure")
	}
}

func TestNativeClientCancellationTargetsProcessGroup(t *testing.T) {
	command := exec.Command("true")
	configureNativeCommand(command, &syscall.Credential{Uid: 1, Gid: 1}, "/home/thinpi", nil)
	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
		t.Fatal("native client was not placed in its own process group")
	}
	if command.Cancel == nil {
		t.Fatal("native client cancellation does not terminate its process group")
	}
	environment := strings.Join(command.Env, "\n")
	for _, want := range []string{"XDG_RUNTIME_DIR=/run/user/1", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1/bus", "PULSE_SERVER=unix:/run/user/1/pulse/native"} {
		if !strings.Contains(environment, want) {
			t.Fatalf("native client environment is missing %q: %s", want, environment)
		}
	}
}
