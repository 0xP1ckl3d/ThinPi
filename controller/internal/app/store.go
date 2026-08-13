package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"thinpi.local/controller/internal/security"

	"golang.org/x/crypto/ssh"
)

var (
	ErrUnauthorised     = errors.New("unauthorised")
	ErrNotFound         = errors.New("not found")
	ErrRateLimited      = errors.New("rate limited")
	ErrTicketInvalid    = errors.New("launch ticket invalid")
	ErrPolicyRestricted = errors.New("access policy restricted")
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Store struct {
	DB          *sql.DB
	Vault       *security.Vault
	Now         func() time.Time
	SessionIdle time.Duration
	TicketTTL   time.Duration
}

func NewStore(db *sql.DB, vault *security.Vault, idle, ticketTTL time.Duration) *Store {
	return &Store{DB: db, Vault: vault, Now: func() time.Time { return time.Now().UTC() }, SessionIdle: idle, TicketTTL: ticketTTL}
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func (s *Store) CreateUser(ctx context.Context, username, displayName, password string, admin, enabled bool) (int64, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	displayName = strings.TrimSpace(displayName)
	if !identifierPattern.MatchString(username) || len(username) > 64 || displayName == "" || len(displayName) > 128 || strings.ContainsAny(displayName, "\r\n\x00") {
		return 0, errors.New("invalid user")
	}
	h, err := security.HashPassword(password)
	if err != nil {
		return 0, err
	}
	now := stamp(s.Now())
	r, err := s.DB.ExecContext(ctx, `INSERT INTO users(username,display_name,password_hash,is_admin,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, username, displayName, h, admin, enabled, now, now)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) LoginUsers(ctx context.Context) ([]LoginUser, bool, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,username,display_name,profile_photo IS NOT NULL,updated_at FROM users WHERE enabled=1 ORDER BY CASE WHEN last_login_at IS NULL THEN 1 ELSE 0 END,last_login_at DESC,display_name COLLATE NOCASE LIMIT 7`)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	users := make([]LoginUser, 0, 6)
	hasMore := false
	for rows.Next() {
		if len(users) == 6 {
			hasMore = true
			break
		}
		var user LoginUser
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.HasProfilePhoto, &user.ProfilePhotoVersion); err != nil {
			return nil, false, err
		}
		users = append(users, user)
	}
	return users, hasMore, rows.Err()
}

func (s *Store) UpdateOwnProfile(ctx context.Context, userID, sessionID int64, username, displayName, currentPassword, newPassword string) (User, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	displayName = strings.TrimSpace(displayName)
	if !identifierPattern.MatchString(username) || len(username) > 64 || displayName == "" || len(displayName) > 128 || strings.ContainsAny(displayName, "\r\n\x00") || currentPassword == "" {
		return User{}, errors.New("invalid profile")
	}
	var currentHash string
	var user User
	if err := s.DB.QueryRowContext(ctx, `SELECT id,username,display_name,password_hash,is_admin,enabled FROM users WHERE id=?`, userID).Scan(&user.ID, &user.Username, &user.DisplayName, &currentHash, &user.IsAdmin, &user.Enabled); err != nil {
		return User{}, err
	}
	if !security.VerifyPassword(currentHash, currentPassword) {
		return User{}, ErrUnauthorised
	}
	nextHash := currentHash
	if newPassword != "" {
		var err error
		nextHash, err = security.HashPassword(newPassword)
		if err != nil {
			return User{}, err
		}
	}
	now := stamp(s.Now())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE users SET username=?,display_name=?,password_hash=?,updated_at=? WHERE id=?`, username, displayName, nextHash, now, userID); err != nil {
		return User{}, err
	}
	if username != user.Username || newPassword != "" {
		if _, err = tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE user_id=? AND id<>? AND revoked_at IS NULL`, now, userID, sessionID); err != nil {
			return User{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return User{}, err
	}
	user.Username = username
	user.DisplayName = displayName
	s.audit(ctx, &userID, nil, "profile_updated", nil, "self_service", "success", nil)
	return user, nil
}

func (s *Store) Authenticate(ctx context.Context, username, password, source string) (string, string, User, error) {
	now := s.Now()
	key := strings.ToLower(strings.TrimSpace(username)) + "|" + source
	cutoff := stamp(now.Add(-10 * time.Minute))
	var failures int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM login_attempts WHERE source_key=? AND success=0 AND attempted_at>?`, key, cutoff).Scan(&failures)
	if failures >= 8 {
		s.audit(ctx, nil, nil, "login_failed", nil, source, "rate_limited", nil)
		return "", "", User{}, ErrRateLimited
	}
	var u User
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT id,username,display_name,password_hash,is_admin,enabled FROM users WHERE username=?`, strings.TrimSpace(username)).Scan(&u.ID, &u.Username, &u.DisplayName, &hash, &u.IsAdmin, &u.Enabled)
	valid := err == nil && u.Enabled && security.VerifyPassword(hash, password)
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO login_attempts(source_key,attempted_at,success) VALUES(?,?,?)`, key, stamp(now), valid)
	if !valid {
		// A real Argon2 calculation prevents a missing username from becoming a timing oracle.
		if errors.Is(err, sql.ErrNoRows) {
			dummy, _ := security.HashPassword("thinpi-dummy-password")
			security.VerifyPassword(dummy, password)
		}
		s.audit(ctx, nil, nil, "login_failed", nil, source, "denied", nil)
		return "", "", User{}, ErrUnauthorised
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return "", "", User{}, err
	}
	csrf, err := security.RandomToken(24)
	if err != nil {
		return "", "", User{}, err
	}
	idle := s.UserIdleTimeout(ctx, u.ID)
	_, err = s.DB.ExecContext(ctx, `INSERT INTO sessions(token_hash,csrf_hash,user_id,expires_at,last_seen_at,created_at) VALUES(?,?,?,?,?,?)`, security.TokenHash(token), security.TokenHash(csrf), u.ID, stamp(now.Add(idle)), stamp(now), stamp(now))
	if err != nil {
		return "", "", User{}, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE users SET last_login_at=?,updated_at=? WHERE id=?`, stamp(now), stamp(now), u.ID)
	s.audit(ctx, &u.ID, nil, "login_success", nil, source, "success", nil)
	return token, csrf, u, nil
}

func (s *Store) Session(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrUnauthorised
	}
	var sess Session
	var expires string
	var revoked sql.NullString
	err := s.DB.QueryRowContext(ctx, `SELECT s.id,u.id,u.username,u.display_name,u.is_admin,u.enabled,s.csrf_hash,s.expires_at,s.revoked_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, security.TokenHash(token)).Scan(&sess.ID, &sess.User.ID, &sess.User.Username, &sess.User.DisplayName, &sess.User.IsAdmin, &sess.User.Enabled, &sess.CSRFHash, &expires, &revoked)
	if err != nil || revoked.Valid || !sess.User.Enabled {
		return Session{}, ErrUnauthorised
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !s.Now().Before(exp) {
		return Session{}, ErrUnauthorised
	}
	now := s.Now()
	_, _ = s.DB.ExecContext(ctx, `UPDATE sessions SET last_seen_at=?,expires_at=? WHERE id=?`, stamp(now), stamp(now.Add(s.UserIdleTimeout(ctx, sess.User.ID))), sess.ID)
	return sess, nil
}

func (s *Store) UserIdleTimeout(ctx context.Context, userID int64) time.Duration {
	minutes := 30
	if err := s.DB.QueryRowContext(ctx, `SELECT idle_logout_minutes FROM user_policies WHERE user_id=?`, userID).Scan(&minutes); err != nil || minutes < 1 || minutes > 1440 {
		if fallback := int(s.SessionIdle / time.Minute); fallback >= 1 && fallback <= 1440 {
			minutes = fallback
		}
	}
	return time.Duration(minutes) * time.Minute
}

func (s *Store) Logout(ctx context.Context, sessionID, userID int64, source string) {
	now := stamp(s.Now())
	_, _ = s.DB.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE id=?`, now, sessionID)
	s.audit(ctx, &userID, nil, "logout", nil, source, "success", nil)
}

func (s *Store) CreateAdminHandoff(ctx context.Context, userID int64, source string) (string, error) {
	var authorised int
	if err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id=? AND is_admin=1 AND enabled=1`, userID).Scan(&authorised); err != nil {
		return "", ErrUnauthorised
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	now := s.Now()
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM admin_handoffs WHERE expires_at<? OR redeemed_at IS NOT NULL`, stamp(now.Add(-time.Hour)))
	_, err = s.DB.ExecContext(ctx, `INSERT INTO admin_handoffs(token_hash,user_id,expires_at,created_at) VALUES(?,?,?,?)`, security.TokenHash(token), userID, stamp(now.Add(45*time.Second)), stamp(now))
	if err == nil {
		s.audit(ctx, &userID, nil, "admin_handoff_created", nil, source, "success", nil)
	}
	return token, err
}

func (s *Store) RedeemAdminHandoff(ctx context.Context, token, source string) (string, error) {
	if token == "" {
		return "", ErrUnauthorised
	}
	sessionToken, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	csrf, err := security.RandomToken(24)
	if err != nil {
		return "", err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var userID int64
	var expires string
	var redeemed sql.NullString
	var idleMinutes int
	err = tx.QueryRowContext(ctx, `SELECT h.user_id,h.expires_at,h.redeemed_at,COALESCE(p.idle_logout_minutes,30) FROM admin_handoffs h JOIN users u ON u.id=h.user_id LEFT JOIN user_policies p ON p.user_id=u.id WHERE h.token_hash=? AND u.is_admin=1 AND u.enabled=1`, security.TokenHash(token)).Scan(&userID, &expires, &redeemed, &idleMinutes)
	expiry, parseErr := time.Parse(time.RFC3339Nano, expires)
	if err != nil || parseErr != nil || redeemed.Valid || !s.Now().Before(expiry) {
		return "", ErrUnauthorised
	}
	now := s.Now()
	result, err := tx.ExecContext(ctx, `UPDATE admin_handoffs SET redeemed_at=? WHERE token_hash=? AND redeemed_at IS NULL`, stamp(now), security.TokenHash(token))
	if err != nil {
		return "", err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return "", ErrUnauthorised
	}
	if idleMinutes < 1 || idleMinutes > 1440 {
		idleMinutes = 30
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO sessions(token_hash,csrf_hash,user_id,expires_at,last_seen_at,created_at) VALUES(?,?,?,?,?,?)`, security.TokenHash(sessionToken), security.TokenHash(csrf), userID, stamp(now.Add(time.Duration(idleMinutes)*time.Minute)), stamp(now), stamp(now))
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	s.audit(ctx, &userID, nil, "admin_handoff_redeemed", nil, source, "success", nil)
	return sessionToken, nil
}

func (s *Store) ConnectionsForUser(ctx context.Context, userID int64) ([]Connection, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT DISTINCT c.id,c.name,c.description,c.protocol,c.enabled,c.icon,c.sort_order FROM connections c LEFT JOIN user_connection_permissions up ON up.connection_id=c.id AND up.user_id=? LEFT JOIN group_connection_permissions gp ON gp.connection_id=c.id LEFT JOIN user_groups ug ON ug.group_id=gp.group_id AND ug.user_id=? WHERE c.enabled=1 AND ((up.can_launch=1) OR (gp.can_launch=1 AND ug.user_id IS NOT NULL)) ORDER BY c.sort_order,c.name`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Connection{}
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Protocol, &c.Enabled, &c.Icon, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CanLaunch(ctx context.Context, userID, connectionID int64) bool {
	var ok int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM connections c WHERE c.id=? AND c.enabled=1 AND (EXISTS(SELECT 1 FROM user_connection_permissions p WHERE p.user_id=? AND p.connection_id=c.id AND p.can_launch=1) OR EXISTS(SELECT 1 FROM user_groups ug JOIN group_connection_permissions p ON p.group_id=ug.group_id WHERE ug.user_id=? AND p.connection_id=c.id AND p.can_launch=1))`, connectionID, userID, userID).Scan(&ok)
	return err == nil
}

func (s *Store) AccessPolicy(ctx context.Context, userID int64) (AccessPolicy, error) {
	p := AccessPolicy{UserID: userID, Timezone: "Australia/Sydney", AllowedDaysMask: 127, AccessStartMinute: 0, AccessEndMinute: 1440, IdleLogoutMinutes: 30}
	err := s.DB.QueryRowContext(ctx, `SELECT timezone,allowed_days_mask,access_start_minute,access_end_minute,daily_limit_minutes,max_session_minutes,idle_logout_minutes FROM user_policies WHERE user_id=?`, userID).Scan(&p.Timezone, &p.AllowedDaysMask, &p.AccessStartMinute, &p.AccessEndMinute, &p.DailyLimitMinutes, &p.MaxSessionMinutes, &p.IdleLogoutMinutes)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	return p, err
}

func (s *Store) PolicyStatus(ctx context.Context, userID int64) (PolicyStatus, error) {
	p, err := s.AccessPolicy(ctx, userID)
	if err != nil {
		return PolicyStatus{}, err
	}
	loc, err := time.LoadLocation(p.Timezone)
	if err != nil {
		return PolicyStatus{}, errors.New("invalid policy timezone")
	}
	now := s.Now()
	local := now.In(loc)
	status := PolicyStatus{Allowed: true, Timezone: p.Timezone, DailyLimitMinutes: p.DailyLimitMinutes, MaxSessionMinutes: p.MaxSessionMinutes, IdleLogoutMinutes: p.IdleLogoutMinutes}
	dayBit := 1 << ((int(local.Weekday()) + 6) % 7)
	if p.AllowedDaysMask&dayBit == 0 {
		status.Allowed = false
		status.Reason = "Access is not available on " + local.Weekday().String() + "s."
	}
	minute := local.Hour()*60 + local.Minute()
	inWindow := p.AccessStartMinute == 0 && p.AccessEndMinute == 1440
	if !inWindow && p.AccessStartMinute < p.AccessEndMinute {
		inWindow = minute >= p.AccessStartMinute && minute < p.AccessEndMinute
	} else if !inWindow && p.AccessStartMinute > p.AccessEndMinute {
		inWindow = minute >= p.AccessStartMinute || minute < p.AccessEndMinute
	}
	if status.Allowed && !inWindow {
		status.Allowed = false
		status.Reason = fmt.Sprintf("Access is available from %02d:%02d to %02d:%02d.", p.AccessStartMinute/60, p.AccessStartMinute%60, p.AccessEndMinute/60, p.AccessEndMinute%60)
	}
	startLocal := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	rows, err := s.DB.QueryContext(ctx, `SELECT started_at,ended_at FROM session_usage WHERE user_id=? AND started_at>=?`, userID, stamp(startLocal.UTC()))
	if err != nil {
		return PolicyStatus{}, err
	}
	usedSeconds := 0
	for rows.Next() {
		var startedText string
		var endedText sql.NullString
		if rows.Scan(&startedText, &endedText) != nil {
			continue
		}
		started, parseErr := time.Parse(time.RFC3339Nano, startedText)
		if parseErr != nil {
			continue
		}
		ended := now
		if endedText.Valid {
			if parsed, parseErr := time.Parse(time.RFC3339Nano, endedText.String); parseErr == nil {
				ended = parsed
			}
		}
		if ended.After(started) {
			usedSeconds += int(ended.Sub(started).Seconds())
		}
	}
	rows.Close()
	status.DailyUsedMinutes = (usedSeconds + 59) / 60
	remainingSeconds := 0
	if p.DailyLimitMinutes > 0 {
		remainingSeconds = p.DailyLimitMinutes*60 - usedSeconds
		if status.Allowed && remainingSeconds <= 0 {
			status.Allowed = false
			status.Reason = "The daily time allowance has been used."
		}
	}
	if p.MaxSessionMinutes > 0 {
		status.EffectiveMaxSeconds = p.MaxSessionMinutes * 60
	}
	if remainingSeconds > 0 && (status.EffectiveMaxSeconds == 0 || remainingSeconds < status.EffectiveMaxSeconds) {
		status.EffectiveMaxSeconds = remainingSeconds
	}
	return status, nil
}

func (s *Store) CreateLaunchTicket(ctx context.Context, userID, connectionID int64, deviceIdentifier, source string) (string, error) {
	if !s.CanLaunch(ctx, userID, connectionID) {
		s.audit(ctx, &userID, nil, "launch_denied", &connectionID, source, "denied", nil)
		return "", ErrUnauthorised
	}
	policy, err := s.PolicyStatus(ctx, userID)
	if err != nil {
		return "", err
	}
	if !policy.Allowed {
		s.audit(ctx, &userID, nil, "launch_denied", &connectionID, source, "policy", map[string]string{"reason": policy.Reason})
		return "", ErrPolicyRestricted
	}
	var deviceID int64
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM devices WHERE device_identifier=? AND enabled=1`, deviceIdentifier).Scan(&deviceID)
	if err != nil {
		return "", ErrUnauthorised
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	now := s.Now()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO launch_tickets(token_hash,user_id,device_id,connection_id,expires_at,created_at,max_session_seconds) VALUES(?,?,?,?,?,?,?)`, security.TokenHash(token), userID, deviceID, connectionID, stamp(now.Add(s.TicketTTL)), stamp(now), policy.EffectiveMaxSeconds)
	if err == nil {
		s.audit(ctx, &userID, &deviceID, "launch_approved", &connectionID, source, "success", nil)
	}
	return token, err
}

// CreateMaintenanceTicket authorises one fixed local-console transition. It is
// intentionally separate from launch tickets: only an enabled administrator
// may create it and it is bound to one enabled ThinPi device.
func (s *Store) CreateMaintenanceTicket(ctx context.Context, userID int64, deviceIdentifier, source string) (string, error) {
	var deviceID int64
	err := s.DB.QueryRowContext(ctx, `SELECT d.id FROM devices d JOIN users u ON u.id=? WHERE d.device_identifier=? AND d.enabled=1 AND u.enabled=1 AND u.is_admin=1`, userID, deviceIdentifier).Scan(&deviceID)
	if err != nil {
		return "", ErrUnauthorised
	}
	token, err := security.RandomToken(32)
	if err != nil {
		return "", err
	}
	now := s.Now()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO maintenance_tickets(token_hash,user_id,device_id,expires_at,created_at) VALUES(?,?,?,?,?)`, security.TokenHash(token), userID, deviceID, stamp(now.Add(s.TicketTTL)), stamp(now))
	if err == nil {
		s.audit(ctx, &userID, &deviceID, "maintenance_authorised", nil, source, "success", nil)
	}
	return token, err
}

func (s *Store) RedeemMaintenanceTicket(ctx context.Context, token string, deviceID int64, source string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	var expires string
	var redeemed sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT t.user_id,t.expires_at,t.redeemed_at FROM maintenance_tickets t JOIN users u ON u.id=t.user_id WHERE t.token_hash=? AND t.device_id=? AND u.enabled=1 AND u.is_admin=1`, security.TokenHash(token), deviceID).Scan(&userID, &expires, &redeemed)
	if err != nil || redeemed.Valid {
		return ErrTicketInvalid
	}
	expiry, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !s.Now().Before(expiry) {
		return ErrTicketInvalid
	}
	result, err := tx.ExecContext(ctx, `UPDATE maintenance_tickets SET redeemed_at=? WHERE token_hash=? AND redeemed_at IS NULL`, stamp(s.Now()), security.TokenHash(token))
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrTicketInvalid
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.audit(ctx, &userID, &deviceID, "maintenance_started", nil, source, "success", nil)
	return nil
}

func (s *Store) RedeemLaunchTicket(ctx context.Context, token string, deviceID int64, source string) (LaunchManifest, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return LaunchManifest{}, err
	}
	defer tx.Rollback()
	var m LaunchManifest
	var userID int64
	var expires string
	var redeemed sql.NullString
	var credentialID sql.NullInt64
	var configText string
	err = tx.QueryRowContext(ctx, `SELECT t.id,t.user_id,t.connection_id,t.expires_at,t.redeemed_at,c.name,c.protocol,c.host,c.port,c.protocol_config_json,COALESCE((SELECT up.credential_id FROM user_connection_permissions up WHERE up.user_id=t.user_id AND up.connection_id=t.connection_id AND up.can_launch=1 AND up.credential_id IS NOT NULL),(SELECT gp.credential_id FROM user_groups ug JOIN group_connection_permissions gp ON gp.group_id=ug.group_id WHERE ug.user_id=t.user_id AND gp.connection_id=t.connection_id AND gp.can_launch=1 AND gp.credential_id IS NOT NULL ORDER BY gp.group_id LIMIT 1),c.credential_id),t.max_session_seconds FROM launch_tickets t JOIN connections c ON c.id=t.connection_id WHERE t.token_hash=? AND t.device_id=? AND c.enabled=1`, security.TokenHash(token), deviceID).Scan(&m.TicketID, &userID, &m.ConnectionID, &expires, &redeemed, &m.Name, &m.Protocol, &m.Host, &m.Port, &configText, &credentialID, &m.MaxSessionSeconds)
	if err != nil || redeemed.Valid {
		return LaunchManifest{}, ErrTicketInvalid
	}
	m.Config = json.RawMessage(configText)
	if err = tx.QueryRowContext(ctx, `SELECT terminal_theme FROM kiosk_settings WHERE id=1`).Scan(&m.TerminalTheme); err != nil {
		return LaunchManifest{}, err
	}
	exp, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !s.Now().Before(exp) {
		return LaunchManifest{}, ErrTicketInvalid
	}
	var authorised int
	err = tx.QueryRowContext(ctx, `SELECT 1 WHERE EXISTS(SELECT 1 FROM user_connection_permissions WHERE user_id=? AND connection_id=? AND can_launch=1) OR EXISTS(SELECT 1 FROM user_groups ug JOIN group_connection_permissions gp ON gp.group_id=ug.group_id WHERE ug.user_id=? AND gp.connection_id=? AND gp.can_launch=1)`, userID, m.ConnectionID, userID, m.ConnectionID).Scan(&authorised)
	if err != nil {
		return LaunchManifest{}, ErrTicketInvalid
	}
	if credentialID.Valid {
		var username sql.NullString
		var encrypted []byte
		err = tx.QueryRowContext(ctx, `SELECT username,encrypted_secret,secret_type FROM credentials WHERE id=?`, credentialID.Int64).Scan(&username, &encrypted, &m.CredentialType)
		if err != nil {
			return LaunchManifest{}, err
		}
		m.Username = username.String
		if len(encrypted) > 0 {
			plain, e := s.Vault.Decrypt(encrypted)
			if e != nil {
				return LaunchManifest{}, e
			}
			m.Password = string(plain)
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE launch_tickets SET redeemed_at=? WHERE id=? AND redeemed_at IS NULL`, stamp(s.Now()), m.TicketID)
	if err != nil {
		return LaunchManifest{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return LaunchManifest{}, ErrTicketInvalid
	}
	if err = tx.Commit(); err != nil {
		return LaunchManifest{}, err
	}
	s.audit(ctx, &userID, &deviceID, "launch_ticket_redeemed", &m.ConnectionID, source, "success", nil)
	return m, nil
}

func (s *Store) DeviceByToken(ctx context.Context, token string) (Device, error) {
	var d Device
	err := s.DB.QueryRowContext(ctx, `SELECT id,name,device_identifier,enabled FROM devices WHERE token_hash=?`, security.TokenHash(token)).Scan(&d.ID, &d.Name, &d.Identifier, &d.Enabled)
	if err != nil || !d.Enabled {
		return Device{}, ErrUnauthorised
	}
	return d, nil
}

func (s *Store) CreateEnrolmentToken(ctx context.Context, name string, ttl time.Duration) (string, error) {
	token, err := security.RandomToken(24)
	if err != nil {
		return "", err
	}
	now := s.Now()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO enrolment_tokens(token_hash,name,expires_at,created_at) VALUES(?,?,?,?)`, security.TokenHash(token), strings.TrimSpace(name), stamp(now.Add(ttl)), stamp(now))
	return token, err
}

func (s *Store) EnrolDevice(ctx context.Context, enrolToken, identifier, name, source string) (string, Device, error) {
	if !identifierPattern.MatchString(identifier) || name == "" || len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
		return "", Device{}, errors.New("invalid device")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", Device{}, err
	}
	defer tx.Rollback()
	var id int64
	var exp string
	var redeemed sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id,expires_at,redeemed_at FROM enrolment_tokens WHERE token_hash=?`, security.TokenHash(enrolToken)).Scan(&id, &exp, &redeemed)
	if err != nil || redeemed.Valid {
		return "", Device{}, ErrUnauthorised
	}
	expiry, e := time.Parse(time.RFC3339Nano, exp)
	if e != nil || !s.Now().Before(expiry) {
		return "", Device{}, ErrUnauthorised
	}
	credential, err := security.RandomToken(32)
	if err != nil {
		return "", Device{}, err
	}
	now := stamp(s.Now())
	r, err := tx.ExecContext(ctx, `INSERT INTO devices(name,device_identifier,token_hash,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?)`, name, identifier, security.TokenHash(credential), true, now, now)
	if err != nil {
		return "", Device{}, err
	}
	deviceID, _ := r.LastInsertId()
	if _, err = tx.ExecContext(ctx, `UPDATE enrolment_tokens SET redeemed_at=? WHERE id=? AND redeemed_at IS NULL`, now, id); err != nil {
		return "", Device{}, err
	}
	if err = tx.Commit(); err != nil {
		return "", Device{}, err
	}
	d := Device{ID: deviceID, Name: name, Identifier: identifier, Enabled: true}
	s.audit(ctx, nil, &deviceID, "device_enrolled", nil, source, "success", nil)
	return credential, d, nil
}

func (s *Store) Heartbeat(ctx context.Context, d Device, source string, versions json.RawMessage) error {
	if len(versions) == 0 || !json.Valid(versions) {
		versions = json.RawMessage(`{}`)
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE devices SET last_seen_at=?,last_ip=?,client_versions_json=?,updated_at=? WHERE id=? AND enabled=1`, stamp(s.Now()), source, string(versions), stamp(s.Now()), d.ID)
	return err
}

func (s *Store) SessionEvent(ctx context.Context, d Device, ticketID int64, connectionID *int64, event, result, source string, metadata json.RawMessage) {
	allowed := map[string]bool{"session_started": true, "session_exited": true, "session_failed": true}
	var userID, ticketConnectionID int64
	ticketErr := s.DB.QueryRowContext(ctx, `SELECT user_id,connection_id FROM launch_tickets WHERE id=? AND device_id=? AND redeemed_at IS NOT NULL`, ticketID, d.ID).Scan(&userID, &ticketConnectionID)
	if !allowed[event] || ticketErr != nil || connectionID == nil || *connectionID != ticketConnectionID {
		event = "session_event_rejected"
		result = "invalid"
	} else {
		now := s.Now()
		if event == "session_started" {
			_, _ = s.DB.ExecContext(ctx, `INSERT INTO session_usage(ticket_id,user_id,device_id,connection_id,started_at) VALUES(?,?,?,?,?) ON CONFLICT(ticket_id) DO NOTHING`, ticketID, userID, d.ID, ticketConnectionID, stamp(now))
		} else {
			var startedText string
			if err := s.DB.QueryRowContext(ctx, `SELECT started_at FROM session_usage WHERE ticket_id=?`, ticketID).Scan(&startedText); err == nil {
				if started, parseErr := time.Parse(time.RFC3339Nano, startedText); parseErr == nil {
					duration := max(0, int(now.Sub(started).Seconds()))
					_, _ = s.DB.ExecContext(ctx, `UPDATE session_usage SET ended_at=?,duration_seconds=? WHERE ticket_id=? AND ended_at IS NULL`, stamp(now), duration, ticketID)
				}
			}
		}
	}
	var m any
	if len(metadata) > 0 && json.Valid(metadata) {
		_ = json.Unmarshal(metadata, &m)
	}
	var actor *int64
	if userID != 0 {
		actor = &userID
	}
	s.audit(ctx, actor, &d.ID, event, connectionID, source, result, m)
}

func (s *Store) AuditAdmin(ctx context.Context, userID int64, source, resource string, id int64) {
	s.audit(ctx, &userID, nil, "admin_change", nil, source, "success", map[string]any{"resource": resource, "id": id})
}

func (s *Store) AuditDeviceRevoked(ctx context.Context, userID, deviceID int64, source string) {
	s.audit(ctx, &userID, &deviceID, "device_revocation", nil, source, "success", nil)
}

func (s *Store) audit(ctx context.Context, userID, deviceID *int64, event string, connectionID *int64, source, result string, metadata any) {
	raw := []byte(`{}`)
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			raw = b
		}
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO audit_events(timestamp,actor_user_id,device_id,event_type,connection_id,source_ip,result,metadata_json) VALUES(?,?,?,?,?,?,?,?)`, stamp(s.Now()), userID, deviceID, event, connectionID, source, result, string(raw))
}

func (s *Store) CreateCredential(ctx context.Context, name, username, secret, kind string) (int64, error) {
	if kind != "password" && kind != "username_only" && kind != "ssh_private_key" {
		return 0, errors.New("invalid credential type")
	}
	name = strings.TrimSpace(name)
	username = strings.TrimSpace(username)
	if name == "" || len(name) > 128 || len(username) > 256 || len(secret) > 16384 || strings.ContainsAny(username, "\r\n\x00") || strings.ContainsRune(secret, '\x00') {
		return 0, errors.New("invalid credential")
	}
	if kind == "password" && secret == "" {
		return 0, errors.New("password credential requires a secret")
	}
	if kind == "ssh_private_key" && (username == "" || !validSSHPrivateKey(secret)) {
		return 0, errors.New("SSH private-key credential requires a username and private key")
	}
	var encrypted []byte
	var err error
	if secret != "" {
		encrypted, err = s.Vault.Encrypt([]byte(secret))
		if err != nil {
			return 0, err
		}
	}
	now := stamp(s.Now())
	r, err := s.DB.ExecContext(ctx, `INSERT INTO credentials(name,username,encrypted_secret,secret_type,created_at,updated_at) VALUES(?,?,?,?,?,?)`, name, username, encrypted, kind, now, now)
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func validSSHPrivateKey(secret string) bool {
	_, err := ssh.ParsePrivateKey([]byte(strings.TrimSpace(secret)))
	return err == nil
}

func (s *Store) ReplaceCredential(ctx context.Context, id int64, username, secret string) error {
	var kind string
	var currentUsername sql.NullString
	if err := s.DB.QueryRowContext(ctx, `SELECT username,secret_type FROM credentials WHERE id=?`, id).Scan(&currentUsername, &kind); err != nil {
		return err
	}
	if strings.TrimSpace(username) == "" {
		username = currentUsername.String
	}
	username = strings.TrimSpace(username)
	if secret == "" || len(secret) > 16384 || len(username) > 256 || strings.ContainsAny(username, "\r\n\x00") || strings.ContainsRune(secret, '\x00') {
		return errors.New("invalid credential")
	}
	if kind == "ssh_private_key" && (username == "" || !validSSHPrivateKey(secret)) {
		return errors.New("invalid SSH private key")
	}
	encrypted, err := s.Vault.Encrypt([]byte(secret))
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE credentials SET username=?,encrypted_secret=?,updated_at=? WHERE id=?`, username, encrypted, stamp(s.Now()), id)
	return err
}

func (s *Store) SeedDev(ctx context.Context) error {
	var count int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return s.ensureDevEnhancements(ctx)
	}
	admin, err := s.CreateUser(ctx, "admin", "Administrator", "thinpi-dev", true, true)
	if err != nil {
		return err
	}
	wife, err := s.CreateUser(ctx, "wife", "Wife", "thinpi-dev", false, true)
	if err != nil {
		return err
	}
	daughter, err := s.CreateUser(ctx, "daughter", "Daughter", "thinpi-dev", false, true)
	if err != nil {
		return err
	}
	now := stamp(s.Now())
	r, err := s.DB.ExecContext(ctx, `INSERT INTO groups(name,description,created_at,updated_at) VALUES('Family','Shared family systems',?,?)`, now, now)
	if err != nil {
		return err
	}
	family, _ := r.LastInsertId()
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO user_groups(user_id,group_id) VALUES(?,?),(?,?)`, wife, family, daughter, family)
	configs := []struct {
		name, protocol, host string
		port                 int
		cfg                  string
	}{{"Mock School PC", "rdp", "school-pc.invalid", 3389, `{"fullscreen":true,"dynamic_resolution":true,"audio":true,"clipboard":false,"auto_reconnect":true}`}, {"Mock Gaming PC", "moonlight", "gaming-pc.invalid", 47984, `{"application":"Desktop","width":1920,"height":1080,"fps":60,"bitrate_kbps":20000,"audio":true,"gamepad":true}`}, {"Mock Admin PC", "rdp", "admin-pc.invalid", 3389, `{"fullscreen":true}`}}
	ids := make([]int64, 0, 3)
	for i, c := range configs {
		r, err = s.DB.ExecContext(ctx, `INSERT INTO connections(name,protocol,host,port,sort_order,protocol_config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`, c.name, c.protocol, c.host, c.port, i, c.cfg, now, now)
		if err != nil {
			return err
		}
		id, _ := r.LastInsertId()
		ids = append(ids, id)
	}
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO user_connection_permissions(user_id,connection_id) VALUES(?,?),(?,?),(?,?),(?,?)`, daughter, ids[0], daughter, ids[1], wife, ids[1], admin, ids[2])
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO group_connection_permissions(group_id,connection_id) VALUES(?,?)`, family, ids[0])
	// A known development device makes the entire local ticket flow runnable without manual enrolment.
	_, _ = s.DB.ExecContext(ctx, `INSERT INTO devices(name,device_identifier,token_hash,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?)`, "Development Pi", "dev-device", security.TokenHash("dev-device-token"), true, now, now)
	return s.ensureDevEnhancements(ctx)
}

func (s *Store) ensureDevEnhancements(ctx context.Context) error {
	var daughter int64
	if err := s.DB.QueryRowContext(ctx, `SELECT id FROM users WHERE username='daughter'`).Scan(&daughter); err != nil {
		return err
	}
	now := stamp(s.Now())
	_, err := s.DB.ExecContext(ctx, `INSERT INTO user_policies(user_id,timezone,allowed_days_mask,access_start_minute,access_end_minute,daily_limit_minutes,max_session_minutes,updated_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(user_id) DO NOTHING`, daughter, "Australia/Sydney", 127, 0, 1440, 120, 30, now)
	if err != nil {
		return err
	}
	var connectionID int64
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM connections WHERE name='Demo Linux Desktop'`).Scan(&connectionID)
	if errors.Is(err, sql.ErrNoRows) {
		r, insertErr := s.DB.ExecContext(ctx, `INSERT INTO connections(name,description,protocol,host,port,enabled,icon,sort_order,protocol_config_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, "Demo Linux Desktop", "Linux workstation over VNC", "vnc", "linux-workstation.invalid", 5900, true, "linux", 3, `{"fullscreen":true,"shared":true,"view_only":false}`, now, now)
		if insertErr != nil {
			return insertErr
		}
		connectionID, _ = r.LastInsertId()
	} else if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO user_connection_permissions(user_id,connection_id,can_launch) VALUES(?,?,1) ON CONFLICT(user_id,connection_id) DO UPDATE SET can_launch=1`, daughter, connectionID)
	if err != nil {
		return err
	}
	var schoolConnectionID, schoolCredentialID int64
	if err = s.DB.QueryRowContext(ctx, `SELECT id FROM connections WHERE name='Mock School PC'`).Scan(&schoolConnectionID); err != nil {
		return err
	}
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM credentials WHERE name='Daughter school account'`).Scan(&schoolCredentialID)
	if errors.Is(err, sql.ErrNoRows) {
		schoolCredentialID, err = s.CreateCredential(ctx, "Daughter school account", "student.daughter", "school-demo-password", "password")
	}
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE user_connection_permissions SET credential_id=? WHERE user_id=? AND connection_id=?`, schoolCredentialID, daughter, schoolConnectionID)
	return err
}

func (s *Store) Reset(ctx context.Context) error {
	return fmt.Errorf("reset is implemented by deleting the development database before opening it")
}
