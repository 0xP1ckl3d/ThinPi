//go:build !windows

package launch

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

type PlatformRunner struct{}

func (PlatformRunner) Run(ctx context.Context, c Command) error {
	if c.Path == "thinpi-mock" {
		return runMock(ctx, c)
	}
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
	configure := func(cmd *exec.Cmd) {
		configureNativeCommand(cmd, credential, sessionHome, c.Env)
	}
	if err := ensureMoonlightPaired(ctx, c, configure); err != nil {
		return err
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
	output := &boundedOutput{limit: 32768}
	cmd.Stdout = output
	cmd.Stderr = output
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	// The root daemon owns the device credential. Native clients drop to the
	// kiosk identity so Xorg, audio, input and Moonlight pairing state work in
	// the same session as the launcher.
	configure(cmd)
	err := cmd.Start()
	if err == nil && c.OnStarted != nil {
		c.OnStarted(cmd.Process.Pid)
	}
	if err == nil {
		err = cmd.Wait()
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
	cmd.Env = append(os.Environ(), "HOME="+sessionHome, "USER=thinpi", "LOGNAME=thinpi", "THINPI_SESSION_HOME="+sessionHome)
	cmd.Env = append(cmd.Env, environment...)
}

func clientExitedNormally(output string) bool {
	upper := strings.ToUpper(output)
	return strings.Contains(upper, "ERRINFO_LOGOFF_BY_USER") ||
		strings.Contains(upper, "ERRINFO_RPC_INITIATED_LOGOFF") ||
		strings.Contains(upper, "ERRINFO_RPC_INITIATED_DISCONNECT")
}

func classifyClientFailure(output string) error {
	upper := strings.ToUpper(output)
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
