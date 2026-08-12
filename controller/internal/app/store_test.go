package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"thinpi.local/controller/internal/database"
	"thinpi.local/controller/internal/security"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	v, _ := security.NewVault(security.DeterministicDevKey())
	s := NewStore(db, v, 30*time.Minute, 30*time.Second)
	return s
}

func TestACLAndDisabledConnection(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, daughter, err := s.Authenticate(ctx, "daughter", "thinpi-dev", "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.ConnectionsForUser(ctx, daughter.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("daughter got %d connections, want 3", len(got))
	}
	if _, err = s.DB.Exec(`UPDATE connections SET enabled=0 WHERE name='Mock School PC'`); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ConnectionsForUser(ctx, daughter.ID)
	if len(got) != 2 {
		t.Fatalf("disabled connection remained visible: %d", len(got))
	}
}

func TestDisabledUserCannotLogin(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	id, err := s.CreateUser(ctx, "off", "Off User", "password1", false, false)
	if err != nil || id == 0 {
		t.Fatal(err)
	}
	if _, _, _, err = s.Authenticate(ctx, "off", "password1", "test"); !errors.Is(err, ErrUnauthorised) {
		t.Fatalf("got %v", err)
	}
}

func TestSessionExpiryAndLoginRateLimit(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if _, err := s.CreateUser(ctx, "rate", "Rate Test", "password1", false, true); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	s.Now = func() time.Time { return base }
	token, _, _, err := s.Authenticate(ctx, "rate", "password1", "source")
	if err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return base.Add(31 * time.Minute) }
	if _, err = s.Session(ctx, token); !errors.Is(err, ErrUnauthorised) {
		t.Fatalf("expired session: %v", err)
	}
	s.Now = func() time.Time { return base }
	for i := 0; i < 8; i++ {
		_, _, _, _ = s.Authenticate(ctx, "rate", "wrong-password", "limited")
	}
	if _, _, _, err = s.Authenticate(ctx, "rate", "wrong-password", "limited"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}
}

func TestDeviceRevocation(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	token, _ := s.CreateEnrolmentToken(ctx, "Pi", time.Minute)
	credential, d, err := s.EnrolDevice(ctx, token, "revoked-pi", "Pi", "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE devices SET enabled=0 WHERE id=?`, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeviceByToken(ctx, credential); !errors.Is(err, ErrUnauthorised) {
		t.Fatalf("revoked device accepted: %v", err)
	}
}

func TestTicketSingleUseExpiryAndWrongDevice(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	_, _, u, _ := s.Authenticate(ctx, "daughter", "thinpi-dev", "test")
	enrol, _ := s.CreateEnrolmentToken(ctx, "Pi", time.Minute)
	cred, d, err := s.EnrolDevice(ctx, enrol, "pi-1", "Pi", "test")
	if err != nil || cred == "" {
		t.Fatal(err)
	}
	connections, _ := s.ConnectionsForUser(ctx, u.ID)
	ticket, err := s.CreateLaunchTicket(ctx, u.ID, connections[0].ID, d.Identifier, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RedeemLaunchTicket(ctx, ticket, d.ID+1, "test"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("wrong device: %v", err)
	}
	if _, err = s.RedeemLaunchTicket(ctx, ticket, d.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RedeemLaunchTicket(ctx, ticket, d.ID, "test"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("reused: %v", err)
	}
	s.Now = func() time.Time { return time.Now().UTC() }
	ticket, _ = s.CreateLaunchTicket(ctx, u.ID, connections[0].ID, d.Identifier, "test")
	s.Now = func() time.Time { return time.Now().UTC().Add(time.Minute) }
	if _, err = s.RedeemLaunchTicket(ctx, ticket, d.ID, "test"); !errors.Is(err, ErrTicketInvalid) {
		t.Fatalf("expired: %v", err)
	}
}

func TestCredentialOnlyReturnedOnRedeem(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	cid, err := s.CreateCredential(ctx, "school", "alice", "top-secret", "password")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Exec(`UPDATE connections SET credential_id=? WHERE name='Mock School PC'`, cid)
	if err != nil {
		t.Fatal(err)
	}
	_, _, u, _ := s.Authenticate(ctx, "daughter", "thinpi-dev", "test")
	_, err = s.DB.Exec(`UPDATE user_connection_permissions SET credential_id=? WHERE user_id=? AND connection_id=(SELECT id FROM connections WHERE name='Mock School PC')`, cid, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	enrol, _ := s.CreateEnrolmentToken(ctx, "Pi", time.Minute)
	_, d, _ := s.EnrolDevice(ctx, enrol, "pi-secret", "Pi", "test")
	connections, _ := s.ConnectionsForUser(ctx, u.ID)
	ticket, _ := s.CreateLaunchTicket(ctx, u.ID, connections[0].ID, d.Identifier, "test")
	m, err := s.RedeemLaunchTicket(ctx, ticket, d.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if m.Username != "alice" || m.Password != "top-secret" {
		t.Fatalf("bad manifest: %#v", m)
	}
}

func TestPolicyBlocksLaunchAndCapsSession(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)
	if err := s.SeedDev(ctx); err != nil {
		t.Fatal(err)
	}
	s.Now = func() time.Time { return time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC) }
	_, _, u, err := s.Authenticate(ctx, "daughter", "thinpi-dev", "test")
	if err != nil {
		t.Fatal(err)
	}
	connections, err := s.ConnectionsForUser(ctx, u.ID)
	if err != nil || len(connections) == 0 {
		t.Fatalf("connections: %v", err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE user_policies SET allowed_days_mask=0 WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateLaunchTicket(ctx, u.ID, connections[0].ID, "dev-device", "test"); !errors.Is(err, ErrPolicyRestricted) {
		t.Fatalf("restricted policy allowed launch: %v", err)
	}
	if _, err = s.DB.ExecContext(ctx, `UPDATE user_policies SET allowed_days_mask=127,max_session_minutes=5,daily_limit_minutes=0 WHERE user_id=?`, u.ID); err != nil {
		t.Fatal(err)
	}
	ticket, err := s.CreateLaunchTicket(ctx, u.ID, connections[0].ID, "dev-device", "test")
	if err != nil {
		t.Fatal(err)
	}
	var deviceID int64
	if err = s.DB.QueryRowContext(ctx, `SELECT id FROM devices WHERE device_identifier='dev-device'`).Scan(&deviceID); err != nil {
		t.Fatal(err)
	}
	manifest, err := s.RedeemLaunchTicket(ctx, ticket, deviceID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.MaxSessionSeconds != 300 {
		t.Fatalf("session cap=%d, want 300", manifest.MaxSessionSeconds)
	}
}
