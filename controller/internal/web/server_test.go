package web

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"thinpi.local/controller/internal/app"
	"thinpi.local/controller/internal/database"
	"thinpi.local/controller/internal/security"
)

func testHTTP(t *testing.T) (*httptest.Server, *app.Store) {
	t.Helper()
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, _ := security.NewVault(security.DeterministicDevKey())
	store := app.NewStore(db, vault, 30*time.Minute, 30*time.Second)
	if err = store.SeedDev(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), true, "test").Handler)
	t.Cleanup(server.Close)
	return server, store
}
func request(t *testing.T, client *http.Client, method, url, token string, body any) (int, map[string]any) {
	t.Helper()
	var b io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		b = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, url, b)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out := map[string]any{}
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}
func TestFullLoginListLaunchRedeemSequence(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	status, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter", "password": "thinpi-dev"})
	if status != 200 {
		t.Fatalf("login %d %#v", status, login)
	}
	token := login["token"].(string)
	status, list := request(t, client, "GET", server.URL+"/api/v1/connections", token, nil)
	if status != 200 {
		t.Fatal(status)
	}
	items := list["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("items=%d", len(items))
	}
	id := int64(items[0].(map[string]any)["id"].(float64))
	status, launch := request(t, client, "POST", server.URL+"/api/v1/connections/"+strconv.FormatInt(id, 10)+"/launch", token, map[string]string{"device_identifier": "dev-device"})
	if status != 201 {
		t.Fatalf("launch %d %#v", status, launch)
	}
	ticket := launch["ticket"].(string)
	status, redeemed := request(t, client, "POST", server.URL+"/api/v1/agent/redeem-launch", "dev-device-token", map[string]string{"ticket": ticket})
	if status != 200 {
		t.Fatalf("redeem %d %#v", status, redeemed)
	}
	manifest := redeemed["manifest"].(map[string]any)
	if manifest["username"] != "student.daughter" || manifest["password"] != "school-demo-password" {
		t.Fatal("assigned credential was not resolved for the device agent")
	}
	status, _ = request(t, client, "POST", server.URL+"/api/v1/agent/redeem-launch", "dev-device-token", map[string]string{"ticket": ticket})
	if status != 403 {
		t.Fatalf("reuse status=%d", status)
	}
}
func TestAdminAndUserRoleEnforced(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	_, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter", "password": "thinpi-dev"})
	status, out := request(t, client, "GET", server.URL+"/api/v1/admin/users", login["token"].(string), nil)
	if status != 403 {
		t.Fatalf("status=%d %#v", status, out)
	}
}

func TestAdminKioskSettingsReachLoginBootstrap(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()

	status, bootstrap := request(t, client, "GET", server.URL+"/api/v1/login-users", "", nil)
	if status != http.StatusOK {
		t.Fatalf("bootstrap status=%d %#v", status, bootstrap)
	}
	configuration := bootstrap["configuration"].(map[string]any)
	if configuration["screen_sleep_minutes"] != float64(15) {
		t.Fatalf("default screen sleep=%v", configuration["screen_sleep_minutes"])
	}

	_, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	adminToken := login["token"].(string)
	status, output := request(t, client, "PUT", server.URL+"/api/v1/admin/settings/1", adminToken, map[string]any{"screen_sleep_minutes": 7, "show_user_list": false, "terminal_theme": "light", "client_theme": "forest"})
	if status != http.StatusNoContent {
		t.Fatalf("settings update status=%d %#v", status, output)
	}

	status, bootstrap = request(t, client, "GET", server.URL+"/api/v1/login-users", "", nil)
	configuration = bootstrap["configuration"].(map[string]any)
	if status != http.StatusOK || configuration["screen_sleep_minutes"] != float64(7) || configuration["terminal_theme"] != "light" || configuration["client_theme"] != "forest" {
		t.Fatalf("updated bootstrap status=%d %#v", status, bootstrap)
	}
	if items := bootstrap["items"].([]any); len(items) != 0 || bootstrap["has_more"] != true {
		t.Fatalf("hidden user list was exposed: %#v", bootstrap)
	}

	status, _ = request(t, client, "PUT", server.URL+"/api/v1/admin/settings/1", adminToken, map[string]any{"screen_sleep_minutes": 1441, "show_user_list": true, "terminal_theme": "dark", "client_theme": "ocean"})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid screen sleep accepted: %d", status)
	}
}

func TestAdminProfilePhotoAppearsOnKioskLogin(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	_, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	adminToken := login["token"].(string)
	adminID := int64(login["user"].(map[string]any)["id"].(float64))
	photo := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Zl9sAAAAASUVORK5CYII="
	status, output := request(t, client, "PUT", server.URL+"/api/v1/admin/profile-photos/"+strconv.FormatInt(adminID, 10), adminToken, map[string]string{"data_url": photo})
	if status != http.StatusNoContent {
		t.Fatalf("photo upload status=%d %#v", status, output)
	}

	status, bootstrap := request(t, client, "GET", server.URL+"/api/v1/login-users", "", nil)
	if status != http.StatusOK {
		t.Fatal(status)
	}
	photoURL := ""
	for _, item := range bootstrap["items"].([]any) {
		user := item.(map[string]any)
		if user["username"] == "admin" {
			photoURL, _ = user["profile_photo_url"].(string)
		}
	}
	if photoURL == "" {
		t.Fatalf("profile photo URL absent: %#v", bootstrap)
	}
	response, err := client.Get(server.URL + photoURL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("photo response=%d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
}

func TestSSHIsAProductionProtocolAndMockIsNot(t *testing.T) {
	s := &Server{Dev: false}
	if !s.validProtocol("ssh") {
		t.Fatal("production SSH protocol was rejected")
	}
	if s.validProtocol("mock") {
		t.Fatal("production mock protocol was accepted")
	}
}

func TestLocalMaintenanceRequiresAdminAndSingleUseDeviceTicket(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	_, userLogin := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter", "password": "thinpi-dev"})
	status, _ := request(t, client, "POST", server.URL+"/api/v1/maintenance", userLogin["token"].(string), map[string]string{"device_identifier": "dev-device"})
	if status != http.StatusForbidden {
		t.Fatalf("non-admin maintenance status=%d", status)
	}
	_, adminLogin := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	status, created := request(t, client, "POST", server.URL+"/api/v1/maintenance", adminLogin["token"].(string), map[string]string{"device_identifier": "dev-device"})
	if status != http.StatusCreated || created["ticket"] == nil {
		t.Fatalf("admin maintenance status=%d %#v", status, created)
	}
	ticket := created["ticket"].(string)
	status, _ = request(t, client, "POST", server.URL+"/api/v1/agent/redeem-maintenance", "dev-device-token", map[string]string{"ticket": ticket})
	if status != http.StatusNoContent {
		t.Fatalf("maintenance redemption status=%d", status)
	}
	status, _ = request(t, client, "POST", server.URL+"/api/v1/agent/redeem-maintenance", "dev-device-token", map[string]string{"ticket": ticket})
	if status != http.StatusForbidden {
		t.Fatalf("maintenance ticket reuse status=%d", status)
	}
}

func TestBrowserNavigationRedirectsNaturally(t *testing.T) {
	server, store := testHTTP(t)
	client := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	assertRedirect := func(path, want string, cookie *http.Cookie) *http.Response {
		t.Helper()
		req, _ := http.NewRequest("GET", server.URL+path, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != want {
			res.Body.Close()
			t.Fatalf("GET %s: status=%d location=%q, want %q", path, res.StatusCode, res.Header.Get("Location"), want)
		}
		return res
	}

	assertRedirect("/", "/admin/login", nil).Body.Close()
	assertRedirect("/admin", "/admin/login", nil).Body.Close()
	assertRedirect("/admin/", "/admin", nil).Body.Close()

	loginBody := bytes.NewBufferString(`{"username":"admin","password":"thinpi-dev"}`)
	loginRequest, _ := http.NewRequest("POST", server.URL+"/api/v1/auth/login", loginBody)
	loginRequest.Header.Set("Content-Type", "application/json")
	loginResponse, err := client.Do(loginRequest)
	if err != nil {
		t.Fatal(err)
	}
	var login map[string]any
	_ = json.NewDecoder(loginResponse.Body).Decode(&login)
	loginResponse.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range loginResponse.Cookies() {
		if cookie.Name == "thinpi_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("login did not issue a browser cookie")
	}
	assertRedirect("/", "/admin", sessionCookie).Body.Close()
	assertRedirect("/admin/login", "/admin", sessionCookie).Body.Close()

	adminRequest, _ := http.NewRequest("GET", server.URL+"/admin", nil)
	adminRequest.AddCookie(sessionCookie)
	adminResponse, err := client.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	if adminResponse.StatusCode != http.StatusOK {
		t.Fatalf("authenticated /admin status=%d", adminResponse.StatusCode)
	}
	adminResponse.Body.Close()

	if _, err := store.DB.Exec(`UPDATE sessions SET revoked_at=?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	expiredResponse := assertRedirect("/admin", "/admin/login", sessionCookie)
	defer expiredResponse.Body.Close()
	cleared := false
	for _, cookie := range expiredResponse.Cookies() {
		cleared = cleared || cookie.Name == "thinpi_session" && cookie.MaxAge < 0
	}
	if !cleared {
		t.Fatal("expired browser session cookie was not cleared")
	}

	status, output := request(t, client, "GET", server.URL+"/api/v1/admin/users", login["token"].(string), nil)
	if status != http.StatusUnauthorized || output["error"] == nil {
		t.Fatalf("expired API session did not retain JSON 401: %d %#v", status, output)
	}
}

func TestLauncherAdministratorHandoffIsSingleUse(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	_, adminLogin := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	adminToken := adminLogin["token"].(string)
	status, handoff := request(t, client, "POST", server.URL+"/api/v1/admin-handoff", adminToken, map[string]any{})
	if status != http.StatusCreated {
		t.Fatalf("handoff status=%d %#v", status, handoff)
	}
	path := handoff["path"].(string)
	noRedirect := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedirect.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/admin" {
		t.Fatalf("redemption status=%d location=%q", res.StatusCode, res.Header.Get("Location"))
	}
	var sessionCookie *http.Cookie
	for _, cookie := range res.Cookies() {
		if cookie.Name == "thinpi_session" {
			sessionCookie = cookie
		}
	}
	res.Body.Close()
	if sessionCookie == nil || sessionCookie.Value == "" {
		t.Fatal("handoff did not issue an administrator browser session")
	}
	adminRequest, _ := http.NewRequest("GET", server.URL+"/admin", nil)
	adminRequest.AddCookie(sessionCookie)
	adminResponse, err := client.Do(adminRequest)
	if err != nil {
		t.Fatal(err)
	}
	if adminResponse.StatusCode != http.StatusOK {
		t.Fatalf("admin status=%d", adminResponse.StatusCode)
	}
	adminResponse.Body.Close()
	replay, err := noRedirect.Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer replay.Body.Close()
	if replay.StatusCode != http.StatusSeeOther || replay.Header.Get("Location") != "/admin/login" {
		t.Fatalf("handoff replay was accepted: %d %q", replay.StatusCode, replay.Header.Get("Location"))
	}

	_, userLogin := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter", "password": "thinpi-dev"})
	status, _ = request(t, client, "POST", server.URL+"/api/v1/admin-handoff", userLogin["token"].(string), map[string]any{})
	if status != http.StatusForbidden {
		t.Fatalf("non-admin handoff status=%d", status)
	}
}

func TestProductionAdminPageContainsNoDevelopmentFixtures(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault, _ := security.NewVault(security.DeterministicDevKey())
	store := app.NewStore(db, vault, 30*time.Minute, 30*time.Second)
	if err := store.SeedDev(context.Background()); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), false, "test").Handler)
	defer server.Close()
	_, login := request(t, server.Client(), "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	req, _ := http.NewRequest("GET", server.URL+"/admin", nil)
	req.Header.Set("Authorization", "Bearer "+login["token"].(string))
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	for _, forbidden := range []string{"Development", "Local test mode", "Test the experience", "daughter / thinpi-dev", "Demo session"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("production page exposes development fixture %q", forbidden)
		}
	}
	if !strings.Contains(string(body), "Linux command line (locked SSH)") || !strings.Contains(string(body), "trusted on first use") {
		t.Fatal("production admin page does not expose the managed SSH workflow")
	}
	status, output := request(t, server.Client(), "POST", server.URL+"/api/v1/admin/connections", login["token"].(string), map[string]any{"name": "Injected demo", "description": "", "protocol": "mock", "host": "mock", "port": 1, "enabled": true, "icon": "", "sort_order": 0, "protocol_config": map[string]any{}})
	if status != http.StatusBadRequest {
		t.Fatalf("production accepted mock connection: %d %#v", status, output)
	}
	status, output = request(t, server.Client(), "POST", server.URL+"/api/v1/admin/connections", login["token"].(string), map[string]any{"name": "Managed SSH", "description": "remote shell only", "protocol": "ssh", "host": "ssh.example", "port": 22, "enabled": true, "icon": "", "sort_order": 0, "protocol_config": map[string]any{"terminal_title": "Secure shell"}})
	if status != http.StatusCreated {
		t.Fatalf("production rejected SSH without an administrator-supplied host key: %d %#v", status, output)
	}
}

func TestProductionDoesNotTrustForwardedProtoFromClient(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	vault, _ := security.NewVault(security.DeterministicDevKey())
	store := app.NewStore(db, vault, 30*time.Minute, 30*time.Second)
	server := httptest.NewServer(New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), false, "test").Handler)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/healthz", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	res, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUpgradeRequired {
		t.Fatalf("spoofed forwarded proto bypassed TLS requirement: %d", res.StatusCode)
	}
}

func TestAdministratorCRUD(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	_, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "admin", "password": "thinpi-dev"})
	token := login["token"].(string)
	status, user := request(t, client, "POST", server.URL+"/api/v1/admin/users", token, map[string]any{"username": "new-user", "display_name": "New User", "password": "password1", "is_admin": false, "enabled": true})
	if status != 201 {
		t.Fatalf("user %d %#v", status, user)
	}
	status, group := request(t, client, "POST", server.URL+"/api/v1/admin/groups", token, map[string]any{"name": "New Group", "description": "test"})
	if status != 201 {
		t.Fatalf("group %d %#v", status, group)
	}
	status, credential := request(t, client, "POST", server.URL+"/api/v1/admin/credentials", token, map[string]any{"name": "Remote", "username": "remote", "secret": "remote-password", "secret_type": "password"})
	if status != 201 {
		t.Fatalf("credential %d %#v", status, credential)
	}
	credentialID := int64(credential["id"].(float64))
	status, connection := request(t, client, "POST", server.URL+"/api/v1/admin/connections", token, map[string]any{"name": "New PC", "description": "test", "protocol": "rdp", "host": "new-pc.internal", "port": 3389, "enabled": true, "icon": "", "sort_order": 9, "protocol_config": map[string]any{"fullscreen": true}, "credential_id": credentialID})
	if status != 201 {
		t.Fatalf("connection %d %#v", status, connection)
	}
	connectionID := int64(connection["id"].(float64))
	status, listedConnections := request(t, client, "GET", server.URL+"/api/v1/admin/connections", token, nil)
	if status != 200 {
		t.Fatalf("connection list status=%d", status)
	}
	items := listedConnections["items"].([]any)
	var savedCredentialID int64
	for _, item := range items {
		entry := item.(map[string]any)
		if int64(entry["id"].(float64)) == connectionID && entry["credential_id"] != nil {
			savedCredentialID = int64(entry["credential_id"].(float64))
		}
	}
	if savedCredentialID != credentialID {
		t.Fatalf("connection credential=%d, want %d", savedCredentialID, credentialID)
	}
	status, _ = request(t, client, "POST", server.URL+"/api/v1/admin/permissions", token, map[string]any{"subject_type": "user", "subject_id": int64(user["id"].(float64)), "connection_id": connectionID, "can_launch": true})
	if status != 201 {
		t.Fatalf("permission status=%d", status)
	}
	status, _ = request(t, client, "POST", server.URL+"/api/v1/admin/memberships", token, map[string]any{"user_id": int64(user["id"].(float64)), "group_id": int64(group["id"].(float64)), "member": true})
	if status != 201 {
		t.Fatalf("membership status=%d", status)
	}
	status, credentials := request(t, client, "GET", server.URL+"/api/v1/admin/credentials", token, nil)
	if status != 200 {
		t.Fatal(status)
	}
	raw, _ := json.Marshal(credentials)
	if bytes.Contains(raw, []byte("remote-password")) || bytes.Contains(raw, []byte("encrypted_secret")) {
		t.Fatal("credential secret leaked from admin list")
	}
	status, _ = request(t, client, "PUT", server.URL+"/api/v1/admin/credentials/"+strconv.FormatInt(credentialID, 10), token, map[string]string{"username": "remote2", "secret": "replacement"})
	if status != 204 {
		t.Fatalf("credential update status=%d", status)
	}
	status, enrol := request(t, client, "POST", server.URL+"/api/v1/admin/enrolment-tokens", token, map[string]any{"name": "Test Pi", "ttl_minutes": 15})
	if status != 201 {
		t.Fatalf("enrolment token %d %#v", status, enrol)
	}
	status, device := request(t, client, "POST", server.URL+"/api/v1/devices/enrol", "", map[string]string{"token": enrol["token"].(string), "device_identifier": "admin-crud-pi", "name": "Test Pi"})
	if status != 201 {
		t.Fatalf("enrol device %d %#v", status, device)
	}
	deviceID := int64(device["device"].(map[string]any)["id"].(float64))
	status, _ = request(t, client, "PUT", server.URL+"/api/v1/admin/devices/"+strconv.FormatInt(deviceID, 10), token, map[string]any{"enabled": false})
	if status != 204 {
		t.Fatalf("revoke status=%d", status)
	}
}

func TestLoginChoicesAndSelfServiceProfile(t *testing.T) {
	server, _ := testHTTP(t)
	client := server.Client()
	status, choices := request(t, client, "GET", server.URL+"/api/v1/login-users", "", nil)
	if status != http.StatusOK || len(choices["items"].([]any)) == 0 {
		t.Fatalf("login choices status=%d %#v", status, choices)
	}
	_, login := request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter", "password": "thinpi-dev"})
	token := login["token"].(string)
	status, profile := request(t, client, "PUT", server.URL+"/api/v1/me", token, map[string]string{"username": "daughter-2", "display_name": "Daughter Two", "current_password": "thinpi-dev", "new_password": "new-password"})
	if status != http.StatusOK {
		t.Fatalf("profile update status=%d %#v", status, profile)
	}
	status, _ = request(t, client, "POST", server.URL+"/api/v1/auth/login", "", map[string]string{"username": "daughter-2", "password": "new-password"})
	if status != http.StatusOK {
		t.Fatalf("updated profile credentials were rejected: %d", status)
	}
}

func TestAdminJavaScriptDoesNotPassMapIndexToFetch(t *testing.T) {
	server, _ := testHTTP(t)
	res, err := server.Client().Get(server.URL + "/static/admin-v2.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte(".map(api)")) {
		t.Fatal("admin refresh passes Array.map index to fetch options")
	}
	if !bytes.Contains(body, []byte("paths.map(path => api(path))")) {
		t.Fatal("admin refresh does not isolate API paths from Array.map arguments")
	}
	if bytes.Contains(body, []byte("prompt(")) || bytes.Contains(body, []byte("confirm(")) {
		t.Fatal("admin actions use unsupported native prompt dialogs")
	}
	if !bytes.Contains(body, []byte("window.location.replace('/admin/login')")) {
		t.Fatal("expired admin API sessions do not redirect the browser to login")
	}
	for _, required := range []string{"ssh:{port:22", "ssh_private_key", "assign-connection", "assign-user", "remove-assignment", "certificate_mode:'tofu'", "return-client"} {
		if !bytes.Contains(body, []byte(required)) {
			t.Fatalf("admin workflow is missing %q", required)
		}
	}
	if bytes.Contains(body, []byte("ssh_host_key")) {
		t.Fatal("admin workflow still requests an SSH host public key")
	}
	if bytes.Contains(body, []byte("REPLACE_WITH_VERIFIED_SERVER_PUBLIC_KEY")) {
		t.Fatal("admin SSH editor still ships an invalid placeholder host key")
	}
}
