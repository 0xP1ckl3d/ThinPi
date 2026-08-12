package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"thinpi.local/controller/migrations"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	dsn := path
	if path == ":memory:" {
		dsn = "file:thinpi-test?mode=memory&cache=shared"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, err
	}
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if _, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
			db.Close()
			return nil, fmt.Errorf("enable SQLite WAL: %w", err)
		}
	}
	return db, nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix := strings.SplitN(entry.Name(), "_", 2)[0]
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		var exists int
		err = db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version=?`, version).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		body, err := migrations.FS.ReadFile(entry.Name())
		if err != nil {
			return err
		}
		// Schema-rebuild migrations need foreign-key enforcement paused before
		// the transaction begins. It is restored and verified immediately after.
		if _, err = db.ExecContext(ctx, `PRAGMA foreign_keys=OFF`); err != nil {
			return err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			_, _ = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
			return err
		}
		if _, err = tx.ExecContext(ctx, string(body)); err != nil {
			tx.Rollback()
			_, _ = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?,?)`, version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			tx.Rollback()
			_, _ = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
			return err
		}
		if err = tx.Commit(); err != nil {
			_, _ = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`)
			return err
		}
		if _, err = db.ExecContext(ctx, `PRAGMA foreign_keys=ON`); err != nil {
			return err
		}
		var broken string
		if err = db.QueryRowContext(ctx, `SELECT 'broken' FROM pragma_foreign_key_check LIMIT 1`).Scan(&broken); err == nil {
			return fmt.Errorf("migration %s left invalid foreign keys", entry.Name())
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}
