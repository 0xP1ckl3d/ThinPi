package web

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"thinpi.local/controller/internal/app"
	"thinpi.local/controller/internal/security"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	Store   *app.Store
	Log     *slog.Logger
	Dev     bool
	Version string
	Handler http.Handler
}
type ctxKey string

const sessionKey ctxKey = "session"

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func New(store *app.Store, logger *slog.Logger, dev bool, version string) *Server {
	s := &Server{Store: store, Log: logger, Dev: dev, Version: version}
	mux := http.NewServeMux()
	assets, _ := fs.Sub(staticFS, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("GET /{$}", s.root)
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("GET /api/v1/login-users", s.loginUsers)
	mux.HandleFunc("GET /api/v1/profile-photos/{id}", s.profilePhoto)
	mux.Handle("POST /api/v1/auth/logout", s.withSession(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/me", s.withSession(http.HandlerFunc(s.me)))
	mux.Handle("PUT /api/v1/me", s.withSession(http.HandlerFunc(s.updateMe)))
	mux.Handle("POST /api/v1/admin-handoff", s.withAdmin(http.HandlerFunc(s.createAdminHandoff)))
	mux.Handle("POST /api/v1/maintenance", s.withAdmin(http.HandlerFunc(s.createMaintenanceTicket)))
	mux.Handle("GET /api/v1/connections", s.withSession(http.HandlerFunc(s.connections)))
	mux.Handle("POST /api/v1/connections/{id}/launch", s.withSession(http.HandlerFunc(s.launch)))
	mux.HandleFunc("POST /api/v1/devices/enrol", s.enrol)
	mux.Handle("POST /api/v1/agent/redeem-launch", s.withDevice(http.HandlerFunc(s.redeem)))
	mux.Handle("POST /api/v1/agent/redeem-maintenance", s.withDevice(http.HandlerFunc(s.redeemMaintenance)))
	mux.Handle("POST /api/v1/agent/heartbeat", s.withDevice(http.HandlerFunc(s.heartbeat)))
	mux.Handle("POST /api/v1/agent/session-event", s.withDevice(http.HandlerFunc(s.sessionEvent)))
	mux.HandleFunc("GET /admin/login", s.adminLoginPage)
	mux.HandleFunc("POST /admin/login", s.adminLogin)
	mux.HandleFunc("GET /admin/handoff", s.redeemAdminHandoff)
	mux.HandleFunc("GET /admin/{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/admin", http.StatusSeeOther) })
	mux.Handle("GET /admin", s.withAdminPage(http.HandlerFunc(s.adminPage)))
	mux.Handle("POST /admin/logout", s.withAdminPage(http.HandlerFunc(s.logout)))
	mux.Handle("GET /api/v1/admin/{resource}", s.withAdmin(http.HandlerFunc(s.adminList)))
	mux.Handle("POST /api/v1/admin/{resource}", s.withAdmin(http.HandlerFunc(s.adminCreate)))
	mux.Handle("PUT /api/v1/admin/{resource}/{id}", s.withAdmin(http.HandlerFunc(s.adminUpdate)))
	mux.Handle("DELETE /api/v1/admin/{resource}/{id}", s.withAdmin(http.HandlerFunc(s.adminDelete)))
	s.Handler = s.middleware(mux)
	return s
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		id, _ := security.RandomToken(9)
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		recorder := &statusWriter{ResponseWriter: w}
		defer func() {
			status := recorder.status
			if status == 0 {
				status = 200
			}
			s.Log.Info("http_request", "request_id", id, "method", r.Method, "path", r.URL.Path, "status", status, "source_ip", remoteIP(r), "duration_ms", time.Since(started).Milliseconds())
		}()
		if !s.Dev && r.TLS == nil {
			s.error(recorder, r, http.StatusUpgradeRequired, "TLS_REQUIRED", "HTTPS is required.")
			return
		}
		next.ServeHTTP(recorder, r)
	})
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func tokenFrom(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer "), false
	}
	if c, err := r.Cookie("thinpi_session"); err == nil {
		return c.Value, true
	}
	return "", false
}
func (s *Server) sessionFrom(r *http.Request) (app.Session, bool, error) {
	token, cookie := tokenFrom(r)
	session, err := s.Store.Session(r.Context(), token)
	return session, cookie, err
}
func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, cookie, err := s.sessionFrom(r)
		if err != nil {
			s.error(w, r, 401, "AUTHENTICATION_REQUIRED", "Please sign in.")
			return
		}
		if cookie && isMutation(r.Method) {
			candidate := r.Header.Get("X-CSRF-Token")
			if candidate == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				_ = r.ParseForm()
				candidate = r.FormValue("csrf")
			}
			if subtle.ConstantTimeCompare([]byte(sess.CSRFHash), []byte(security.TokenHash(candidate))) != 1 {
				s.error(w, r, 403, "CSRF_INVALID", "The request could not be verified.")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}
func (s *Server) withAdminPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess, cookie, err := s.sessionFrom(r)
		if err != nil || !sess.User.IsAdmin {
			if cookie {
				s.clearSessionCookie(w)
			}
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
		if cookie && isMutation(r.Method) {
			candidate := r.Header.Get("X-CSRF-Token")
			if candidate == "" && strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				_ = r.ParseForm()
				candidate = r.FormValue("csrf")
			}
			if subtle.ConstantTimeCompare([]byte(sess.CSRFHash), []byte(security.TokenHash(candidate))) != 1 {
				s.error(w, r, 403, "CSRF_INVALID", "The request could not be verified.")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}
func (s *Server) withAdmin(next http.Handler) http.Handler {
	return s.withSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := r.Context().Value(sessionKey).(app.Session)
		if !sess.User.IsAdmin {
			s.error(w, r, 403, "ADMIN_REQUIRED", "Administrator access is required.")
			return
		}
		next.ServeHTTP(w, r)
	}))
}
func (s *Server) withDevice(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, _ := tokenFrom(r)
		d, err := s.Store.DeviceByToken(r.Context(), tok)
		if err != nil {
			s.error(w, r, 401, "DEVICE_AUTHENTICATION_REQUIRED", "Device authentication failed.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey("device"), d)))
	})
}
func isMutation(m string) bool { return m != "GET" && m != "HEAD" && m != "OPTIONS" }
func decode(r *http.Request, v any) error {
	r.Body = io.NopCloser(io.LimitReader(r.Body, 1<<20))
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if err := d.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (s *Server) error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "request_id": w.Header().Get("X-Request-ID")}})
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.DB.PingContext(r.Context()); err != nil {
		s.error(w, r, 503, "DATABASE_UNAVAILABLE", "Service unavailable.")
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ok", "version": s.Version})
}
func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	sess, cookie, err := s.sessionFrom(r)
	if err == nil && sess.User.IsAdmin {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}
	if cookie {
		s.clearSessionCookie(w)
	}
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "thinpi_session", Value: "", Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteStrictMode, MaxAge: -1})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid login request.")
		return
	}
	token, csrf, u, err := s.Store.Authenticate(r.Context(), q.Username, q.Password, remoteIP(r))
	if errors.Is(err, app.ErrRateLimited) {
		s.error(w, r, 429, "AUTHENTICATION_RATE_LIMITED", "Too many failed attempts. Try again later.")
		return
	}
	if err != nil {
		s.error(w, r, 401, "INVALID_CREDENTIALS", "The username or password was incorrect.")
		return
	}
	// The database expiry is sliding and authoritative. A session cookie avoids
	// imposing a second, fixed browser-side deadline while the user is active.
	http.SetCookie(w, &http.Cookie{Name: "thinpi_session", Value: token, Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteStrictMode})
	writeJSON(w, 200, map[string]any{"token": token, "csrf_token": csrf, "user": u})
}
func (s *Server) loginUsers(w http.ResponseWriter, r *http.Request) {
	var screenSleepMinutes int
	var showUserList bool
	var terminalTheme, clientTheme string
	err := s.Store.DB.QueryRowContext(r.Context(), `SELECT screen_sleep_minutes,show_user_list,terminal_theme,client_theme FROM kiosk_settings WHERE id=1`).Scan(&screenSleepMinutes, &showUserList, &terminalTheme, &clientTheme)
	if err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Unable to load kiosk settings.")
		return
	}
	users := []app.LoginUser{}
	hasMore := !showUserList
	if showUserList {
		users, hasMore, err = s.Store.LoginUsers(r.Context())
		for i := range users {
			if users[i].HasProfilePhoto {
				version := strings.ReplaceAll(users[i].ProfilePhotoVersion, ":", "")
				users[i].ProfilePhotoURL = "/api/v1/profile-photos/" + strconv.FormatInt(users[i].ID, 10) + "?v=" + version
			}
		}
	}
	if err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Unable to load sign-in choices.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"items": users, "has_more": hasMore, "configuration": map[string]any{"screen_sleep_minutes": screenSleepMinutes, "terminal_theme": terminalTheme, "client_theme": clientTheme}})
}

func (s *Server) profilePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var photo []byte
	var mime string
	if err != nil || s.Store.DB.QueryRowContext(r.Context(), `SELECT profile_photo,profile_photo_mime FROM users WHERE id=? AND enabled=1 AND profile_photo IS NOT NULL`, id).Scan(&photo, &mime) != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(photo)
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	s.Store.Logout(r.Context(), sess.ID, sess.User.ID, remoteIP(r))
	s.clearSessionCookie(w)
	if strings.HasPrefix(r.URL.Path, "/admin/") {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"user": r.Context().Value(sessionKey).(app.Session).User})
}
func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	var q struct {
		Username        string `json:"username"`
		DisplayName     string `json:"display_name"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid profile update.")
		return
	}
	user, err := s.Store.UpdateOwnProfile(r.Context(), sess.User.ID, sess.ID, q.Username, q.DisplayName, q.CurrentPassword, q.NewPassword)
	if errors.Is(err, app.ErrUnauthorised) {
		s.error(w, r, 403, "INVALID_CURRENT_PASSWORD", "Your current password was incorrect.")
		return
	}
	if err != nil {
		s.error(w, r, 400, "INVALID_PROFILE", "The username may already be used, or the new password is shorter than 8 characters.")
		return
	}
	writeJSON(w, 200, map[string]any{"user": user})
}
func (s *Server) createAdminHandoff(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	token, err := s.Store.CreateAdminHandoff(r.Context(), sess.User.ID, remoteIP(r))
	if err != nil {
		s.error(w, r, 403, "ADMIN_REQUIRED", "Administrator access is required.")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"path": "/admin/handoff?code=" + token, "expires_in": 45})
}
func (s *Server) redeemAdminHandoff(w http.ResponseWriter, r *http.Request) {
	token, err := s.Store.RedeemAdminHandoff(r.Context(), r.URL.Query().Get("code"), remoteIP(r))
	if err != nil {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "thinpi_session", Value: token, Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
func (s *Server) connections(w http.ResponseWriter, r *http.Request) {
	u := r.Context().Value(sessionKey).(app.Session).User
	items, err := s.Store.ConnectionsForUser(r.Context(), u.ID)
	if err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Unable to load connections.")
		return
	}
	policy, err := s.Store.PolicyStatus(r.Context(), u.ID)
	if err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Unable to evaluate access policy.")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "policy": policy})
}
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var q struct {
		DeviceIdentifier string `json:"device_identifier"`
	}
	if err != nil || decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid launch request.")
		return
	}
	u := r.Context().Value(sessionKey).(app.Session).User
	ticket, err := s.Store.CreateLaunchTicket(r.Context(), u.ID, id, q.DeviceIdentifier, remoteIP(r))
	if err != nil {
		if errors.Is(err, app.ErrPolicyRestricted) {
			policy, _ := s.Store.PolicyStatus(r.Context(), u.ID)
			s.error(w, r, 403, "ACCESS_POLICY_RESTRICTED", policy.Reason)
			return
		}
		s.error(w, r, 403, "CONNECTION_NOT_AUTHORISED", "You are not authorised to launch this connection.")
		return
	}
	writeJSON(w, 201, map[string]any{"ticket": ticket, "expires_in": int(s.Store.TicketTTL.Seconds())})
}

func (s *Server) createMaintenanceTicket(w http.ResponseWriter, r *http.Request) {
	var q struct {
		DeviceIdentifier string `json:"device_identifier"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid maintenance request.")
		return
	}
	u := r.Context().Value(sessionKey).(app.Session).User
	ticket, err := s.Store.CreateMaintenanceTicket(r.Context(), u.ID, q.DeviceIdentifier, remoteIP(r))
	if err != nil {
		s.error(w, r, 403, "MAINTENANCE_NOT_AUTHORISED", "Local maintenance is not authorised for this device.")
		return
	}
	writeJSON(w, 201, map[string]any{"ticket": ticket, "expires_in": int(s.Store.TicketTTL.Seconds())})
}
func (s *Server) enrol(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Token            string `json:"token"`
		DeviceIdentifier string `json:"device_identifier"`
		Name             string `json:"name"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid enrolment request.")
		return
	}
	credential, d, err := s.Store.EnrolDevice(r.Context(), q.Token, q.DeviceIdentifier, q.Name, remoteIP(r))
	if err != nil {
		s.error(w, r, 401, "ENROLMENT_INVALID", "The enrolment token is invalid or expired.")
		return
	}
	writeJSON(w, 201, map[string]any{"device": d, "device_token": credential})
}
func (s *Server) redeem(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Ticket string `json:"ticket"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid ticket request.")
		return
	}
	d := r.Context().Value(ctxKey("device")).(app.Device)
	m, err := s.Store.RedeemLaunchTicket(r.Context(), q.Ticket, d.ID, remoteIP(r))
	if err != nil {
		s.error(w, r, 403, "LAUNCH_TICKET_INVALID", "The launch ticket is invalid, expired, used, or belongs to another device.")
		return
	}
	writeJSON(w, 200, map[string]any{"manifest": m})
}

func (s *Server) redeemMaintenance(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Ticket string `json:"ticket"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid maintenance ticket request.")
		return
	}
	d := r.Context().Value(ctxKey("device")).(app.Device)
	if err := s.Store.RedeemMaintenanceTicket(r.Context(), q.Ticket, d.ID, remoteIP(r)); err != nil {
		s.error(w, r, 403, "MAINTENANCE_TICKET_INVALID", "The maintenance ticket is invalid, expired, used, or belongs to another device.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Versions json.RawMessage `json:"versions"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid heartbeat.")
		return
	}
	d := r.Context().Value(ctxKey("device")).(app.Device)
	if err := s.Store.Heartbeat(r.Context(), d, remoteIP(r), q.Versions); err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Heartbeat failed.")
		return
	}
	w.WriteHeader(204)
}
func (s *Server) sessionEvent(w http.ResponseWriter, r *http.Request) {
	var q struct {
		TicketID     int64           `json:"ticket_id"`
		ConnectionID *int64          `json:"connection_id"`
		Event        string          `json:"event"`
		Result       string          `json:"result"`
		Metadata     json.RawMessage `json:"metadata"`
	}
	if decode(r, &q) != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid session event.")
		return
	}
	d := r.Context().Value(ctxKey("device")).(app.Device)
	s.Store.SessionEvent(r.Context(), d, q.TicketID, q.ConnectionID, q.Event, q.Result, remoteIP(r), q.Metadata)
	w.WriteHeader(204)
}

var loginTemplate = template.Must(template.New("login").Parse(`<!doctype html><html lang="en-AU"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>ThinPi administration</title><link rel="stylesheet" href="/static/admin.css"></head><body><main class="login"><section class="active"><h1>ThinPi administration</h1><p class="muted">Sign in with an administrator account.</p>{{if .}}<p class="danger">{{.}}</p>{{end}}<form method="post"><p><label>Username<input name="username" autocomplete="username" required></label></p><p><label>Password<input type="password" name="password" autocomplete="current-password" required></label></p><button class="primary">Sign in</button></form></section></main></body></html>`))

func (s *Server) adminLoginPage(w http.ResponseWriter, r *http.Request) {
	if sess, cookie, err := s.sessionFrom(r); err == nil && sess.User.IsAdmin {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	} else if cookie {
		s.clearSessionCookie(w)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = loginTemplate.Execute(w, nil)
}
func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.adminLoginPage(w, r)
		return
	}
	token, _, u, err := s.Store.Authenticate(r.Context(), r.FormValue("username"), r.FormValue("password"), remoteIP(r))
	if err != nil || !u.IsAdmin {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = loginTemplate.Execute(w, "The username or password was incorrect.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "thinpi_session", Value: token, Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/admin", 303)
}

var adminTemplate = template.Must(template.ParseFS(staticFS, "static/admin.html"))

func (s *Server) adminPage(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	csrf, _ := security.RandomToken(24)
	_, _ = s.Store.DB.ExecContext(r.Context(), `UPDATE sessions SET csrf_hash=? WHERE id=?`, security.TokenHash(csrf), sess.ID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminTemplate.ExecuteTemplate(w, "admin.html", map[string]any{"CSRF": csrf, "Name": sess.User.DisplayName, "Dev": s.Dev})
}

func (s *Server) adminList(w http.ResponseWriter, r *http.Request) {
	resource := r.PathValue("resource")
	limit := 100
	if n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n > 0 && n <= 500 {
		limit = n
	}
	queries := map[string]string{
		"users":       `SELECT id,username,display_name,is_admin,enabled,profile_photo IS NOT NULL AS has_profile_photo,last_login_at,updated_at,created_at FROM users ORDER BY username`,
		"groups":      `SELECT id,name,description,created_at FROM groups ORDER BY name`,
		"connections": `SELECT id,name,description,protocol,host,port,enabled,icon,sort_order,protocol_config_json,credential_id FROM connections ORDER BY sort_order,name`,
		"credentials": `SELECT id,name,username,secret_type,created_at,updated_at FROM credentials ORDER BY name`,
		"devices":     `SELECT id,name,device_identifier,enabled,last_seen_at,last_ip,client_versions_json,created_at FROM devices ORDER BY name`,
		"policies":    `SELECT u.id AS user_id,u.username,u.display_name,COALESCE(p.timezone,'Australia/Sydney') AS timezone,COALESCE(p.allowed_days_mask,127) AS allowed_days_mask,COALESCE(p.access_start_minute,0) AS access_start_minute,COALESCE(p.access_end_minute,1440) AS access_end_minute,COALESCE(p.daily_limit_minutes,0) AS daily_limit_minutes,COALESCE(p.max_session_minutes,0) AS max_session_minutes,COALESCE(p.idle_logout_minutes,30) AS idle_logout_minutes FROM users u LEFT JOIN user_policies p ON p.user_id=u.id ORDER BY u.username`,
		"permissions": `SELECT 'user' AS subject_type,u.id AS subject_id,u.display_name AS subject_name,c.id AS connection_id,c.name AS connection_name,p.can_launch,p.credential_id,cr.name AS credential_name FROM user_connection_permissions p JOIN users u ON u.id=p.user_id JOIN connections c ON c.id=p.connection_id LEFT JOIN credentials cr ON cr.id=p.credential_id UNION ALL SELECT 'group',g.id,g.name,c.id,c.name,p.can_launch,p.credential_id,cr.name FROM group_connection_permissions p JOIN groups g ON g.id=p.group_id JOIN connections c ON c.id=p.connection_id LEFT JOIN credentials cr ON cr.id=p.credential_id ORDER BY subject_type,subject_name,connection_name`,
		"audit":       fmt.Sprintf(`SELECT id,timestamp,actor_user_id,device_id,event_type,connection_id,source_ip,result,metadata_json FROM audit_events ORDER BY timestamp DESC LIMIT %d`, limit),
		"settings":    `SELECT id,screen_sleep_minutes,show_user_list,terminal_theme,client_theme,updated_at FROM kiosk_settings WHERE id=1`,
	}
	if resource == "dashboard" {
		var users, devices, online, failures, launches int
		s.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM users WHERE enabled=1`).Scan(&users)
		s.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM devices WHERE enabled=1`).Scan(&devices)
		s.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM devices WHERE enabled=1 AND last_seen_at>?`, time.Now().UTC().Add(-2*time.Minute).Format(time.RFC3339Nano)).Scan(&online)
		s.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM audit_events WHERE event_type='login_failed' AND timestamp>?`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339Nano)).Scan(&failures)
		s.Store.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM audit_events WHERE event_type='launch_approved' AND timestamp>?`, time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339Nano)).Scan(&launches)
		writeJSON(w, 200, map[string]any{"enabled_users": users, "enrolled_devices": devices, "online_devices": online, "failed_logins_24h": failures, "launches_24h": launches, "dev_mode": s.Dev})
		return
	}
	q, ok := queries[resource]
	if !ok {
		s.error(w, r, 404, "NOT_FOUND", "Resource not found.")
		return
	}
	rows, err := s.Store.DB.QueryContext(r.Context(), q)
	if err != nil {
		s.error(w, r, 500, "INTERNAL_ERROR", "Unable to load resource.")
		return
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	items := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if rows.Scan(ptrs...) != nil {
			continue
		}
		m := map[string]any{}
		for i, c := range cols {
			v := values[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			if c == "is_admin" || c == "enabled" || c == "can_launch" || c == "has_profile_photo" || c == "show_user_list" {
				if n, ok := v.(int64); ok {
					v = n != 0
				}
			}
			m[c] = v
		}
		items = append(items, m)
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (s *Server) adminCreate(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	resource := r.PathValue("resource")
	var id int64
	var err error
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch resource {
	case "users":
		var q struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Password    string `json:"password"`
			IsAdmin     bool   `json:"is_admin"`
			Enabled     bool   `json:"enabled"`
		}
		if decode(r, &q) == nil {
			id, err = s.Store.CreateUser(r.Context(), q.Username, q.DisplayName, q.Password, q.IsAdmin, q.Enabled)
		} else {
			err = errors.New("invalid request")
		}
	case "groups":
		var q struct{ Name, Description string }
		if decode(r, &q) == nil && validLabel(q.Name, 128) && len(q.Description) <= 1024 {
			var x sql.Result
			x, err = s.Store.DB.ExecContext(r.Context(), `INSERT INTO groups(name,description,created_at,updated_at) VALUES(?,?,?,?)`, q.Name, q.Description, now, now)
			if err == nil {
				id, _ = x.LastInsertId()
			}
		} else {
			err = errors.New("invalid request")
		}
	case "connections":
		var q struct {
			Name           string          `json:"name"`
			Description    string          `json:"description"`
			Protocol       string          `json:"protocol"`
			Host           string          `json:"host"`
			Icon           string          `json:"icon"`
			Port           int             `json:"port"`
			SortOrder      int             `json:"sort_order"`
			Enabled        bool            `json:"enabled"`
			ProtocolConfig json.RawMessage `json:"protocol_config"`
			CredentialID   *int64          `json:"credential_id"`
		}
		if decode(r, &q) == nil && validLabel(q.Name, 128) && len(q.Description) <= 1024 && len(q.Icon) <= 128 && s.validProtocol(q.Protocol) && validHost(q.Host) && q.Port > 0 && q.Port < 65536 && validProtocolConfig(q.Protocol, q.ProtocolConfig) && s.credentialAllowedForProtocol(r.Context(), q.CredentialID, q.Protocol) {
			var x sql.Result
			x, err = s.Store.DB.ExecContext(r.Context(), `INSERT INTO connections(name,description,protocol,host,port,enabled,icon,sort_order,protocol_config_json,credential_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, q.Name, q.Description, q.Protocol, q.Host, q.Port, q.Enabled, q.Icon, q.SortOrder, string(q.ProtocolConfig), q.CredentialID, now, now)
			if err == nil {
				id, _ = x.LastInsertId()
			}
		} else {
			err = errors.New("invalid connection")
		}
	case "credentials":
		var q struct {
			Name       string `json:"name"`
			Username   string `json:"username"`
			Secret     string `json:"secret"`
			SecretType string `json:"secret_type"`
		}
		if decode(r, &q) == nil {
			id, err = s.Store.CreateCredential(r.Context(), q.Name, q.Username, q.Secret, q.SecretType)
		} else {
			err = errors.New("invalid request")
		}
	case "enrolment-tokens":
		var q struct {
			Name       string `json:"name"`
			TTLMinutes int    `json:"ttl_minutes"`
		}
		if decode(r, &q) != nil || q.TTLMinutes < 1 || q.TTLMinutes > 1440 {
			s.error(w, r, 400, "INVALID_REQUEST", "Invalid enrolment token request.")
			return
		}
		token, e := s.Store.CreateEnrolmentToken(r.Context(), q.Name, time.Duration(q.TTLMinutes)*time.Minute)
		if e != nil {
			s.error(w, r, 500, "INTERNAL_ERROR", "Unable to create token.")
			return
		}
		writeJSON(w, 201, map[string]string{"token": token})
		return
	case "permissions":
		var q struct {
			SubjectType  string `json:"subject_type"`
			SubjectID    int64  `json:"subject_id"`
			ConnectionID int64  `json:"connection_id"`
			CanLaunch    bool   `json:"can_launch"`
			CredentialID *int64 `json:"credential_id"`
		}
		if decode(r, &q) != nil {
			s.error(w, r, 400, "INVALID_REQUEST", "Invalid assignment.")
			return
		}
		table, col := "user_connection_permissions", "user_id"
		if q.SubjectType == "group" {
			table, col = "group_connection_permissions", "group_id"
		} else if q.SubjectType != "user" {
			s.error(w, r, 400, "INVALID_REQUEST", "Invalid assignment type.")
			return
		}
		if q.CanLaunch {
			var protocol string
			if lookupErr := s.Store.DB.QueryRowContext(r.Context(), `SELECT protocol FROM connections WHERE id=?`, q.ConnectionID).Scan(&protocol); lookupErr != nil || !s.credentialAllowedForProtocol(r.Context(), q.CredentialID, protocol) {
				err = errors.New("credential is not compatible with the connection")
			} else {
				_, err = s.Store.DB.ExecContext(r.Context(), fmt.Sprintf(`INSERT INTO %s(%s,connection_id,can_launch,credential_id) VALUES(?,?,1,?) ON CONFLICT(%s,connection_id) DO UPDATE SET can_launch=1,credential_id=excluded.credential_id`, table, col, col), q.SubjectID, q.ConnectionID, q.CredentialID)
			}
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), fmt.Sprintf(`DELETE FROM %s WHERE %s=? AND connection_id=?`, table, col), q.SubjectID, q.ConnectionID)
		}
		id = q.ConnectionID
	case "memberships":
		var q struct {
			UserID  int64 `json:"user_id"`
			GroupID int64 `json:"group_id"`
			Member  bool  `json:"member"`
		}
		if decode(r, &q) != nil {
			s.error(w, r, 400, "INVALID_REQUEST", "Invalid membership.")
			return
		}
		if q.Member {
			_, err = s.Store.DB.ExecContext(r.Context(), `INSERT INTO user_groups(user_id,group_id) VALUES(?,?) ON CONFLICT DO NOTHING`, q.UserID, q.GroupID)
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `DELETE FROM user_groups WHERE user_id=? AND group_id=?`, q.UserID, q.GroupID)
		}
	default:
		s.error(w, r, 404, "NOT_FOUND", "Resource not found.")
		return
	}
	if err != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "The resource could not be created.")
		return
	}
	s.Store.AuditAdmin(r.Context(), sess.User.ID, remoteIP(r), resource, id)
	writeJSON(w, 201, map[string]int64{"id": id})
}

func (s *Server) adminUpdate(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	resource := r.PathValue("resource")
	revokedDevice := false
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid identifier.")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	switch resource {
	case "users":
		var q struct {
			DisplayName string `json:"display_name"`
			IsAdmin     bool   `json:"is_admin"`
			Enabled     bool   `json:"enabled"`
			Password    string `json:"password"`
		}
		if decode(r, &q) != nil || !validLabel(q.DisplayName, 128) {
			err = errors.New("invalid")
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE users SET display_name=?,is_admin=?,enabled=?,updated_at=? WHERE id=?`, q.DisplayName, q.IsAdmin, q.Enabled, now, id)
			if err == nil && q.Password != "" {
				var h string
				h, err = security.HashPassword(q.Password)
				if err == nil {
					_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE users SET password_hash=?,updated_at=? WHERE id=?`, h, now, id)
				}
			}
		}
	case "groups":
		var q struct{ Name, Description string }
		if decode(r, &q) != nil || !validLabel(q.Name, 128) || len(q.Description) > 1024 {
			err = errors.New("invalid")
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE groups SET name=?,description=?,updated_at=? WHERE id=?`, q.Name, q.Description, now, id)
		}
	case "connections":
		var q struct {
			Name           string          `json:"name"`
			Description    string          `json:"description"`
			Protocol       string          `json:"protocol"`
			Host           string          `json:"host"`
			Icon           string          `json:"icon"`
			Port           int             `json:"port"`
			SortOrder      int             `json:"sort_order"`
			Enabled        bool            `json:"enabled"`
			ProtocolConfig json.RawMessage `json:"protocol_config"`
			CredentialID   *int64          `json:"credential_id"`
		}
		if decode(r, &q) != nil || !validLabel(q.Name, 128) || len(q.Description) > 1024 || len(q.Icon) > 128 || !s.validProtocol(q.Protocol) || !validHost(q.Host) || q.Port < 1 || q.Port > 65535 || !validProtocolConfig(q.Protocol, q.ProtocolConfig) || !s.credentialAllowedForProtocol(r.Context(), q.CredentialID, q.Protocol) {
			err = errors.New("invalid")
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE connections SET name=?,description=?,protocol=?,host=?,port=?,enabled=?,icon=?,sort_order=?,protocol_config_json=?,credential_id=?,updated_at=? WHERE id=?`, q.Name, q.Description, q.Protocol, q.Host, q.Port, q.Enabled, q.Icon, q.SortOrder, string(q.ProtocolConfig), q.CredentialID, now, id)
		}
	case "credentials":
		var q struct {
			Username string `json:"username"`
			Secret   string `json:"secret"`
		}
		if decode(r, &q) != nil || q.Secret == "" {
			err = errors.New("invalid")
		} else {
			err = s.Store.ReplaceCredential(r.Context(), id, q.Username, q.Secret)
		}
	case "devices":
		var q struct {
			Name    *string `json:"name"`
			Enabled *bool   `json:"enabled"`
		}
		if decode(r, &q) != nil || (q.Name != nil && !validLabel(*q.Name, 128)) {
			err = errors.New("invalid")
		} else {
			if q.Name != nil {
				_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE devices SET name=?,updated_at=? WHERE id=?`, *q.Name, now, id)
			}
			if err == nil && q.Enabled != nil {
				_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE devices SET enabled=?,updated_at=? WHERE id=?`, *q.Enabled, now, id)
				revokedDevice = !*q.Enabled
			}
		}
	case "policies":
		var q app.AccessPolicy
		if decode(r, &q) != nil || q.UserID != id || q.AllowedDaysMask < 0 || q.AllowedDaysMask > 127 || q.AccessStartMinute < 0 || q.AccessStartMinute > 1439 || q.AccessEndMinute < 1 || q.AccessEndMinute > 1440 || q.DailyLimitMinutes < 0 || q.DailyLimitMinutes > 1440 || q.MaxSessionMinutes < 0 || q.MaxSessionMinutes > 720 || q.IdleLogoutMinutes < 1 || q.IdleLogoutMinutes > 1440 {
			err = errors.New("invalid")
		} else if _, zoneErr := time.LoadLocation(q.Timezone); zoneErr != nil {
			err = errors.New("invalid timezone")
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `INSERT INTO user_policies(user_id,timezone,allowed_days_mask,access_start_minute,access_end_minute,daily_limit_minutes,max_session_minutes,idle_logout_minutes,updated_at) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET timezone=excluded.timezone,allowed_days_mask=excluded.allowed_days_mask,access_start_minute=excluded.access_start_minute,access_end_minute=excluded.access_end_minute,daily_limit_minutes=excluded.daily_limit_minutes,max_session_minutes=excluded.max_session_minutes,idle_logout_minutes=excluded.idle_logout_minutes,updated_at=excluded.updated_at`, id, q.Timezone, q.AllowedDaysMask, q.AccessStartMinute, q.AccessEndMinute, q.DailyLimitMinutes, q.MaxSessionMinutes, q.IdleLogoutMinutes, now)
			if err == nil {
				_, _ = s.Store.DB.ExecContext(r.Context(), `DELETE FROM launch_tickets WHERE user_id=? AND redeemed_at IS NULL`, id)
			}
		}
	case "settings":
		var q struct {
			ScreenSleepMinutes int    `json:"screen_sleep_minutes"`
			ShowUserList       bool   `json:"show_user_list"`
			TerminalTheme      string `json:"terminal_theme"`
			ClientTheme        string `json:"client_theme"`
		}
		validClientThemes := map[string]bool{"ocean": true, "graphite": true, "forest": true, "sunset": true, "high-contrast": true}
		if decode(r, &q) != nil || id != 1 || q.ScreenSleepMinutes < 0 || q.ScreenSleepMinutes > 1440 || (q.TerminalTheme != "dark" && q.TerminalTheme != "light") || !validClientThemes[q.ClientTheme] {
			err = errors.New("invalid")
		} else {
			_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE kiosk_settings SET screen_sleep_minutes=?,show_user_list=?,terminal_theme=?,client_theme=?,updated_at=? WHERE id=1`, q.ScreenSleepMinutes, q.ShowUserList, q.TerminalTheme, q.ClientTheme, now)
		}
	case "profile-photos":
		var q struct {
			DataURL string `json:"data_url"`
		}
		if decode(r, &q) != nil {
			err = errors.New("invalid")
		} else if q.DataURL == "" {
			_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE users SET profile_photo=NULL,profile_photo_mime=NULL,updated_at=? WHERE id=?`, now, id)
		} else {
			mime, photo, photoErr := decodeProfilePhoto(q.DataURL)
			if photoErr != nil {
				err = photoErr
			} else {
				_, err = s.Store.DB.ExecContext(r.Context(), `UPDATE users SET profile_photo=?,profile_photo_mime=?,updated_at=? WHERE id=?`, photo, mime, now, id)
			}
		}
	default:
		s.error(w, r, 404, "NOT_FOUND", "Resource not found.")
		return
	}
	if err != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "The resource could not be updated.")
		return
	}
	if revokedDevice {
		s.Store.AuditDeviceRevoked(r.Context(), sess.User.ID, id, remoteIP(r))
	}
	s.Store.AuditAdmin(r.Context(), sess.User.ID, remoteIP(r), resource, id)
	w.WriteHeader(204)
}

func decodeProfilePhoto(dataURL string) (string, []byte, error) {
	header, encoded, ok := strings.Cut(dataURL, ",")
	if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
		return "", nil, errors.New("invalid profile photo")
	}
	photo, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(photo) == 0 || len(photo) > 256*1024 {
		return "", nil, errors.New("invalid profile photo")
	}
	mime := http.DetectContentType(photo)
	if mime != "image/png" && mime != "image/jpeg" && mime != "image/webp" {
		return "", nil, errors.New("invalid profile photo")
	}
	return mime, photo, nil
}
func (s *Server) adminDelete(w http.ResponseWriter, r *http.Request) {
	sess := r.Context().Value(sessionKey).(app.Session)
	resource := r.PathValue("resource")
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.error(w, r, 400, "INVALID_REQUEST", "Invalid identifier.")
		return
	}
	tables := map[string]string{"groups": "groups", "connections": "connections", "credentials": "credentials"}
	table, ok := tables[resource]
	if !ok {
		s.error(w, r, 405, "METHOD_NOT_ALLOWED", "This resource cannot be deleted.")
		return
	}
	_, err = s.Store.DB.ExecContext(r.Context(), fmt.Sprintf("DELETE FROM %s WHERE id=?", table), id)
	if err != nil {
		s.error(w, r, 409, "RESOURCE_IN_USE", "The resource is still in use.")
		return
	}
	s.Store.AuditAdmin(r.Context(), sess.User.ID, remoteIP(r), resource, id)
	w.WriteHeader(204)
}
func (s *Server) validProtocol(p string) bool {
	return p == "rdp" || p == "moonlight" || p == "vnc" || p == "ssh" || s.Dev && p == "mock"
}

func (s *Server) credentialAllowedForProtocol(ctx context.Context, credentialID *int64, protocol string) bool {
	if credentialID == nil {
		return true
	}
	var kind string
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT secret_type FROM credentials WHERE id=?`, *credentialID).Scan(&kind); err != nil {
		return false
	}
	return kind != "ssh_private_key" || protocol == "ssh"
}

func validProtocolConfig(protocol string, raw json.RawMessage) bool {
	if !json.Valid(raw) {
		return false
	}
	if protocol != "ssh" {
		return true
	}
	var cfg struct {
		TerminalTitle string `json:"terminal_title,omitempty"`
	}
	if json.Unmarshal(raw, &cfg) != nil || len(cfg.TerminalTitle) > 128 || strings.ContainsAny(cfg.TerminalTitle, "\r\n\x00") {
		return false
	}
	return true
}
func validLabel(v string, max int) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= max && !strings.ContainsAny(v, "\r\n\x00")
}
func validHost(h string) bool {
	h = strings.TrimSpace(h)
	if h == "" || len(h) > 253 || !asciiAlphaNum(h[0]) {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !asciiAlphaNum(c) && c != '.' && c != '_' && c != '-' && c != ':' {
			return false
		}
	}
	return true
}
func asciiAlphaNum(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
