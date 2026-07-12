package workspacescope

import (
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedActiveWorkspace(t *testing.T, db *sqlx.DB, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			settings TEXT NOT NULL DEFAULT '{}',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		DELETE FROM workspaces;
		DELETE FROM users;
		INSERT INTO workspaces (id, name, created_at, updated_at)
			VALUES (?, 'Active', ?, ?);
		INSERT INTO users (id, email, settings, created_at, updated_at)
			VALUES ('default-user', 'default@kandev.local', ?, ?, ?);
	`, workspaceID, now, now, `{"workspace_id":"`+workspaceID+`"}`, now, now); err != nil {
		t.Fatalf("seed active workspace: %v", err)
	}
}

func TestDefaultResolverReflectsActiveWorkspaceChange(t *testing.T) {
	db := newTestDB(t)
	seedActiveWorkspace(t, db, "ws-first")

	var resolver DefaultResolver
	got, err := resolver.Resolve(db)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	if got != "ws-first" {
		t.Fatalf("expected ws-first, got %q", got)
	}

	// The active workspace is mutable at runtime; switching it must be
	// reflected on the next resolve, not pinned to the first value.
	seedActiveWorkspace(t, db, "ws-second")
	got, err = resolver.Resolve(db)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if got != "ws-second" {
		t.Fatalf("expected ws-second after switch, got %q", got)
	}
}

func TestDefaultResolverIsPerDB(t *testing.T) {
	db1 := newTestDB(t)
	db2 := newTestDB(t)
	seedActiveWorkspace(t, db1, "ws-one")
	seedActiveWorkspace(t, db2, "ws-two")

	var resolver DefaultResolver
	got, err := resolver.Resolve(db1)
	if err != nil {
		t.Fatalf("resolve db1: %v", err)
	}
	if got != "ws-one" {
		t.Fatalf("expected ws-one, got %q", got)
	}
	got, err = resolver.Resolve(db2)
	if err != nil {
		t.Fatalf("resolve db2: %v", err)
	}
	if got != "ws-two" {
		t.Fatalf("expected ws-two for second db, got %q", got)
	}
}

func TestDefaultResolverRecoversAfterError(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	if _, err := db.Exec(`
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE users (
			id TEXT PRIMARY KEY
		);
		INSERT INTO workspaces (id, name, created_at, updated_at)
			VALUES ('ws-active', 'Active', ?, ?);
	`, now, now); err != nil {
		t.Fatalf("seed malformed users table: %v", err)
	}

	var resolver DefaultResolver
	if _, err := resolver.Resolve(db); err == nil {
		t.Fatal("expected malformed users table to fail")
	}

	if _, err := db.Exec(`DROP TABLE users`); err != nil {
		t.Fatalf("drop malformed users table: %v", err)
	}
	seedActiveWorkspace(t, db, "ws-active")

	got, err := resolver.Resolve(db)
	if err != nil {
		t.Fatalf("resolve after fixing users table: %v", err)
	}
	if got != "ws-active" {
		t.Fatalf("expected ws-active after retry, got %q", got)
	}
}
