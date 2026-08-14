//go:build !windows

package launch

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestClassifyAudioFailureAsRecoverable(t *testing.T) {
	err := classifyClientFailure("Failed to open audio device")
	var audioError *AudioUnavailableError
	if !errors.As(err, &audioError) {
		t.Fatalf("audio failure was not recoverable: %T %v", err, err)
	}
}

func TestMoonlightFatalAudioOutputStopsHungClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := Command{MoonlightPairing: &MoonlightPairing{}}
	cmd := exec.CommandContext(ctx, "sh", "-c", "printf 'Failed to open audio device\\n' >&2; sleep 30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	output := &boundedOutput{limit: 32768, updated: make(chan struct{}, 1)}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err := waitForNativeClient(cmd, output, command)
	var audioError *AudioUnavailableError
	if !errors.As(err, &audioError) {
		t.Fatalf("fatal audio output was not returned as recoverable: %T %v", err, err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("hung Moonlight client was not stopped promptly: %s", elapsed)
	}
}

func TestUserInitiatedRDPLogoffIsSuccessful(t *testing.T) {
	output := "certificate mismatch warning from connection startup\nERRINFO_LOGOFF_BY_USER (0x0000000C): The disconnection was initiated by the user logging off"
	if !clientExitedNormally(output) {
		t.Fatal("user-initiated RDP logoff was classified as a client failure")
	}
}

func TestNativeClientCancellationTargetsProcessGroup(t *testing.T) {
	t.Setenv("THINPI_AUDIO_DRIVER", "alsa")
	t.Setenv("THINPI_ALSA_DEVICE", "plughw:CARD=vc4hdmi0,DEV=0")
	command := exec.Command("moonlight-qt")
	configureNativeCommand(command, &syscall.Credential{Uid: 1, Gid: 1}, "/home/thinpi", nil)
	if command.SysProcAttr == nil || !command.SysProcAttr.Setpgid {
		t.Fatal("native client was not placed in its own process group")
	}
	if command.Cancel == nil {
		t.Fatal("native client cancellation does not terminate its process group")
	}
	environment := strings.Join(command.Env, "\n")
	for _, want := range []string{"XDG_RUNTIME_DIR=/run/user/1", "DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1/bus", "PULSE_SERVER=unix:/run/user/1/pulse/native", "SDL_AUDIODRIVER=alsa", "SDL_AUDIO_DRIVER=alsa", "AUDIODEV=plughw:CARD=vc4hdmi0,DEV=0", "SDL_AUDIO_ALSA_DEFAULT_PLAYBACK_DEVICE=plughw:CARD=vc4hdmi0,DEV=0"} {
		if !strings.Contains(environment, want) {
			t.Fatalf("native client environment is missing %q: %s", want, environment)
		}
	}
}

func TestNativePulseOutputAvailableRejectsOnlyNullSink(t *testing.T) {
	if nativePulseOutputAvailable("0\tauto_null\tmodule-null-sink.c\ts16le 2ch 44100Hz\tSUSPENDED\n") {
		t.Fatal("PulseAudio null sink was accepted as physical output")
	}
	if !nativePulseOutputAvailable("1\talsa_output.platform-hdmi.stereo\tmodule-alsa-card.c\ts16le 2ch 48000Hz\tRUNNING\n") {
		t.Fatal("physical PulseAudio sink was rejected")
	}
}
