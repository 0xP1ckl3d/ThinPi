package launch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSSHKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIE4N4OjnVgCJ0eHqCY3YQBMJm1r+4BjJvYX0S2Ctmock"
const changedSSHKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIF4N4OjnVgCJ0eHqCY3YQBMJm1r+4BjJvYX0S2Ctmore"

func TestSSHTrustStoreAcceptsFirstUseAndRequiresChangedKeyApproval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known-hosts")
	keys := []string{testSSHKey}
	store := NewSSHTrustStore(path)
	store.Scan = func(context.Context, string, int) ([]string, error) { return keys, nil }

	if pending, err := store.Prepare(context.Background(), "server.example", 22); err != nil || pending != nil {
		t.Fatalf("first use was not accepted: pending=%#v err=%v", pending, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "server.example "+testSSHKey) {
		t.Fatalf("first key was not persisted: %q %v", data, err)
	}
	if pending, err := store.Prepare(context.Background(), "server.example", 22); err != nil || pending != nil {
		t.Fatalf("unchanged key was rejected: pending=%#v err=%v", pending, err)
	}

	keys = []string{changedSSHKey}
	pending, err := store.Prepare(context.Background(), "server.example", 22)
	var changed *SSHHostKeyChangedError
	if !errors.As(err, &changed) || pending == nil || !strings.HasPrefix(changed.Fingerprint, "SHA256:") {
		t.Fatalf("changed key did not require confirmation: pending=%#v err=%v", pending, err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), changedSSHKey) {
		t.Fatal("changed key was trusted before approval")
	}
	if err := store.Accept(pending); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if !strings.Contains(string(data), changedSSHKey) || strings.Contains(string(data), testSSHKey) {
		t.Fatalf("approved key did not replace old key: %q", data)
	}
}
