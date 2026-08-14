//go:build !windows

package launch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type PlatformRunner struct{}

func (PlatformRunner) Run(ctx context.Context, c Command) error {
	if c.Path == "thinpi-mock" {
		return runMock(ctx, c)
	}
	credential, sessionHome := nativeSessionIdentity()
	configure := func(cmd *exec.Cmd) {
		configureNativeCommand(cmd, credential, sessionHome, c.Env)
	}
	if err := ensureMoonlightPaired(ctx, c, configure); err != nil {
		return err
	}
	audioSink, err := prepareMoonlightAudio(ctx, c, configure)
	if err != nil {
		return err
	}
	if audioSink != "" && c.OnAudioReady != nil {
		c.OnAudioReady(audioSink)
	}
	args := append([]string(nil), c.Args...)
	var materialFiles []string
	defer func() {
		for _, path := range materialFiles {
			_ = os.Remove(path)
		}
	}()
	for _, material := range c.Files {
		if material.Placeholder == "" || !strings.Contains(strings.Join(args, "\x00"), material.Placeholder) {
			return errors.New("invalid protected-file placeholder")
		}
		f, err := os.CreateTemp("", "thinpi-session-material-*")
		if err != nil {
			return err
		}
		path := f.Name()
		materialFiles = append(materialFiles, path)
		if _, err = f.WriteString(material.Content); err == nil {
			err = f.Chmod(0600)
		}
		if err == nil && credential != nil {
			err = f.Chown(int(credential.Uid), int(credential.Gid))
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], material.Placeholder, path)
		}
	}
	var passwordFile string
	if c.Password != "" {
		tool, err := exec.LookPath("vncpasswd")
		if err != nil {
			return errors.New("vncpasswd is unavailable")
		}
		filter := exec.CommandContext(ctx, tool, "-f")
		filter.Stdin = strings.NewReader(c.Password + "\n")
		var encoded bytes.Buffer
		filter.Stdout = &encoded
		if err = filter.Run(); err != nil {
			return errors.New("could not prepare VNC credential")
		}
		f, err := os.CreateTemp("", "thinpi-vnc-password-*")
		if err != nil {
			return err
		}
		passwordFile = f.Name()
		defer os.Remove(passwordFile)
		if _, err = f.Write(encoded.Bytes()); err != nil {
			f.Close()
			return err
		}
		if err = f.Chmod(0600); err == nil && credential != nil {
			err = f.Chown(int(credential.Uid), int(credential.Gid))
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			return err
		}
		for i := range args {
			args[i] = strings.ReplaceAll(args[i], "{password_file}", passwordFile)
		}
	}
	cmd := exec.CommandContext(ctx, c.Path, args...)
	output := &boundedOutput{limit: 32768, updated: make(chan struct{}, 1)}
	cmd.Stdout = output
	cmd.Stderr = output
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	// The root daemon owns the device credential. Native clients drop to the
	// kiosk identity so Xorg, audio, input and Moonlight pairing state work in
	// the same session as the launcher.
	configure(cmd)
	err = cmd.Start()
	if err == nil && c.OnStarted != nil {
		c.OnStarted(cmd.Process.Pid)
	}
	if err == nil {
		err = waitForNativeClient(cmd, output, c)
	}
	if err != nil && ctx.Err() == nil {
		diagnostic := redactClientOutput(output.String(), c)
		if strings.TrimSpace(diagnostic) == "" {
			diagnostic = err.Error()
		} else {
			diagnostic = err.Error() + ": " + diagnostic
		}
		if clientExitedNormally(diagnostic) {
			return nil
		}
		return classifyClientFailure(diagnostic)
	}
	return err
}

func waitForNativeClient(cmd *exec.Cmd, output *boundedOutput, command Command) error {
	if command.MoonlightPairing == nil {
		return cmd.Wait()
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	for {
		select {
		case err := <-wait:
			return err
		case <-output.updated:
			diagnostic := redactClientOutput(output.String(), command)
			if earlyFailure := moonlightFatalStartupFailure(diagnostic); earlyFailure != nil {
				if cmd.Cancel != nil {
					_ = cmd.Cancel()
				} else {
					_ = cmd.Process.Kill()
				}
				<-wait
				return earlyFailure
			}
		}
	}
}

func nativeSessionIdentity() (*syscall.Credential, string) {
	var credential *syscall.Credential
	var sessionHome string
	if u, err := user.Lookup("thinpi"); err == nil {
		uid, uidErr := strconv.ParseUint(u.Uid, 10, 32)
		gid, gidErr := strconv.ParseUint(u.Gid, 10, 32)
		if uidErr == nil && gidErr == nil {
			credential = &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}
			if ids, err := u.GroupIds(); err == nil {
				for _, id := range ids {
					if n, err := strconv.ParseUint(id, 10, 32); err == nil {
						credential.Groups = append(credential.Groups, uint32(n))
					}
				}
			}
			sessionHome = u.HomeDir
		}
	}
	return credential, sessionHome
}

func configureNativeCommand(cmd *exec.Cmd, credential *syscall.Credential, sessionHome string, environment []string) {
	if len(environment) > 0 {
		cmd.Env = append(os.Environ(), environment...)
	}
	if credential == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential, Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	runtimeDir := nativeRuntimeDir(credential)
	audioDriver := nativeAudioDriver()
	if nativeEnvironmentFlag(environment, "THINPI_AUDIO_DISABLED") {
		audioDriver = "dummy"
	}
	cmd.Env = append(os.Environ(), "HOME="+sessionHome, "USER=thinpi", "LOGNAME=thinpi",
		"THINPI_SESSION_HOME="+sessionHome, "XDG_RUNTIME_DIR="+runtimeDir,
		"DBUS_SESSION_BUS_ADDRESS=unix:path="+runtimeDir+"/bus",
		"PULSE_SERVER=unix:"+runtimeDir+"/pulse/native",
		"SDL_AUDIODRIVER="+audioDriver, "SDL_AUDIO_DRIVER="+audioDriver)
	if audioDriver == "alsa" && strings.Contains(strings.ToLower(filepath.Base(cmd.Path)), "moonlight") {
		if device := nativeALSAAudioDevice(); device != "" {
			// Moonlight 6.1 on Raspberry Pi uses SDL2, which reads AUDIODEV.
			// Set the SDL3 names too so a package upgrade keeps using the
			// discovered system playback device rather than an arbitrary card.
			cmd.Env = append(cmd.Env, "AUDIODEV="+device,
				"SDL_AUDIO_ALSA_DEFAULT_DEVICE="+device,
				"SDL_AUDIO_ALSA_DEFAULT_PLAYBACK_DEVICE="+device)
			slog.Info("Moonlight audio output selected", "driver", audioDriver,
				"device", device)
		} else {
			slog.Warn("Moonlight could not find a writable ALSA playback device")
		}
	}
	cmd.Env = append(cmd.Env, environment...)
}

func nativeAudioDriver() string {
	if configured := strings.ToLower(strings.TrimSpace(os.Getenv("THINPI_AUDIO_DRIVER"))); configured == "alsa" || configured == "pulseaudio" {
		return configured
	}
	return "pulseaudio"
}

func nativeRuntimeDir(credential *syscall.Credential) string {
	if configured := strings.TrimSpace(os.Getenv("THINPI_SESSION_RUNTIME_DIR")); configured != "" {
		return configured
	}
	const piRuntimeDir = "/run/thinpi-session"
	if info, err := os.Stat(piRuntimeDir); err == nil && info.IsDir() {
		return piRuntimeDir
	}
	return "/run/user/" + strconv.FormatUint(uint64(credential.Uid), 10)
}

func prepareMoonlightAudio(ctx context.Context, command Command, configure func(*exec.Cmd)) (string, error) {
	if command.MoonlightPairing == nil || nativeEnvironmentFlag(command.Env, "THINPI_AUDIO_DISABLED") || nativeAudioDriver() != "pulseaudio" {
		return "", nil
	}
	if !nativeRaspberryPi() {
		pactl, err := exec.LookPath("pactl")
		if err != nil {
			return "", nil
		}
		probe := exec.CommandContext(ctx, pactl, "list", "short", "sinks")
		configure(probe)
		output, probeErr := probe.CombinedOutput()
		if probeErr != nil || !nativePulseOutputAvailable(string(output)) {
			diagnostic := strings.TrimSpace(string(output))
			if diagnostic == "" && probeErr != nil {
				diagnostic = probeErr.Error()
			}
			return "", &AudioUnavailableError{
				Message:    "No usable audio output is available for this connection.",
				Diagnostic: diagnostic,
			}
		}
		return "", nil
	}
	device := nativeALSAAudioDevice()
	if device == "" {
		return "", &AudioUnavailableError{
			Message:    "ThinPi could not find a connected audio playback device.",
			Diagnostic: "No ALSA playback PCM was discovered for the Raspberry Pi kiosk session.",
		}
	}
	helper := nativePiAudioHelper()
	digest := sha256.Sum256([]byte(device))
	sinkName := fmt.Sprintf("thinpi_%x", digest[:8])
	prepare := exec.CommandContext(ctx, helper, "prepare", device, sinkName)
	configure(prepare)
	output, err := prepare.CombinedOutput()
	if err != nil {
		diagnostic := strings.TrimSpace(string(output))
		if diagnostic == "" {
			diagnostic = err.Error()
		}
		slog.Error("Moonlight audio output preparation failed", "device", device,
			"error", diagnostic)
		return "", &AudioUnavailableError{
			Message:    "ThinPi could not prepare the selected audio output.",
			Diagnostic: diagnostic,
		}
	}
	slog.Info("Moonlight audio output prepared", "driver", "pulseaudio",
		"device", device, "sink", sinkName)
	return sinkName, nil
}

func nativePiAudioHelper() string {
	helper := strings.TrimSpace(os.Getenv("THINPI_PI_AUDIO_HELPER"))
	if helper == "" {
		helper = "/usr/local/libexec/thinpi-prepare-pi-audio"
	}
	return helper
}

func (PlatformRunner) SetAudioSuspended(sink string, suspended bool) error {
	action := "resume"
	if suspended {
		action = "suspend"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	credential, sessionHome := nativeSessionIdentity()
	cmd := exec.CommandContext(ctx, nativePiAudioHelper(), action, sink)
	configureNativeCommand(cmd, credential, sessionHome, nil)
	output, err := cmd.CombinedOutput()
	if err != nil {
		diagnostic := strings.TrimSpace(string(output))
		if diagnostic == "" {
			diagnostic = err.Error()
		}
		return fmt.Errorf("%s ThinPi audio sink %s: %s", action, sink, diagnostic)
	}
	return nil
}

func nativeEnvironmentFlag(environment []string, name string) bool {
	prefix := name + "="
	for i := len(environment) - 1; i >= 0; i-- {
		if strings.HasPrefix(environment[i], prefix) {
			value := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(environment[i], prefix)))
			return value == "1" || value == "true" || value == "yes"
		}
	}
	return false
}

func nativePulseOutputAvailable(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.ToLower(fields[1])
		driver := ""
		if len(fields) > 2 {
			driver = strings.ToLower(fields[2])
		}
		if !strings.Contains(name, "auto_null") && !strings.Contains(driver, "module-null-sink") {
			return true
		}
	}
	return false
}

func nativeRaspberryPi() bool {
	model, _ := os.ReadFile("/proc/device-tree/model")
	return bytes.Contains(model, []byte("Raspberry Pi"))
}

func nativeALSAAudioDevice() string {
	if configured := strings.TrimSpace(os.Getenv("THINPI_ALSA_DEVICE")); configured != "" {
		return configured
	}
	if !nativeRaspberryPi() {
		return ""
	}
	asoundRoot := os.Getenv("THINPI_ASOUND_ROOT")
	if asoundRoot == "" {
		asoundRoot = "/proc/asound"
	}
	drmRoot := os.Getenv("THINPI_DRM_SYSFS_ROOT")
	if drmRoot == "" {
		drmRoot = "/sys/class/drm"
	}
	// Do not open the device to validate it here. HDMI ALSA playback is
	// exclusive and a pre-launch probe can still own the device when Moonlight
	// starts, intermittently making Moonlight's audio initialization fail.
	return piALSAAudioDevice(asoundRoot, drmRoot)
}

func clientExitedNormally(output string) bool {
	upper := strings.ToUpper(output)
	return strings.Contains(upper, "ERRINFO_LOGOFF_BY_USER") ||
		strings.Contains(upper, "ERRINFO_RPC_INITIATED_LOGOFF") ||
		strings.Contains(upper, "ERRINFO_RPC_INITIATED_DISCONNECT")
}

func classifyClientFailure(output string) error {
	upper := strings.ToUpper(output)
	if audioFailure := moonlightFatalStartupFailure(output); audioFailure != nil {
		return audioFailure
	}
	switch {
	case strings.Contains(upper, "FAILED TO OPEN DISPLAY") || strings.Contains(upper, "CANNOT OPEN DISPLAY"):
		return &ClientRuntimeError{Message: "The remote client could not access the ThinPi display. Restart the ThinPi UI and try again.", Diagnostic: output}
	case strings.Contains(upper, "ERRCONNECT_LOGON_FAILURE") || strings.Contains(upper, "STATUS_LOGON_FAILURE") || strings.Contains(upper, "AUTHENTICATION FAILURE"):
		return &ClientRuntimeError{Message: "The remote system rejected the assigned username or password.", Diagnostic: output}
	case strings.Contains(upper, "PASSWORD_EXPIRED"):
		return &ClientRuntimeError{Message: "The assigned remote password has expired.", Diagnostic: output}
	case strings.Contains(upper, "ACCOUNT_LOCKED"):
		return &ClientRuntimeError{Message: "The assigned remote account is locked.", Diagnostic: output}
	case strings.Contains(upper, "CERTIFICATE") && (strings.Contains(upper, "MISMATCH") || strings.Contains(upper, "CHANGED")):
		return &ClientRuntimeError{Message: "The remote system certificate changed. An administrator must verify it before reconnecting.", Diagnostic: output}
	case strings.Contains(upper, "ERRCONNECT_TLS_CONNECT_FAILED"):
		return &ClientRuntimeError{Message: "The remote system rejected the secure RDP handshake. Check its TLS and RDP security settings.", Diagnostic: output}
	case strings.Contains(upper, "ERRCONNECT_CONNECT_TRANSPORT_FAILED") || strings.Contains(upper, "CONNECTION REFUSED") || strings.Contains(upper, "NO ROUTE TO HOST"):
		return &ClientRuntimeError{Message: "The ThinPi client could not reach the configured remote host and port.", Diagnostic: output}
	case strings.Contains(upper, "COMMAND LINE") && (strings.Contains(upper, "ERROR") || strings.Contains(upper, "FAILED")):
		return &ClientRuntimeError{Message: "The installed remote client rejected this connection configuration.", Diagnostic: output}
	default:
		return &ClientRuntimeError{Message: "The remote client exited unexpectedly. An administrator can inspect: journalctl -b -u thinpi-agent -o cat --no-pager", Diagnostic: output}
	}
}

func moonlightFatalStartupFailure(output string) error {
	upper := strings.ToUpper(output)
	if strings.Contains(upper, "FAILED TO OPEN AUDIO DEVICE") ||
		strings.Contains(upper, "NO AVAILABLE AUDIO DEVICE") ||
		strings.Contains(upper, "AUDIO DEVICE") && strings.Contains(upper, "FAILED") {
		return &AudioUnavailableError{Message: "The remote client could not open the selected audio output.", Diagnostic: output}
	}
	return nil
}
