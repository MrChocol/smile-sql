// Package db opens the SQLite metadata database and runs migrations.
//
// The driver is modernc.org/sqlite (pure Go, no cgo).  WAL mode and
// foreign-key enforcement are enabled on every connection.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"sql-mgr/migrations"
)

// Open creates a new *sql.DB for the SQLite file at path, applies
// connection-level pragmas, and returns it ready for use.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}

	// Verify connectivity.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", err)
	}

	// Connection-level pragmas (executed once per connection by the driver
	// when set via the DSN, but we also set them here for safety).
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("exec %s: %w", p, err)
		}
	}

	return db, nil
}

// Migrate executes the embedded init SQL and then runs incremental
// schema upgrades for existing databases.  All statements use
// IF NOT EXISTS or column-existence checks so they are safe to run
// repeatedly.
func Migrate(db *sql.DB) error {
	if _, err := db.Exec(migrations.InitSQL); err != nil {
		return fmt.Errorf("run init migrations: %w", err)
	}

	// --- Incremental upgrades for existing databases ---

	// Add git_username column to settings table if missing.
	if err := addColumnIfMissing(db, "settings", "git_username", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("upgrade settings.git_username: %w", err)
	}

	// Add git_email column to settings table if missing.
	if err := addColumnIfMissing(db, "settings", "git_email", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("upgrade settings.git_email: %w", err)
	}

	return nil
}

// addColumnIfMissing checks whether a column exists in the given table
// and runs ALTER TABLE ADD COLUMN if it does not.  This is needed
// because SQLite does not support "ADD COLUMN IF NOT EXISTS".
func addColumnIfMissing(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()

	exists := false
	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == column {
			exists = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !exists {
		_, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
		if err != nil {
			return fmt.Errorf("alter table %s add %s: %w", table, column, err)
		}
	}
	return nil
}
