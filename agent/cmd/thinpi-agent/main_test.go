package main

import "testing"

func TestMockClientsAreRestrictedToLoopbackDevelopmentControllers(t *testing.T) {
	for _, raw := range []string{"http://127.0.0.1:8080", "http://[::1]:8080", "http://localhost:8080"} {
		if !localDevelopmentController(raw) {
			t.Fatalf("development controller rejected: %s", raw)
		}
	}
	for _, raw := range []string{"https://thinpi.example.internal", "http://10.0.0.5:8080", "https://localhost:8443", "not-a-url"} {
		if localDevelopmentController(raw) {
			t.Fatalf("non-development controller accepted: %s", raw)
		}
	}
}
