package database

import (
	"path/filepath"
	"testing"
)

func TestFileDatabaseMigratesAndUsesWAL(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "thinpi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='launch_tickets'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 1 {
		t.Fatal("migration did not create launch_tickets")
	}
}
