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
	var sleepMinutes int
	var showUserList bool
	var terminalTheme, clientTheme string
	if err := db.QueryRow(`SELECT screen_sleep_minutes,show_user_list,terminal_theme,client_theme FROM kiosk_settings WHERE id=1`).Scan(&sleepMinutes, &showUserList, &terminalTheme, &clientTheme); err != nil {
		t.Fatal(err)
	}
	if sleepMinutes != 15 || !showUserList || terminalTheme != "dark" || clientTheme != "ocean" {
		t.Fatalf("kiosk defaults = %d,%v,%q,%q", sleepMinutes, showUserList, terminalTheme, clientTheme)
	}
}
