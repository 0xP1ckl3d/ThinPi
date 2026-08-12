package launch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"thinpi.local/agent/internal/api"
)

type State string

const (
	Idle      State = "idle"
	Redeeming State = "redeeming_ticket"
	Preparing State = "preparing"
	Starting  State = "starting_client"
	Active    State = "active"
	Stopping  State = "stopping"
	Failed    State = "error"
)

type ClientInfo struct {
	Available bool   `json:"available"`
	Binary    string `json:"binary,omitempty"`
	Version   string `json:"version,omitempty"`
}
type Status struct {
	State               State         `json:"state"`
	ActiveSession       *string       `json:"active_session"`
	ControllerReachable bool          `json:"controller_reachable"`
	FreeRDP             ClientInfo    `json:"freerdp"`
	Moonlight           ClientInfo    `json:"moonlight"`
	VNC                 ClientInfo    `json:"vnc"`
	SSH                 ClientInfo    `json:"ssh"`
	LastError           string        `json:"last_error,omitempty"`
	Confirmation        *Confirmation `json:"confirmation,omitempty"`
}
type Confirmation struct {
	Kind        string `json:"kind"`
	Message     string `json:"message"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Fingerprint string `json:"fingerprint,omitempty"`
}
type Controller interface {
	Redeem(context.Context, string) (api.Manifest, error)
	SessionEvent(context.Context, int64, int64, string, string, any) error
}
type Command struct {
	Path             string
	Args             []string
	Stdin            string
	Password         string
	Env              []string
	Files            []FileInput
	MoonlightPairing *MoonlightPairing
}

type MoonlightPairing struct {
	Host            string
	Username        string
	Password        string
	SunshineAPIPort int
	ClientName      string
}
type FileInput struct {
	Placeholder string
	Content     string
}
type Clients struct {
	FreeRDP   ClientInfo
	Moonlight ClientInfo
	VNC       ClientInfo
	SSH       ClientInfo
	Terminal  ClientInfo
	SSHPass   ClientInfo
}
type Runner interface {
	Run(context.Context, Command) error
}

type ClientRuntimeError struct {
	Message    string
	Diagnostic string
}

func (e *ClientRuntimeError) Error() string { return e.Message }

type boundedOutput struct {
	limit int
	data  []byte
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	written := len(p)
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = append([]byte(nil), b.data[len(b.data)-b.limit:]...)
	}
	return written, nil
}

func (b *boundedOutput) String() string { return string(b.data) }

func redactClientOutput(output string, command Command) string {
	secrets := []string{}
	for _, line := range strings.Split(command.Stdin, "\n") {
		if strings.HasPrefix(line, "/p:") && len(line) > 3 {
			secrets = append(secrets, line[3:])
		}
	}
	if command.Password != "" {
		secrets = append(secrets, command.Password)
	}
	if command.MoonlightPairing != nil {
		secrets = append(secrets, command.MoonlightPairing.Password)
	}
	for _, material := range command.Files {
		if strings.Contains(material.Placeholder, "password") || strings.Contains(material.Placeholder, "identity") {
			secret := strings.TrimSpace(material.Content)
			if secret != "" {
				secrets = append(secrets, secret)
			}
		}
	}
	for _, secret := range secrets {
		if secret != "" {
			output = strings.ReplaceAll(output, secret, "[redacted]")
		}
	}
	output = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r >= 32 && r != 127 {
			return r
		}
		return -1
	}, output)
	return strings.TrimSpace(output)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Command) error {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	if c.Stdin != "" {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	return cmd.Run()
}

type Manager struct {
	mu           sync.Mutex
	status       Status
	controller   Controller
	runner       Runner
	mock         bool
	mockDuration time.Duration
	cancel       context.CancelFunc
	log          *slog.Logger
	clients      Clients
	sshTrust     *SSHTrustStore
	pendingSSH   *PendingSSHHostKey
}

func NewManager(c Controller, r Runner, mock bool, d time.Duration, clients Clients) *Manager {
	return &Manager{controller: c, runner: r, mock: mock, mockDuration: d, clients: clients, sshTrust: NewSSHTrustStore(defaultSSHKnownHostsPath()), status: Status{State: Idle, ControllerReachable: true, FreeRDP: clients.FreeRDP, Moonlight: clients.Moonlight, VNC: clients.VNC, SSH: clients.SSH}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func (m *Manager) SetLogger(log *slog.Logger) {
	if log != nil {
		m.log = log
	}
}
func (m *Manager) Status() Status { m.mu.Lock(); defer m.mu.Unlock(); return m.status }
func (m *Manager) set(state State, id *string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.State = state
	m.status.ActiveSession = id
	if err != nil {
		m.status.LastError = err.Error()
	} else if state == Idle {
		m.status.LastError = ""
		m.status.Confirmation = nil
	}
}
func (m *Manager) Launch(ticket string) (string, error) {
	m.mu.Lock()
	if m.status.State != Idle {
		m.mu.Unlock()
		return "", errors.New("another interactive session is already active")
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	m.status.State = Redeeming
	m.status.ActiveSession = &id
	m.status.LastError = ""
	m.status.Confirmation = nil
	m.pendingSSH = nil
	m.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()
	go m.run(ctx, id, ticket)
	return id, nil
}
func (m *Manager) run(ctx context.Context, id, ticket string) {
	defer func() { m.mu.Lock(); m.cancel = nil; m.mu.Unlock() }()
	manifest, err := m.controller.Redeem(ctx, ticket)
	if err != nil {
		m.mu.Lock()
		m.status.ControllerReachable = false
		m.mu.Unlock()
		m.set(Failed, nil, errors.New("The controller rejected this launch. Refresh your connections and try again."))
		time.AfterFunc(2*time.Second, func() { m.set(Idle, nil, nil) })
		return
	}
	m.mu.Lock()
	m.status.ControllerReachable = true
	m.mu.Unlock()
	m.set(Preparing, &id, nil)
	m.log.Info("launch manifest redeemed", "connection_id", manifest.ConnectionID, "protocol", manifest.Protocol)
	var cmd Command
	if m.mock {
		cmd = MockCommand(m.mockDuration)
	} else {
		if manifest.Protocol == "ssh" {
			pending, trustErr := m.sshTrust.Prepare(ctx, manifest.Host, manifest.Port)
			if trustErr != nil {
				var changed *SSHHostKeyChangedError
				if errors.As(trustErr, &changed) {
					m.mu.Lock()
					m.pendingSSH = pending
					m.status.State = Failed
					m.status.ActiveSession = nil
					m.status.LastError = changed.Error()
					m.status.Confirmation = &Confirmation{Kind: "ssh_host_key_changed", Message: changed.Error(), Host: changed.Host, Port: changed.Port, Fingerprint: changed.Fingerprint}
					m.mu.Unlock()
					_ = m.controller.SessionEvent(ctx, manifest.TicketID, manifest.ConnectionID, "session_failed", "ssh_host_key_changed", map[string]string{"fingerprint": changed.Fingerprint})
					return
				}
				err = trustErr
			}
		}
		if err == nil {
			cmd, err = m.commandFor(manifest)
		}
	}
	if err != nil {
		m.set(Failed, nil, clientPreparationError(err))
		_ = m.controller.SessionEvent(ctx, manifest.TicketID, manifest.ConnectionID, "session_failed", "client_unavailable", nil)
		time.AfterFunc(2*time.Second, func() { m.set(Idle, nil, nil) })
		return
	}
	m.set(Starting, &id, nil)
	_ = m.controller.SessionEvent(ctx, manifest.TicketID, manifest.ConnectionID, "session_started", "success", map[string]string{"protocol": manifest.Protocol})
	m.set(Active, &id, nil)
	m.log.Info("native client active", "connection_id", manifest.ConnectionID, "protocol", manifest.Protocol)
	runCtx := ctx
	var timeoutCancel context.CancelFunc
	if manifest.MaxSessionSeconds > 0 {
		runCtx, timeoutCancel = context.WithTimeout(ctx, time.Duration(manifest.MaxSessionSeconds)*time.Second)
		defer timeoutCancel()
	}
	err = m.runner.Run(runCtx, cmd)
	result := "success"
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result = "timeout"
	} else if err != nil {
		result = "failed"
	}
	_ = m.controller.SessionEvent(context.Background(), manifest.TicketID, manifest.ConnectionID, "session_exited", result, map[string]any{"error": err != nil})
	m.log.Info("native client exited", "connection_id", manifest.ConnectionID, "protocol", manifest.Protocol, "result", result)
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		m.set(Failed, nil, errors.New("This session reached its administrator-set time limit."))
		time.AfterFunc(2*time.Second, func() { m.set(Idle, nil, nil) })
		return
	}
	if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		failure := errors.New("The installed remote client exited unexpectedly. Ask an administrator to inspect the ThinPi agent journal.")
		var runtimeError *ClientRuntimeError
		if errors.As(err, &runtimeError) {
			failure = errors.New(runtimeError.Message)
			m.log.Warn("native client diagnostic", "connection_id", manifest.ConnectionID, "diagnostic", runtimeError.Diagnostic)
		}
		m.log.Warn("native client failure", "connection_id", manifest.ConnectionID, "detail", failure.Error())
		m.set(Failed, nil, failure)
		time.AfterFunc(2*time.Second, func() { m.set(Idle, nil, nil) })
		return
	}
	m.set(Stopping, &id, nil)
	m.set(Idle, nil, nil)
}

func (m *Manager) ResolveSSHHostKeyChange(accept bool) error {
	m.mu.Lock()
	pending := m.pendingSSH
	confirmation := m.status.Confirmation
	m.mu.Unlock()
	if pending == nil || confirmation == nil || confirmation.Kind != "ssh_host_key_changed" {
		return errors.New("no SSH host-key change is awaiting confirmation")
	}
	if accept {
		if err := m.sshTrust.Accept(pending); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.pendingSSH = nil
	m.status.State = Idle
	m.status.ActiveSession = nil
	m.status.LastError = ""
	m.status.Confirmation = nil
	m.mu.Unlock()
	return nil
}
func clientPreparationError(err error) error {
	switch err.Error() {
	case "FreeRDP is unavailable":
		return errors.New("The Remote Desktop client is not installed on this ThinPi.")
	case "Moonlight is unavailable":
		return errors.New("The Moonlight client is not installed on this ThinPi.")
	case "TigerVNC Viewer is unavailable":
		return errors.New("The VNC client is not installed on this ThinPi.")
	case "SSH client is unavailable":
		return errors.New("The secure SSH terminal is not installed on this ThinPi.")
	case "unsupported protocol":
		return errors.New("This connection type is not supported by the installed agent.")
	default:
		return errors.New("This connection is misconfigured. Ask an administrator to review its host, port, credentials, and protocol settings.")
	}
}
func (m *Manager) commandFor(x api.Manifest) (Command, error) {
	switch x.Protocol {
	case "rdp":
		if !m.status.FreeRDP.Available {
			return Command{}, errors.New("FreeRDP is unavailable")
		}
		return FreeRDPCommand(m.status.FreeRDP.Binary, x)
	case "moonlight":
		if !m.status.Moonlight.Available {
			return Command{}, errors.New("Moonlight is unavailable")
		}
		return MoonlightCommand(m.status.Moonlight.Binary, x)
	case "vnc":
		if !m.status.VNC.Available {
			return Command{}, errors.New("TigerVNC Viewer is unavailable")
		}
		return VNCCommand(m.status.VNC.Binary, x)
	case "ssh":
		if !m.status.SSH.Available || !m.clients.Terminal.Available || (x.Password != "" && !m.clients.SSHPass.Available) {
			return Command{}, errors.New("SSH client is unavailable")
		}
		return SSHCommand(m.clients.Terminal.Binary, m.status.SSH.Binary, m.clients.SSHPass.Binary, x)
	default:
		return Command{}, errors.New("unsupported protocol")
	}
}
func (m *Manager) Cancel() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel == nil {
		return errors.New("no active session")
	}
	m.status.State = Stopping
	m.cancel()
	return nil
}

var safeHost = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,252}$`)

func validateHost(host string, port int) error {
	if !safeHost.MatchString(host) || port < 1 || port > 65535 {
		return errors.New("invalid host or port")
	}
	return nil
}

type RDPConfig struct {
	Fullscreen        bool   `json:"fullscreen"`
	DynamicResolution bool   `json:"dynamic_resolution"`
	Audio             bool   `json:"audio"`
	Microphone        bool   `json:"microphone"`
	Clipboard         bool   `json:"clipboard"`
	Drives            bool   `json:"drives"`
	Printers          bool   `json:"printers"`
	Smartcards        bool   `json:"smartcards"`
	AutoReconnect     bool   `json:"auto_reconnect"`
	CertificateName   string `json:"certificate_name,omitempty"`
	CertificateMode   string `json:"certificate_mode,omitempty"`
}

func FreeRDPCommand(binary string, x api.Manifest) (Command, error) {
	if err := validateHost(x.Host, x.Port); err != nil {
		return Command{}, err
	}
	if x.CredentialType == "ssh_private_key" {
		return Command{}, errors.New("an SSH private key cannot be used for RDP")
	}
	if strings.ContainsAny(x.Username, "\r\n") || strings.ContainsAny(x.Password, "\r\n") {
		return Command{}, errors.New("credential contains invalid characters")
	}
	cfg := RDPConfig{Fullscreen: true, DynamicResolution: true, Audio: true, AutoReconnect: true}
	if len(x.Config) > 0 {
		if err := json.Unmarshal(x.Config, &cfg); err != nil {
			return Command{}, err
		}
	}
	args := []string{"/v:" + net.JoinHostPort(x.Host, strconv.Itoa(x.Port))}
	if x.Username != "" {
		args = append(args, "/u:"+x.Username)
	}
	if x.Password != "" {
		args = append(args, "/p:"+x.Password)
	}
	if cfg.Fullscreen {
		args = append(args, "/f")
	}
	if cfg.DynamicResolution {
		args = append(args, "/dynamic-resolution")
	}
	if cfg.Audio {
		args = append(args, "/sound")
	}
	if cfg.Microphone {
		args = append(args, "/microphone")
	}
	if cfg.Clipboard {
		args = append(args, "+clipboard")
	} else {
		args = append(args, "-clipboard")
	}
	if cfg.Drives {
		home := os.Getenv("THINPI_SESSION_HOME")
		if home == "" {
			home = "/home/thinpi"
		}
		if home == "" || strings.ContainsAny(home, "\r\n") {
			return Command{}, errors.New("safe home directory is unavailable")
		}
		args = append(args, "/drive:home,"+home)
	}
	if cfg.Printers {
		args = append(args, "+printer")
	}
	if cfg.Smartcards {
		args = append(args, "/smartcard")
	}
	if cfg.AutoReconnect {
		args = append(args, "+auto-reconnect")
	}
	if cfg.CertificateName != "" {
		if strings.ContainsAny(cfg.CertificateName, "\r\n/") {
			return Command{}, errors.New("invalid certificate name")
		}
		args = append(args, "/cert-name:"+cfg.CertificateName)
	}
	certificateMode := cfg.CertificateMode
	if certificateMode == "" {
		certificateMode = "tofu"
	}
	switch certificateMode {
	case "tofu":
		args = append(args, "/cert:tofu")
	case "deny":
		args = append(args, "/cert:deny")
	case "ignore":
		args = append(args, "/cert:ignore")
	default:
		return Command{}, errors.New("invalid RDP certificate mode")
	}
	return Command{Path: binary, Args: []string{"/args-from:stdin"}, Stdin: strings.Join(args, "\n") + "\n"}, nil
}

type MoonlightConfig struct {
	Application        string `json:"application"`
	Width              int    `json:"width"`
	Height             int    `json:"height"`
	FPS                int    `json:"fps"`
	BitrateKbps        int    `json:"bitrate_kbps"`
	Codec              string `json:"codec"`
	HDR                bool   `json:"hdr"`
	Audio              bool   `json:"audio"`
	Gamepad            bool   `json:"gamepad"`
	PerformanceOverlay bool   `json:"performance_overlay"`
	SunshineAPIPort    int    `json:"sunshine_api_port"`
	PairingName        string `json:"pairing_name"`
}

type VNCConfig struct {
	Fullscreen bool `json:"fullscreen"`
	Shared     bool `json:"shared"`
	ViewOnly   bool `json:"view_only"`
	Clipboard  bool `json:"clipboard"`
}

type SSHConfig struct {
	TerminalTitle string `json:"terminal_title,omitempty"`
}

// SSHCommand starts one single-purpose xterm child. It never invokes a local
// shell. OpenSSH ignores user configuration, pins the host key, disables all
// forwarding and local-command features, and has no escape command prompt.
func SSHCommand(terminalBinary, sshBinary, sshpassBinary string, x api.Manifest) (Command, error) {
	if err := validateHost(x.Host, x.Port); err != nil {
		return Command{}, err
	}
	if x.Username == "" || len(x.Username) > 128 || strings.ContainsAny(x.Username, "\r\n\x00 \t") {
		return Command{}, errors.New("invalid SSH username")
	}
	var cfg SSHConfig
	if len(x.Config) == 0 || json.Unmarshal(x.Config, &cfg) != nil {
		return Command{}, errors.New("invalid SSH settings")
	}
	title := strings.TrimSpace(cfg.TerminalTitle)
	if title == "" {
		title = x.Name
	}
	if title == "" {
		title = "ThinPi SSH"
	}
	if len(title) > 128 || strings.ContainsAny(title, "\r\n\x00") {
		return Command{}, errors.New("invalid SSH terminal title")
	}
	files := []FileInput{}
	sshArgs := []string{"-F", "/dev/null", "-tt",
		"-o", "EscapeChar=none", "-o", "PermitLocalCommand=no", "-o", "ClearAllForwardings=yes",
		"-o", "ForwardAgent=no", "-o", "ForwardX11=no",
		"-o", "ForwardX11Trusted=no", "-o", "ControlMaster=no", "-o", "ProxyCommand=none",
		"-o", "ProxyJump=none", "-o", "IdentityAgent=none", "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=yes",
		"-o", "CheckHostIP=no", "-o", "UserKnownHostsFile=" + defaultSSHKnownHostsPath(),
		"-o", "GlobalKnownHostsFile=/dev/null", "-p", strconv.Itoa(x.Port), "-l", x.Username, x.Host}
	child := []string{sshBinary}
	if x.CredentialType == "ssh_private_key" {
		if x.Password == "" || strings.ContainsRune(x.Password, '\x00') {
			return Command{}, errors.New("SSH private key is unavailable")
		}
		files = append(files, FileInput{Placeholder: "{ssh_identity_file}", Content: strings.TrimSpace(x.Password) + "\n"})
		sshArgs = append([]string{"-o", "IdentityFile={ssh_identity_file}", "-o", "PreferredAuthentications=publickey", "-o", "PubkeyAuthentication=yes", "-o", "PasswordAuthentication=no", "-o", "KbdInteractiveAuthentication=no"}, sshArgs...)
	} else {
		sshArgs = append([]string{"-o", "IdentityFile=none"}, sshArgs...)
	}
	if x.Password != "" && x.CredentialType != "ssh_private_key" {
		if sshpassBinary == "" {
			return Command{}, errors.New("SSH client is unavailable")
		}
		child = []string{sshpassBinary, "-f", "{ssh_password_file}", sshBinary}
		files = append(files, FileInput{Placeholder: "{ssh_password_file}", Content: x.Password + "\n"})
		sshArgs = append([]string{"-o", "PreferredAuthentications=password,keyboard-interactive", "-o", "PubkeyAuthentication=no", "-o", "NumberOfPasswordPrompts=1"}, sshArgs...)
	}
	args := []string{"-title", title,
		"-xrm", "XTerm*fullscreen: always", "-xrm", "XTerm*allowWindowOps: false",
		"-xrm", "XTerm*allowTitleOps: false", "-xrm", "XTerm*allowFontOps: false",
		"-xrm", "XTerm*allowMouseOps: false", "-xrm", "XTerm*logInhibit: true",
		"-xrm", "XTerm*scrollBar: false", "-xrm", "XTerm*toolBar: false", "-e"}
	args = append(args, child...)
	args = append(args, sshArgs...)
	return Command{Path: terminalBinary, Args: args, Files: files, Env: []string{"SHELL=/usr/sbin/nologin"}}, nil
}

func VNCCommand(binary string, x api.Manifest) (Command, error) {
	if err := validateHost(x.Host, x.Port); err != nil {
		return Command{}, err
	}
	if x.CredentialType == "ssh_private_key" {
		return Command{}, errors.New("an SSH private key cannot be used for VNC")
	}
	if strings.ContainsAny(x.Username, "\r\n\x00") {
		return Command{}, errors.New("invalid VNC username")
	}
	if strings.ContainsRune(x.Password, '\x00') {
		return Command{}, errors.New("invalid VNC password")
	}
	cfg := VNCConfig{Fullscreen: true, Shared: true}
	if len(x.Config) > 0 {
		if err := json.Unmarshal(x.Config, &cfg); err != nil {
			return Command{}, err
		}
	}
	host := x.Host
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	args := []string{host + "::" + strconv.Itoa(x.Port), "-SecurityTypes=TLSPlain", fmt.Sprintf("-FullScreen=%d", boolInt(cfg.Fullscreen)), fmt.Sprintf("-Shared=%d", boolInt(cfg.Shared)), fmt.Sprintf("-ViewOnly=%d", boolInt(cfg.ViewOnly)), fmt.Sprintf("-AcceptClipboard=%d", boolInt(cfg.Clipboard)), fmt.Sprintf("-SendClipboard=%d", boolInt(cfg.Clipboard))}
	var environment []string
	if x.Username != "" {
		environment = append(environment, "VNC_USERNAME="+x.Username)
	}
	if x.Password != "" {
		environment = append(environment, "VNC_PASSWORD="+x.Password)
	}
	return Command{Path: binary, Args: args, Env: environment}, nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func MoonlightCommand(binary string, x api.Manifest) (Command, error) {
	if err := validateHost(x.Host, x.Port); err != nil {
		return Command{}, err
	}
	cfg := MoonlightConfig{Application: "Desktop", Width: 1920, Height: 1080, FPS: 60, BitrateKbps: 20000, Audio: true, Gamepad: true, SunshineAPIPort: 47990, PairingName: "ThinPi"}
	if len(x.Config) > 0 {
		if err := json.Unmarshal(x.Config, &cfg); err != nil {
			return Command{}, err
		}
	}
	if cfg.Application == "" || len(cfg.Application) > 128 || strings.ContainsAny(cfg.Application, "\r\n\x00") {
		return Command{}, errors.New("invalid Moonlight application")
	}
	if cfg.SunshineAPIPort < 1 || cfg.SunshineAPIPort > 65535 {
		return Command{}, errors.New("invalid Sunshine API port")
	}
	if cfg.PairingName == "" || len(cfg.PairingName) > 128 || strings.ContainsAny(cfg.PairingName, "\r\n\x00") {
		return Command{}, errors.New("invalid Moonlight pairing name")
	}
	if strings.ContainsAny(x.Username, ":\r\n\x00") || strings.ContainsRune(x.Password, '\x00') {
		return Command{}, errors.New("invalid Sunshine admin credential")
	}
	if cfg.Width < 256 || cfg.Width > 8192 || cfg.Height < 256 || cfg.Height > 8192 || cfg.FPS < 1 || cfg.FPS > 240 || cfg.BitrateKbps < 500 || cfg.BitrateKbps > 500000 {
		return Command{}, errors.New("invalid Moonlight stream settings")
	}
	if !cfg.Audio || !cfg.Gamepad {
		return Command{}, errors.New("installed Moonlight CLI cannot enforce disabled client audio or gamepad input")
	}
	codec := strings.ToLower(cfg.Codec)
	codecValues := map[string]string{"": "auto", "auto": "auto", "h264": "H.264", "h.264": "H.264", "hevc": "HEVC", "av1": "AV1"}
	codecValue, ok := codecValues[codec]
	if !ok {
		return Command{}, errors.New("invalid Moonlight codec")
	}
	moonlightHost := x.Host
	if strings.Contains(x.Host, ":") {
		moonlightHost = "[" + x.Host + "]"
	}
	if x.Port != 47984 {
		moonlightHost = net.JoinHostPort(x.Host, strconv.Itoa(x.Port))
	}
	args := []string{"stream", moonlightHost, cfg.Application,
		"--resolution", fmt.Sprintf("%dx%d", cfg.Width, cfg.Height),
		"--fps", strconv.Itoa(cfg.FPS), "--bitrate", strconv.Itoa(cfg.BitrateKbps),
		"--display-mode", "fullscreen", "--video-decoder", "hardware", "--video-codec", codecValue,
		"--frame-pacing", "--keep-awake"}
	if cfg.HDR {
		args = append(args, "--hdr")
	} else {
		args = append(args, "--no-hdr")
	}
	args = append(args, "--background-gamepad")
	if cfg.PerformanceOverlay {
		args = append(args, "--performance-overlay")
	} else {
		args = append(args, "--no-performance-overlay")
	}
	args = append(args, "--no-audio-on-host")
	return Command{
		Path: binary,
		Args: args,
		MoonlightPairing: &MoonlightPairing{
			Host: x.Host, Username: x.Username, Password: x.Password,
			SunshineAPIPort: cfg.SunshineAPIPort, ClientName: cfg.PairingName,
		},
	}, nil
}
func MockCommand(d time.Duration) Command {
	if d <= 0 {
		d = 3 * time.Second
	}
	return Command{Path: "thinpi-mock", Args: []string{d.String()}}
}

func runMock(ctx context.Context, c Command) error {
	if len(c.Args) != 1 {
		return errors.New("mock duration is required")
	}
	d, err := time.ParseDuration(c.Args[0])
	if err != nil || d <= 0 {
		return errors.New("invalid mock duration")
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func Detect(configured string, candidates []string) ClientInfo {
	if configured != "" && configured != "auto" {
		if p, err := exec.LookPath(configured); err == nil {
			return probe(p)
		}
	}
	for _, name := range candidates {
		if p, err := exec.LookPath(name); err == nil {
			return probe(p)
		}
	}
	return ClientInfo{}
}
func probe(path string) ClientInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
	if err != nil || len(out) == 0 {
		out, _ = exec.CommandContext(ctx, path, "/version").CombinedOutput()
	}
	line, _ := bufio.NewReader(strings.NewReader(string(out))).ReadString('\n')
	return ClientInfo{Available: true, Binary: path, Version: strings.TrimSpace(line)}
}
