package launch

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

func TestSubmitSunshinePINUsesAuthenticatedAPI(t *testing.T) {
	var received map[string]string
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "sunshine-admin" || password != "sunshine-secret" {
			t.Errorf("unexpected API credential: %q %q %t", username, password, ok)
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		if request.URL.Path != "/api/pin" || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected PIN request: %s %s", request.URL.Path, request.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		_, _ = response.Write([]byte(`{"status":true}`))
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	pairing := MoonlightPairing{Host: host, SunshineAPIPort: port, Username: "sunshine-admin", Password: "sunshine-secret", ClientName: "Living Room ThinPi"}
	client := newSunshinePairingClient(filepath.Join(t.TempDir(), "sunshine-certificates.json"), parsed.Host)
	accepted, permanent, err := submitSunshinePIN(context.Background(), client, pairing, "0042")
	if err != nil || !accepted || permanent {
		t.Fatalf("PIN submission failed: accepted=%t permanent=%t err=%v", accepted, permanent, err)
	}
	if received["pin"] != "0042" || received["name"] != "Living Room ThinPi" {
		t.Fatalf("unexpected PIN payload: %#v", received)
	}
}

func TestSunshineCertificateTrustIsTOFU(t *testing.T) {
	store := &sunshineCertificateStore{path: filepath.Join(t.TempDir(), "sunshine-certificates.json")}
	if err := store.verify("sunshine.local:47990", []byte("first certificate")); err != nil {
		t.Fatal(err)
	}
	if err := store.verify("sunshine.local:47990", []byte("first certificate")); err != nil {
		t.Fatalf("persisted certificate was not trusted: %v", err)
	}
	if err := store.verify("sunshine.local:47990", []byte("changed certificate")); err == nil {
		t.Fatal("changed Sunshine certificate was accepted")
	}
}

func TestRandomMoonlightPINIsFourDigits(t *testing.T) {
	pin, err := randomMoonlightPIN()
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9]{4}$`).MatchString(pin) {
		t.Fatalf("invalid generated PIN %q", pin)
	}
}
