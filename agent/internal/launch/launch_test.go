package launch

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"thinpi.local/agent/internal/api"
)

func TestFreeRDPPasswordOnlyInStdin(t *testing.T) {
	m := api.Manifest{Protocol: "rdp", Host: "server.example", Port: 3389, Username: "alice", Password: "very-secret", Config: json.RawMessage(`{"fullscreen":true,"clipboard":false}`)}
	c, err := FreeRDPCommand("xfreerdp3", m)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append([]string{c.Path}, c.Args...), " ")
	if strings.Contains(joined, m.Password) {
		t.Fatal("password leaked into argv")
	}
	if !strings.Contains(c.Stdin, "/p:"+m.Password) {
		t.Fatal("password absent from secure stdin")
	}
	if len(c.Args) != 1 || c.Args[0] != "/args-from:stdin" {
		t.Fatalf("unsafe argv: %#v", c.Args)
	}
	if !strings.Contains(c.Stdin, "/cert:tofu") {
		t.Fatal("RDP connection can still block on an interactive certificate prompt")
	}
	if !strings.Contains(c.Stdin, "+clipboard") {
		t.Fatal("RDP was not attached to the signed-in kiosk clipboard")
	}
}

func TestFreeRDPCertificateModes(t *testing.T) {
	base := api.Manifest{Protocol: "rdp", Host: "server.example", Port: 3389}
	for _, mode := range []string{"tofu", "deny", "ignore"} {
		base.Config = json.RawMessage(`{"certificate_mode":"` + mode + `"}`)
		command, err := FreeRDPCommand("xfreerdp3", base)
		if err != nil || !strings.Contains(command.Stdin, "/cert:"+mode) {
			t.Fatalf("mode %q not enforced: %#v %v", mode, command, err)
		}
	}
	base.Config = json.RawMessage(`{"certificate_mode":"prompt"}`)
	if _, err := FreeRDPCommand("xfreerdp3", base); err == nil {
		t.Fatal("interactive certificate mode was accepted")
	}
}
func TestRejectsUnsafeManifest(t *testing.T) {
	_, err := FreeRDPCommand("xfreerdp", api.Manifest{Host: "host\n/p:injected", Port: 3389, Config: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatal("unsafe host accepted")
	}
	_, err = MoonlightCommand("moonlight-qt", api.Manifest{Host: "good", Port: 47984, Config: json.RawMessage(`{"application":"Bad\nApp","width":1920,"height":1080,"fps":60,"bitrate_kbps":20000}`)})
	if err == nil {
		t.Fatal("unsafe app accepted")
	}
}
func TestMoonlightDirectLaunch(t *testing.T) {
	c, err := MoonlightCommand("moonlight-qt", api.Manifest{Host: "gaming.local", Port: 47984, Username: "sunshine-admin", Password: "pairing-secret", Config: json.RawMessage(`{"application":"Desktop","width":1920,"height":1080,"fps":60,"bitrate_kbps":20000}`)})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(c.Args, " ")
	for _, want := range []string{"stream gaming.local Desktop", "--resolution 1920x1080", "--fps 60", "--bitrate 20000", "--display-mode fullscreen", "--video-decoder auto", "--video-codec H.264", "--audio-config stereo"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "--video-decoder hardware") {
		t.Fatal("Moonlight hardware decoding was forced on an unsupported client")
	}
	if strings.Contains(joined, "sunshine-admin") || strings.Contains(joined, "pairing-secret") {
		t.Fatal("Sunshine administrator credential leaked into Moonlight arguments")
	}
	if c.MoonlightPairing == nil || c.MoonlightPairing.Host != "gaming.local" || c.MoonlightPairing.Username != "sunshine-admin" || c.MoonlightPairing.Password != "pairing-secret" {
		t.Fatalf("automatic pairing metadata missing: %#v", c.MoonlightPairing)
	}
	if c.MoonlightPairing.SunshineAPIPort != 47990 || c.MoonlightPairing.ClientName != "ThinPi" {
		t.Fatalf("unexpected automatic pairing defaults: %#v", c.MoonlightPairing)
	}
}

func TestPiALSAAudioCandidatesPreferPhysicalThenConnectedHDMI(t *testing.T) {
	asoundRoot := t.TempDir()
	drmRoot := t.TempDir()
	disconnected := filepath.Join(drmRoot, "card1-HDMI-A-1")
	connected := filepath.Join(drmRoot, "card1-HDMI-A-2")
	for _, directory := range []string{disconnected, connected} {
		if err := os.MkdirAll(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(disconnected, "status"), []byte("disconnected\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(connected, "status"), []byte("connected\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cards := " 0 [vc4hdmi0     ]: vc4-hdmi - vc4-hdmi-0\n 1 [vc4hdmi1     ]: vc4-hdmi - vc4-hdmi-1\n 2 [USB            ]: USB-Audio - USB Speakers\n"
	pcms := "00-00: MAI PCM : playback 1\n01-00: MAI PCM : playback 1\n02-00: USB Audio : playback 1 : capture 1\n"
	if err := os.WriteFile(filepath.Join(asoundRoot, "cards"), []byte(cards), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(asoundRoot, "pcm"), []byte(pcms), 0644); err != nil {
		t.Fatal(err)
	}
	want := []string{"plughw:CARD=USB,DEV=0", "plughw:CARD=vc4hdmi1,DEV=0", "plughw:CARD=vc4hdmi0,DEV=0"}
	if got := piALSAAudioCandidates(asoundRoot, drmRoot); !slices.Equal(got, want) {
		t.Fatalf("unexpected ALSA device order: got %#v want %#v", got, want)
	}
}

func TestVNCTLSPlainUsesEnvironmentCredentials(t *testing.T) {
	manifest := api.Manifest{Protocol: "vnc", Host: "linux.example", Port: 5900, Username: "student", Password: "vnc-secret", Config: json.RawMessage(`{"fullscreen":true,"shared":true,"clipboard":false}`)}
	command, err := VNCCommand("xtigervncviewer", manifest)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Args, " ")
	if strings.Contains(joined, manifest.Password) || command.Stdin != "" || command.Password != "" {
		t.Fatal("TLSPlain VNC password leaked into argv/stdin or selected legacy VncAuth")
	}
	if strings.Contains(joined, "-PasswordFile") || !strings.Contains(joined, "-SecurityTypes=TLSPlain") {
		t.Fatalf("TLSPlain authentication not configured: %#v", command)
	}
	if !strings.Contains(joined, "linux.example::5900") {
		t.Fatalf("unexpected VNC target: %s", joined)
	}
	if strings.Contains(joined, manifest.Username) || !slices.Contains(command.Env, "VNC_USERNAME="+manifest.Username) {
		t.Fatalf("VNC username not isolated in its supported environment setting: %#v", command)
	}
	if !slices.Contains(command.Env, "VNC_PASSWORD="+manifest.Password) {
		t.Fatalf("VNC password not provided through TLSPlain environment setting: %#v", command)
	}
	if !strings.Contains(joined, "-AcceptClipboard=1") || !strings.Contains(joined, "-SendClipboard=1") {
		t.Fatalf("VNC was not attached to the signed-in kiosk clipboard: %s", joined)
	}
}

func TestVNCTLSPlainRejectsNULPassword(t *testing.T) {
	_, err := VNCCommand("xtigervncviewer", api.Manifest{Host: "linux.example", Port: 5900, Password: "bad\x00password"})
	if err == nil {
		t.Fatal("VNC password containing NUL was accepted for a process environment")
	}
}

func TestSSHUsesTrustedSinglePurposeTerminalWithoutLocalEscape(t *testing.T) {
	manifest := api.Manifest{Protocol: "ssh", Name: "Linux shell", Host: "server.example", Port: 22, Username: "student", Password: "ssh-secret", Config: json.RawMessage(`{"terminal_title":"Secure shell"}`)}
	command, err := SSHCommand("/usr/bin/xterm", "/usr/bin/ssh", "/usr/bin/sshpass", manifest)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append([]string{command.Path}, command.Args...), " ")
	if strings.Contains(joined, manifest.Password) || command.Stdin != "" {
		t.Fatal("SSH password leaked into argv or stdin")
	}
	for _, required := range []string{"/usr/bin/xterm", "XTerm*fullscreen: always", "XTerm*selectToClipboard: true", "XTerm*background: #07111e", "-e /usr/bin/sshpass -f {ssh_password_file} /usr/bin/ssh", "EscapeChar=none", "PermitLocalCommand=no", "ClearAllForwardings=yes", "StrictHostKeyChecking=yes", "UserKnownHostsFile=", "-l student server.example"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing hardening %q in %q", required, joined)
		}
	}
	if strings.Contains(joined, "DisableForwarding") {
		t.Fatal("SSH command contains the sshd-only DisableForwarding option")
	}
	if len(command.Files) != 1 || command.Files[0].Content != manifest.Password+"\n" {
		t.Fatalf("protected SSH files not prepared correctly: %#v", command.Files)
	}
}

func TestSSHDoesNotRequireConfiguredHostKey(t *testing.T) {
	base := api.Manifest{Protocol: "ssh", Host: "server.example", Port: 22, Username: "student", Config: json.RawMessage(`{}`)}
	if _, err := SSHCommand("xterm", "ssh", "sshpass", base); err != nil {
		t.Fatalf("SSH still requires an administrator-configured host key: %v", err)
	}
	base.Username = "student -oProxyCommand=sh"
	if _, err := SSHCommand("xterm", "ssh", "sshpass", base); err == nil {
		t.Fatal("SSH accepted an unsafe username")
	}
}

func TestSSHTerminalTheme(t *testing.T) {
	manifest := api.Manifest{Host: "server.example", Port: 22, Username: "student", Config: json.RawMessage(`{}`), TerminalTheme: "light"}
	command, err := SSHCommand("xterm", "ssh", "sshpass", manifest)
	if err != nil || !strings.Contains(strings.Join(command.Args, " "), "XTerm*background: #f4f6fa") {
		t.Fatalf("light terminal theme not applied: %#v %v", command, err)
	}
	manifest.TerminalTheme = "neon"
	if _, err = SSHCommand("xterm", "ssh", "sshpass", manifest); err == nil {
		t.Fatal("invalid terminal theme accepted")
	}
}

func TestSSHPrivateKeyUsesProtectedIdentityFile(t *testing.T) {
	manifest := api.Manifest{Protocol: "ssh", Host: "server.example", Port: 22, Username: "student", Password: "-----BEGIN PRIVATE KEY-----\nprivate material\n-----END PRIVATE KEY-----", CredentialType: "ssh_private_key", Config: json.RawMessage(`{}`)}
	command, err := SSHCommand("xterm", "ssh", "sshpass", manifest)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(command.Args, " ")
	if strings.Contains(joined, "private material") || !strings.Contains(joined, "IdentityFile={ssh_identity_file}") || strings.Contains(joined, "sshpass") {
		t.Fatalf("private key was not isolated: %#v", command)
	}
	if len(command.Files) != 1 || command.Files[0].Placeholder != "{ssh_identity_file}" {
		t.Fatalf("protected identity file missing: %#v", command.Files)
	}
}

type fakeController struct {
	manifest api.Manifest
	err      error
	mu       sync.Mutex
	events   []string
}

func (f *fakeController) Redeem(context.Context, string) (api.Manifest, error) {
	return f.manifest, f.err
}
func (f *fakeController) SessionEvent(_ context.Context, _, _ int64, event, _ string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (r *blockingRunner) Run(ctx context.Context, _ Command) error {
	close(r.started)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return r.err
	}
}

type concurrentRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *concurrentRunner) Run(ctx context.Context, _ Command) error {
	r.started <- struct{}{}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.release:
		return nil
	}
}

func TestConcurrentLaunchesRemainIndependent(t *testing.T) {
	fc := &fakeController{manifest: api.Manifest{ConnectionID: 1, Protocol: "mock", Host: "mock", Port: 1, Config: json.RawMessage(`{}`)}}
	runner := &concurrentRunner{started: make(chan struct{}, 2), release: make(chan struct{})}
	m := NewManager(fc, runner, true, time.Second, Clients{})
	first, err := m.Launch("valid")
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Launch("second")
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case <-runner.started:
		case <-time.After(time.Second):
			t.Fatal("runner did not start")
		}
	}
	status := m.Status()
	if len(status.Sessions) != 2 || first == second {
		t.Fatalf("concurrent sessions not tracked independently: %#v", status.Sessions)
	}
	close(runner.release)
	deadline := time.Now().Add(time.Second)
	for m.HasSessions() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if m.HasSessions() {
		t.Fatal("completed sessions were not removed")
	}
}
func TestRedeemFailureDoesNotRun(t *testing.T) {
	fc := &fakeController{err: errors.New("invalid ticket")}
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	m := NewManager(fc, runner, true, time.Millisecond, Clients{})
	_, _ = m.Launch("expired")
	select {
	case <-runner.started:
		t.Fatal("runner started after ticket failure")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestRunnerFailureIsVisibleToLauncher(t *testing.T) {
	fc := &fakeController{manifest: api.Manifest{TicketID: 4, ConnectionID: 1, Protocol: "mock", Host: "mock", Port: 1}}
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{}), err: errors.New("native diagnostics must not leak")}
	m := NewManager(fc, runner, true, time.Second, Clients{})
	if _, err := m.Launch("valid"); err != nil {
		t.Fatal(err)
	}
	<-runner.started
	close(runner.release)
	deadline := time.Now().Add(time.Second)
	for m.Status().State != Failed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := m.Status()
	if status.State != Failed || !strings.Contains(status.LastError, "agent journal") {
		t.Fatalf("unexpected status: %#v", status)
	}
	if strings.Contains(status.LastError, "native diagnostics") {
		t.Fatal("raw native-client error leaked to launcher")
	}
}

func TestUnavailableProductionClientReportsSpecificError(t *testing.T) {
	fc := &fakeController{manifest: api.Manifest{TicketID: 5, ConnectionID: 2, Protocol: "rdp", Host: "desktop.internal", Port: 3389}}
	runner := &blockingRunner{started: make(chan struct{}), release: make(chan struct{})}
	m := NewManager(fc, runner, false, time.Second, Clients{})
	if _, err := m.Launch("valid"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for m.Status().State != Failed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	status := m.Status()
	if status.State != Failed || status.LastError != "The Remote Desktop client is not installed on this ThinPi." {
		t.Fatalf("unexpected status: %#v", status)
	}
	select {
	case <-runner.started:
		t.Fatal("runner started without a native client")
	default:
	}
}

func TestMockRunnerHonoursDurationAndCancellation(t *testing.T) {
	start := time.Now()
	if err := runMock(context.Background(), MockCommand(80*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Fatalf("mock returned too early after %s", elapsed)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runMock(ctx, MockCommand(time.Second)); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}
}

type fakeWindowController struct {
	minimized string
	resumed   string
}

func (f *fakeWindowController) Minimize(pid int) (string, error) {
	if pid != 99 {
		return "", errors.New("wrong process")
	}
	f.minimized = "4242"
	return f.minimized, nil
}
func (f *fakeWindowController) Resume(windowID string) error {
	f.resumed = windowID
	return nil
}

func TestManagerMinimizesAndResumesSameActiveWindow(t *testing.T) {
	m := NewManager(&fakeController{}, &blockingRunner{}, false, time.Second, Clients{})
	windows := &fakeWindowController{}
	m.windows = windows
	sessionID := "session-1"
	m.sessions[sessionID] = &managedSession{status: SessionStatus{ID: sessionID, State: Active}, pid: 99}
	if err := m.Minimize(sessionID); err != nil {
		t.Fatal(err)
	}
	if status := m.Status(); !status.Minimized || windows.minimized != "4242" {
		t.Fatalf("session was not minimized: %#v", status)
	}
	if err := m.Resume(sessionID); err != nil {
		t.Fatal(err)
	}
	if status := m.Status(); status.Minimized || windows.resumed != "4242" {
		t.Fatalf("same session window was not resumed: %#v", status)
	}
}
