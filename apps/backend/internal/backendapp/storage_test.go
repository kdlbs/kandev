package backendapp

import (
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func TestRecordSchemaVersionRecordsForPostgres(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kandev.db")
	raw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	writer := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })

	if _, err := writer.Exec(`
		CREATE TABLE kandev_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}

	recordSchemaVersion(writer, "postgres", "v9.9.9", nil)

	var version string
	if err := writer.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, "kandev_version").Scan(&version); err != nil {
		t.Fatalf("read kandev_version: %v", err)
	}
	if version != "v9.9.9" {
		t.Fatalf("kandev_version = %q, want v9.9.9", version)
	}
}
