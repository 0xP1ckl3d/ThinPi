package localapi

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"thinpi.local/agent/internal/api"
	"thinpi.local/agent/internal/launch"
)

type controller struct{}

func (controller) Redeem(context.Context, string) (api.Manifest, error) {
	return api.Manifest{Protocol: "mock", ConnectionID: 1, Host: "mock", Port: 1}, nil
}
func (controller) SessionEvent(context.Context, int64, int64, string, string, any) error { return nil }

type runner struct{}

func (runner) Run(context.Context, launch.Command) error { return nil }

type heldRunner struct{ started chan struct{} }

func (r heldRunner) Run(ctx context.Context, _ launch.Command) error {
	close(r.started)
	<-ctx.Done()
	return ctx.Err()
}

type maintenanceRecorder struct{ ticket string }

func (m *maintenanceRecorder) Open(ticket string) error {
	m.ticket = ticket
	return nil
}
func TestStatusDoesNotExposeCredential(t *testing.T) {
	m := launch.NewManager(controller{}, runner{}, true, time.Millisecond, launch.Clients{})
	s := &Server{Manager: m, DeviceIdentifier: "pi-1"}
	a, b := net.Pipe()
	go s.handle(a)
	_ = json.NewEncoder(b).Encode(map[string]string{"action": "status"})
	var out map[string]any
	if err := json.NewDecoder(b).Decode(&out); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out["device_identifier"] != "pi-1" {
		t.Fatalf("bad status: %#v", out)
	}
	if _, ok := out["device_token"]; ok {
		t.Fatal("device credential exposed")
	}
}

func TestArbitraryActionRejected(t *testing.T) {
	m := launch.NewManager(controller{}, runner{}, true, time.Millisecond, launch.Clients{})
	s := &Server{Manager: m, DeviceIdentifier: "pi-1"}
	a, b := net.Pipe()
	go s.handle(a)
	_ = json.NewEncoder(b).Encode(map[string]string{"action": "exec", "command": "sh"})
	var out map[string]any
	if err := json.NewDecoder(b).Decode(&out); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out["error"] != "unsupported action" {
		t.Fatalf("action accepted: %#v", out)
	}
}

func TestMaintenanceActionRequiresTicketAndFixedBroker(t *testing.T) {
	m := launch.NewManager(controller{}, runner{}, true, time.Millisecond, launch.Clients{})
	recorder := &maintenanceRecorder{}
	s := &Server{Manager: m, DeviceIdentifier: "pi-1", Maintenance: recorder}
	a, b := net.Pipe()
	go s.handle(a)
	_ = json.NewEncoder(b).Encode(map[string]string{"action": "maintenance", "ticket": "one-use-ticket"})
	var out map[string]any
	if err := json.NewDecoder(b).Decode(&out); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out["accepted"] != true || recorder.ticket != "one-use-ticket" {
		t.Fatalf("maintenance not brokered: %#v ticket=%q", out, recorder.ticket)
	}
}

func TestMaintenanceRefusesOpenSessionsUntilTheyAreClosed(t *testing.T) {
	runner := heldRunner{started: make(chan struct{})}
	m := launch.NewManager(controller{}, runner, true, time.Second, launch.Clients{})
	if _, err := m.Launch("ticket"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("remote session did not start")
	}

	recorder := &maintenanceRecorder{}
	s := &Server{Manager: m, DeviceIdentifier: "pi-1", Maintenance: recorder}
	a, b := net.Pipe()
	go s.handle(a)
	_ = json.NewEncoder(b).Encode(map[string]string{"action": "maintenance", "ticket": "one-use-ticket"})
	var out map[string]any
	if err := json.NewDecoder(b).Decode(&out); err != nil {
		t.Fatal(err)
	}
	b.Close()
	if out["accepted"] != false || recorder.ticket != "" {
		t.Fatalf("maintenance bypassed open sessions: %#v ticket=%q", out, recorder.ticket)
	}
	if err := m.Cancel(""); err != nil {
		t.Fatal(err)
	}
}
