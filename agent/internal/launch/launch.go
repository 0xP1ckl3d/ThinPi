package launch

import (
	"bufio"
	"context"
	"encoding/base64"
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
	State               State      `json:"state"`
	ActiveSession       *string    `json:"active_session"`
	ControllerReachable bool       `json:"controller_reachable"`
	FreeRDP             ClientInfo `json:"freerdp"`
	Moonlight           ClientInfo `json:"moonlight"`
	VNC                 ClientInfo `json:"vnc"`
	SSH                 ClientInfo `json:"ssh"`
	LastError           string     `json:"last_error,omitempty"`
}
type Controller interface {
	Redeem(context.Context, string) (api.Manifest, error)
	SessionEvent(context.Context, int64, int64, string, string, any) error
}
type Command struct {
	Path     string
	Args     []string
	Stdin    string
	Password string
	Env      []string
	Files    []FileInput
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
}

func NewManager(c Controller, r Runner, mock bool, d time.Duration, clients Clients) *Manager {
	return &Manager{controller: c, runner: r, mock: mock, mockDuration: d, clients: clients, status: Status{State: Idle, ControllerReachable: true, FreeRDP: clients.FreeRDP, Moonlight: clients.Moonlight, VNC: clients.VNC, SSH: clients.SSH}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
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
		cmd, err = m.commandFor(manifest)
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
		m.set(Failed, nil, errors.New("The remote client exited before a session could be established. Check that the target is reachable and available."))
		time.AfterFunc(2*time.Second, func() { m.set(Idle, nil, nil) })
		return
	}
	m.set(Stopping, &id, nil)
	m.set(Idle, nil, nil)
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
}

func FreeRDPCommand(binary string, x api.Manifest) (Command, error) {
	if err := validateHost(x.Host, x.Port); err != nil {
		return Command{}, err
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
}

type VNCConfig struct {
	Fullscreen bool `json:"fullscreen"`
	Shared     bool `json:"shared"`
	ViewOnly   bool `json:"view_only"`
	Clipboard  bool `json:"clipboard"`
}

type SSHConfig struct {
	HostKey       string `json:"host_key"`
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
	hostKey := strings.TrimSpace(cfg.HostKey)
	parts := strings.Fields(hostKey)
	decodedKey, decodeErr := func() ([]byte, error) {
		if len(parts) != 2 {
			return nil, errors.New("invalid fields")
		}
		return base64.StdEncoding.DecodeString(parts[1])
	}()
	if len(parts) != 2 || !strings.HasPrefix(parts[0], "ssh-") || decodeErr != nil || len(decodedKey) < 32 || strings.ContainsAny(hostKey, "\r\n\x00") {
		return Command{}, errors.New("a pinned SSH host key is required")
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
	knownHost := x.Host
	if x.Port != 22 {
		knownHost = "[" + x.Host + "]:" + strconv.Itoa(x.Port)
	}
	files := []FileInput{{Placeholder: "{known_hosts_file}", Content: knownHost + " " + hostKey + "\n"}}
	sshArgs := []string{"-F", "/dev/null", "-tt",
		"-o", "EscapeChar=none", "-o", "PermitLocalCommand=no", "-o", "ClearAllForwardings=yes",
		"-o", "DisableForwarding=yes", "-o", "ForwardAgent=no", "-o", "ForwardX11=no",
		"-o", "ForwardX11Trusted=no", "-o", "ControlMaster=no", "-o", "ProxyCommand=none",
		"-o", "ProxyJump=none", "-o", "IdentityAgent=none", "-o", "IdentityFile=none",
		"-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=yes",
		"-o", "CheckHostIP=yes", "-o", "UserKnownHostsFile={known_hosts_file}",
		"-o", "GlobalKnownHostsFile=/dev/null", "-p", strconv.Itoa(x.Port), "-l", x.Username, x.Host}
	child := []string{sshBinary}
	if x.Password != "" {
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
	if strings.ContainsAny(x.Username, "\r\n\x00") {
		return Command{}, errors.New("invalid VNC username")
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
	args := []string{host + "::" + strconv.Itoa(x.Port), fmt.Sprintf("-FullScreen=%d", boolInt(cfg.Fullscreen)), fmt.Sprintf("-Shared=%d", boolInt(cfg.Shared)), fmt.Sprintf("-ViewOnly=%d", boolInt(cfg.ViewOnly)), fmt.Sprintf("-AcceptClipboard=%d", boolInt(cfg.Clipboard)), fmt.Sprintf("-SendClipboard=%d", boolInt(cfg.Clipboard))}
	var environment []string
	if x.Username != "" {
		environment = append(environment, "VNC_USERNAME="+x.Username)
	}
	if x.Password != "" {
		args = append(args, "-PasswordFile={password_file}")
	}
	return Command{Path: binary, Args: args, Password: x.Password, Env: environment}, nil
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
	cfg := MoonlightConfig{Application: "Desktop", Width: 1920, Height: 1080, FPS: 60, BitrateKbps: 20000, Audio: true, Gamepad: true}
	if len(x.Config) > 0 {
		if err := json.Unmarshal(x.Config, &cfg); err != nil {
			return Command{}, err
		}
	}
	if cfg.Application == "" || len(cfg.Application) > 128 || strings.ContainsAny(cfg.Application, "\r\n\x00") {
		return Command{}, errors.New("invalid Moonlight application")
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
	return Command{Path: binary, Args: args}, nil
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
