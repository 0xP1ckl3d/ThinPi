package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthcheckUsesLoopbackHealthEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	if err := healthcheck([]string{"--url", server.URL + "/healthz"}); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckRejectsRemoteOrWrongPath(t *testing.T) {
	for _, target := range []string{"https://example.com/healthz", "http://127.0.0.1/admin"} {
		if err := healthcheck([]string{"--url", target}); err == nil || !strings.Contains(err.Error(), "healthcheck") {
			t.Fatalf("target %q returned %v", target, err)
		}
	}
}
