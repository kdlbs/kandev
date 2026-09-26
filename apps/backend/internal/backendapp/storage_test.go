package backendapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/db"
)

func TestRecordSchemaVersionIgnoresDriver(t *testing.T) {
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

	// The driver argument is accepted for compatibility but ignored; even a
	// "postgres" value must not skip recording the schema version.
	recordSchemaVersion(writer, "postgres", "v9.9.9", nil)

	var version string
	if err := writer.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, "kandev_version").Scan(&version); err != nil {
		t.Fatalf("read kandev_version: %v", err)
	}
	if version != "v9.9.9" {
		t.Fatalf("kandev_version = %q, want v9.9.9", version)
	}
}

func TestRecordSchemaVersionAfterPersistenceDoesNotRecordOnLateFailure(t *testing.T) {
	var recorded bool
	lateErr := errors.New("late required store failed")

	err := recordSchemaVersionAfterPersistence(
		func() error { return lateErr },
		func() error { t.Fatal("health check ran after validation failure"); return nil },
		func() { recorded = true },
	)
	if !errors.Is(err, lateErr) {
		t.Fatalf("error = %v, want %v", err, lateErr)
	}
	if recorded {
		t.Fatal("schema version was recorded after a late required-store failure")
	}
}

func TestRecordSchemaVersionAfterPersistencePreservesPreviousVersionOnLateFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kandev.db")
	raw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	writer := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	if _, err := writer.Exec(`
		CREATE TABLE kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
		INSERT INTO kandev_meta (key, value) VALUES ('kandev_version', 'v0.93.0')
	`); err != nil {
		t.Fatalf("seed kandev_meta: %v", err)
	}

	err = recordSchemaVersionAfterPersistence(
		func() error { return errors.New("late required store failed") },
		func() error { return nil },
		func() { recordSchemaVersion(writer, "sqlite", "current", nil) },
	)
	if err == nil {
		t.Fatal("late required-store failure was not returned")
	}
	var version string
	if err := writer.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, "kandev_version").Scan(&version); err != nil {
		t.Fatalf("read kandev_version: %v", err)
	}
	if version != "v0.93.0" {
		t.Fatalf("kandev_version = %q, want previous version v0.93.0", version)
	}
}

func TestRecordSchemaVersionAfterPersistenceRecordsAfterBothGates(t *testing.T) {
	var order []string
	err := recordSchemaVersionAfterPersistence(
		func() error { order = append(order, "validate"); return nil },
		func() error { order = append(order, "health"); return nil },
		func() { order = append(order, "record") },
	)
	if err != nil {
		t.Fatalf("recordSchemaVersionAfterPersistence: %v", err)
	}
	want := []string{"validate", "health", "record"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestProvideRepositoriesStopsOnRequiredMigrationFailure(t *testing.T) {
	home := t.TempDir()
	dataDir := filepath.Join(home, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	dbPath := filepath.Join(dataDir, "kandev.db")
	raw, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_, err = raw.Exec(`
		CREATE TABLE kandev_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO kandev_meta (key, value) VALUES ('kandev_version', 'v0.93.0');
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		INSERT INTO users (id, email, created_at, updated_at)
		VALUES ('legacy-a', 'same@example.test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
		       ('legacy-b', 'same@example.test', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`)
	if err != nil {
		_ = raw.Close()
		t.Fatalf("seed malformed users schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close seed database: %v", err)
	}

	cfg := &config.Config{
		HomeDir: home,
		Database: config.DatabaseConfig{
			Driver: "sqlite",
			Path:   dbPath,
		},
		Source: config.ConfigSource{Values: map[string]config.SettingSource{
			"database.path": config.SourceConfiguration,
		}},
	}
	_, _, _, err = provideRepositories(context.Background(), cfg, newTestLogger(), "test")
	if err == nil {
		t.Fatal("provideRepositories succeeded despite an injected required migration failure")
	}
	if !strings.Contains(err.Error(), "required user migration") ||
		!strings.Contains(err.Error(), "users.email_unique") {
		t.Fatalf("error = %v, want required users.email_unique migration failure", err)
	}

	check, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("reopen failed-upgrade database: %v", err)
	}
	defer func() { _ = check.Close() }()
	var version string
	if err := check.QueryRow(`SELECT value FROM kandev_meta WHERE key = ?`, "kandev_version").Scan(&version); err != nil {
		t.Fatalf("read previous schema version: %v", err)
	}
	if version != "v0.93.0" {
		t.Fatalf("kandev_version after late migration failure = %q, want v0.93.0", version)
	}
}
