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
	Open(string) error
}

type Server struct {
	Manager          *launch.Manager
	DeviceIdentifier string
	Log              *slog.Logger
	Maintenance      Maintenance
}
type request struct {
	Action    string `json:"action"`
	Ticket    string `json:"ticket,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Accept    bool   `json:"accept,omitempty"`
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
			write(c, map[string]any{"accepted": false, "error": "The remote session could not be started."})
			return
		}
		write(c, map[string]any{"accepted": true, "session_id": id})
	case "status":
		write(c, map[string]any{"status": s.Manager.Status(), "device_identifier": s.DeviceIdentifier})
	case "cancel":
		if err := s.Manager.Cancel(q.SessionID); err != nil {
			write(c, map[string]any{"accepted": false, "error": "No remote session is active."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "minimize":
		if err := s.Manager.Minimize(q.SessionID); err != nil {
			write(c, map[string]any{"accepted": false, "error": "No visible remote session is active."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "resume":
		if err := s.Manager.Resume(q.SessionID); err != nil {
			write(c, map[string]any{"accepted": false, "error": "The minimized remote session is no longer available."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "resolve_ssh_host_key":
		if err := s.Manager.ResolveSSHHostKeyChange(q.SessionID, q.Accept); err != nil {
			write(c, map[string]any{"accepted": false, "error": "No SSH host-key change is awaiting confirmation."})
			return
		}
		write(c, map[string]any{"accepted": true})
	case "maintenance":
		if q.Ticket == "" || s.Maintenance == nil {
			write(c, map[string]any{"accepted": false, "error": "Local maintenance is unavailable."})
			return
		}
		if s.Manager.HasSessions() {
			write(c, map[string]any{"accepted": false, "error": "Close all remote connections before opening local maintenance."})
			return
		}
		if err := s.Maintenance.Open(q.Ticket); err != nil {
			write(c, map[string]any{"accepted": false, "error": "Maintenance authorisation was rejected."})
			return
		}
		write(c, map[string]any{"accepted": true})
	default:
		write(c, map[string]any{"error": "unsupported action"})
	}
}
func write(c net.Conn, v any) { _ = json.NewEncoder(c).Encode(v) }
