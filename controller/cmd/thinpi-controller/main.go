package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"thinpi.local/controller/internal/app"
	"thinpi.local/controller/internal/database"
	"thinpi.local/controller/internal/security"
	controllerweb "thinpi.local/controller/internal/web"
)

var version = "dev"

type options struct {
	database, listen, masterKeyFile, tlsCert, tlsKey string
	dev                                              bool
	idle, ticketTTL                                  time.Duration
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("controller stopped", "error", err)
		os.Exit(1)
	}
}

func flags(name string) (*flag.FlagSet, *options) {
	o := &options{}
	f := flag.NewFlagSet(name, flag.ContinueOnError)
	f.StringVar(&o.database, "database", env("THINPI_DATABASE", "/var/lib/thinpi/thinpi.db"), "SQLite database path")
	f.StringVar(&o.listen, "listen", env("THINPI_LISTEN", "0.0.0.0:8443"), "HTTP listen address")
	f.StringVar(&o.masterKeyFile, "master-key-file", env("THINPI_MASTER_KEY_FILE", "/run/secrets/thinpi_master_key"), "32-byte/base64 AES master key file")
	f.StringVar(&o.tlsCert, "tls-cert", env("THINPI_TLS_CERT", ""), "TLS certificate file")
	f.StringVar(&o.tlsKey, "tls-key", env("THINPI_TLS_KEY", ""), "TLS private key file")
	f.BoolVar(&o.dev, "dev", env("THINPI_DEV_MODE", "") == "1", "allow HTTP and use a deterministic development key")
	f.DurationVar(&o.idle, "session-idle-timeout", 30*time.Minute, "session idle timeout")
	f.DurationVar(&o.ticketTTL, "launch-ticket-ttl", 30*time.Second, "launch ticket lifetime")
	return f, o
}

func run(args []string) error {
	command := "serve"
	if len(args) > 0 && (args[0] == "serve" || args[0] == "seed-dev" || args[0] == "reset-dev" || args[0] == "create-enrolment-token" || args[0] == "bootstrap-admin" || args[0] == "generate-master-key" || args[0] == "healthcheck" || args[0] == "version") {
		command = args[0]
		args = args[1:]
	}
	if command == "version" {
		fmt.Println(version)
		return nil
	}
	if command == "generate-master-key" {
		return generateKey(args)
	}
	if command == "healthcheck" {
		return healthcheck(args)
	}
	f, o := flags(command)
	name := f.String("name", "ThinPi", "device name for an enrolment token")
	ttl := f.Duration("ttl", 15*time.Minute, "enrolment token validity")
	username := f.String("username", "admin", "bootstrap administrator username")
	displayName := f.String("display-name", "Administrator", "bootstrap administrator display name")
	passwordStdin := f.Bool("password-stdin", false, "read bootstrap password from stdin")
	if err := f.Parse(args); err != nil {
		return err
	}
	if command == "reset-dev" {
		if !o.dev {
			return errors.New("reset-dev requires --dev")
		}
		abs, err := filepath.Abs(o.database)
		if err != nil {
			return err
		}
		if filepath.Base(abs) == "" || filepath.Ext(abs) != ".db" {
			return errors.New("refusing to reset a path that is not a .db file")
		}
		if err = os.Remove(abs); err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			_ = os.Remove(abs + suffix)
		}
	}
	db, err := database.Open(o.database)
	if err != nil {
		return err
	}
	defer db.Close()
	key, err := loadKey(o)
	if err != nil {
		return err
	}
	vault, err := security.NewVault(key)
	if err != nil {
		return err
	}
	store := app.NewStore(db, vault, o.idle, o.ticketTTL)
	switch command {
	case "seed-dev", "reset-dev":
		if !o.dev {
			return errors.New(command + " requires --dev")
		}
		if err = store.SeedDev(context.Background()); err == nil {
			fmt.Println("Development data ready. Users: admin, wife, daughter; password: thinpi-dev")
		}
		return err
	case "create-enrolment-token":
		token, err := store.CreateEnrolmentToken(context.Background(), *name, *ttl)
		if err == nil {
			fmt.Println(token)
		}
		return err
	case "bootstrap-admin":
		if !*passwordStdin {
			return errors.New("bootstrap-admin requires --password-stdin")
		}
		var admins int
		if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1`).Scan(&admins); err != nil {
			return err
		}
		if admins != 0 {
			return errors.New("an administrator already exists; bootstrap refused")
		}
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 1024), 1024)
		if !scanner.Scan() {
			return errors.New("unable to read password from stdin")
		}
		password := scanner.Text()
		if _, err := store.CreateUser(context.Background(), *username, *displayName, password, true, true); err != nil {
			return err
		}
		fmt.Printf("Administrator created.\nUsername: %s\nDisplay name: %s\n", *username, *displayName)
		return nil
	case "serve":
		return serve(o, store)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func healthcheck(args []string) error {
	f := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	target := f.String("url", "https://127.0.0.1:8443/healthz", "loopback health endpoint")
	if err := f.Parse(args); err != nil {
		return err
	}
	u, err := url.Parse(*target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "/healthz" {
		return errors.New("healthcheck URL must be an HTTP(S) /healthz endpoint")
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("healthcheck URL must use a loopback host")
	}
	transport := &http.Transport{}
	if u.Scheme == "https" {
		// The probe stays inside the container. The public certificate normally
		// names the external hostname rather than 127.0.0.1.
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport}
	response, err := client.Get(u.String())
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", response.Status)
	}
	return nil
}

func serve(o *options, store *app.Store) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if o.dev {
		if err := store.SeedDev(context.Background()); err != nil {
			return err
		}
		logger.Warn("development mode enabled; HTTP and deterministic key are not secure")
	}
	srvImpl := controllerweb.New(store, logger, o.dev, version)
	srv := &http.Server{Addr: o.listen, Handler: srvImpl.Handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		logger.Info("controller listening", "address", o.listen, "version", version)
		if o.tlsCert != "" && o.tlsKey != "" {
			errCh <- srv.ListenAndServeTLS(o.tlsCert, o.tlsKey)
		} else if o.dev {
			errCh <- srv.ListenAndServe()
		} else {
			errCh <- errors.New("production mode requires --tls-cert and --tls-key")
		}
	}()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func loadKey(o *options) ([]byte, error) {
	if o.dev {
		return security.DeterministicDevKey(), nil
	}
	raw, err := os.ReadFile(o.masterKeyFile)
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	return security.ParseMasterKey(raw)
}
func generateKey(args []string) error {
	f := flag.NewFlagSet("generate-master-key", flag.ContinueOnError)
	out := f.String("out", "", "output file (required)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("--out is required")
	}
	if _, err := os.Stat(*out); err == nil {
		return errors.New("refusing to overwrite existing key")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	return os.WriteFile(*out, []byte(base64.StdEncoding.EncodeToString(b)+"\n"), 0640)
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
