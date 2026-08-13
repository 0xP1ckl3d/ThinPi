package localapi

import (
	"bufio"
	"encoding/json"
	"log/slog"
	"net"
	"time"

	"thinpi.local/agent/internal/launch"
)

type Maintenance interface {
	Open(string, string) error
}

type Server struct {
	Manager          *launch.Manager
	DeviceIdentifier string
	Log              *slog.Logger
	Maintenance      Maintenance
}
type request struct {
	Action        string `json:"action"`
	Ticket        string `json:"ticket,omitempty"`
	Accept        bool   `json:"accept,omitempty"`
	TerminalTheme string `json:"terminal_theme,omitempty"`
}

func (s *Server) Serve(l net.Listener) error {
	for {
		c, err := l.Accept()
		if err != nil {
			return err
		}
		go s.handle(c)
	}
}
func (s *Server) handle(c net.Conn) {
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	var q request
	if err := json.NewDecoder(bufio.NewReader(c)).Decode(&q); err != nil {
		write(c, map[string]any{"error": "invalid request"})
		return
	}
	switch q.Action {
	case "launch":
		if q.Ticket == "" {
			write(c, map[string]any{"error": "ticket is required"})
			return
		}
		id, err := s.Manager.Launch(q.Ticket)
		if err != nil {
			write(c, map[string]any{"accepted": false, "error": "A remote session is already active."})
			return
		}
		write(c, map[string]any{"accepted": true, "session_id": id})
	case "status":
		write(c, map[string]any{"status": s.Manager.Status(), "device_identifier": s.DeviceIdentifier})
	case "cancel":
		if err := s.Manager.Cancel(); err != nil {
			write(c, map[string]any{"accepted": false, "error": "No remote session is active."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "resolve_ssh_host_key":
		if err := s.Manager.ResolveSSHHostKeyChange(q.Accept); err != nil {
			write(c, map[string]any{"accepted": false, "error": "No SSH host-key change is awaiting confirmation."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "maintenance":
		if q.Ticket == "" || s.Maintenance == nil || s.Manager.Status().State != launch.Idle {
			write(c, map[string]any{"accepted": false, "error": "Local maintenance is unavailable."})
			return
		}
		if err := s.Maintenance.Open(q.Ticket, q.TerminalTheme); err != nil {
			write(c, map[string]any{"accepted": false, "error": "Maintenance authorisation was rejected."})
			return
		}
		write(c, map[string]any{"accepted": true})
	default:
		write(c, map[string]any{"error": "unsupported action"})
	}
}
func write(c net.Conn, v any) { _ = json.NewEncoder(c).Encode(v) }
