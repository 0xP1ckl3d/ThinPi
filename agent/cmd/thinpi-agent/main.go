package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"thinpi.local/agent/internal/api"
	"thinpi.local/agent/internal/config"
	"thinpi.local/agent/internal/launch"
	"thinpi.local/agent/internal/localapi"
	"thinpi.local/agent/internal/maintenance"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("agent stopped", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	cmd := "serve"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "enrol" || args[0] == "status" || args[0] == "version") {
		cmd = args[0]
		args = args[1:]
	}
	if cmd == "version" {
		fmt.Println(version)
		return nil
	}
	f := flag.NewFlagSet(cmd, flag.ContinueOnError)
	configPath := f.String("config", env("THINPI_AGENT_CONFIG", "/etc/thinpi/agent.json"), "agent configuration file")
	server := f.String("server", "", "controller URL (enrol)")
	token := f.String("token", "", "one-time enrolment token")
	tokenStdin := f.Bool("token-stdin", false, "read the one-time enrolment token from stdin")
	identifier := f.String("device-id", "", "stable device identifier")
	name := f.String("name", "ThinPi", "device display name")
	ca := f.String("ca-certificate", "", "private CA certificate")
	deviceFile := f.String("device-file", "/etc/thinpi/device.json", "device credential destination")
	if err := f.Parse(args); err != nil {
		return err
	}
	if cmd == "enrol" {
		if *tokenStdin {
			if *token != "" {
				return errors.New("use only one of --token or --token-stdin")
			}
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 1024), 4096)
			if !scanner.Scan() {
				return errors.New("unable to read enrolment token from stdin")
			}
			*token = strings.TrimSpace(scanner.Text())
		}
		if *server == "" || *token == "" || *identifier == "" {
			return errors.New("--server, an enrolment token, and --device-id are required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d, err := api.Enrol(ctx, *server, *ca, *token, *identifier, *name)
		if err != nil {
			return err
		}
		if err = config.SaveDevice(*deviceFile, d); err != nil {
			return err
		}
		fmt.Printf("Device %s enrolled; credential written to %s\n", d.DeviceIdentifier, *deviceFile)
		return nil
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	if os.Getenv("THINPI_AGENT_MOCK_CLIENTS") == "1" {
		cfg.MockClients = true
	}
	if cfg.MockClients && !localDevelopmentController(cfg.ControllerURL) {
		return errors.New("mock clients are restricted to a loopback HTTP development controller")
	}
	d, err := config.LoadDevice(cfg.DeviceFile)
	if err != nil && cfg.MockClients && localDevelopmentController(cfg.ControllerURL) {
		d = config.DeviceFile{DeviceIdentifier: "dev-device", DeviceToken: "dev-device-token", Name: "Development Pi"}
	} else if err != nil {
		return fmt.Errorf("load device credential: %w", err)
	}
	client, err := api.New(cfg.ControllerURL, d.DeviceToken, cfg.CACertificate)
	if err != nil {
		return err
	}
	rdp := launch.Detect(cfg.FreeRDPBinary, []string{"xfreerdp3", "xfreerdp", "sdl-freerdp3", "sdl-freerdp"})
	moon := launch.Detect(cfg.MoonlightBinary, []string{"moonlight-qt", "moonlight"})
	vnc := launch.Detect(cfg.VNCBinary, []string{"xtigervncviewer", "tigervncviewer", "vncviewer"})
	ssh := launch.Detect(cfg.SSHBinary, []string{"ssh"})
	terminal := launch.Detect(cfg.TerminalBinary, []string{"xterm"})
	sshpass := launch.Detect(cfg.SSHPassBinary, []string{"sshpass"})
	if cfg.MockClients {
		rdp.Available = true
		moon.Available = true
		vnc.Available = true
		ssh.Available = true
		terminal.Available = true
		sshpass.Available = true
	}
	clients := launch.Clients{FreeRDP: rdp, Moonlight: moon, VNC: vnc, SSH: ssh, Terminal: terminal, SSHPass: sshpass}
	manager := launch.NewManager(client, launch.PlatformRunner{}, cfg.MockClients, time.Duration(cfg.MockDurationSeconds)*time.Second, clients)
	if cmd == "status" {
		fmt.Printf("device=%s freerdp=%v moonlight=%v vnc=%v ssh=%v terminal=%v\n", d.DeviceIdentifier, rdp.Available, moon.Available, vnc.Available, ssh.Available, terminal.Available)
		return nil
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	maintenanceBroker := maintenance.New(client, cfg.MaintenanceUser, logger)
	manager.SetLogger(logger)
	logger.Info("native clients detected", "freerdp", rdp, "moonlight", moon, "vnc", vnc, "ssh", ssh, "terminal", terminal, "sshpass", sshpass, "mock", cfg.MockClients, "version", version)
	listener, err := localapi.ListenLocal(cfg.Socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go heartbeat(ctx, client, version, rdp, moon, vnc, ssh, logger)
	errCh := make(chan error, 1)
	go func() {
		errCh <- (&localapi.Server{Manager: manager, DeviceIdentifier: d.DeviceIdentifier, Log: logger, Maintenance: maintenanceBroker}).Serve(listener)
	}()
	select {
	case err = <-errCh:
		return err
	case <-ctx.Done():
		return nil
	}
}
func heartbeat(ctx context.Context, c *api.Client, version string, rdp, moon, vnc, ssh launch.ClientInfo, log *slog.Logger) {
	send := func() {
		hctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		if err := c.Heartbeat(hctx, map[string]any{"agent": version, "freerdp": rdp, "moonlight": moon, "vnc": vnc, "ssh": ssh}); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn("controller heartbeat failed", "error", safeError(err))
		}
	}
	send()
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}
func safeError(err error) string {
	s := err.Error()
	for _, marker := range []string{"Bearer ", "ticket=", "password="} {
		if i := strings.Index(s, marker); i >= 0 {
			s = s[:i] + marker + "[redacted]"
		}
	}
	return s
}
func localDevelopmentController(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" || u.Hostname() == "" {
		return false
	}
	return strings.EqualFold(u.Hostname(), "localhost") || net.ParseIP(u.Hostname()).IsLoopback()
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
