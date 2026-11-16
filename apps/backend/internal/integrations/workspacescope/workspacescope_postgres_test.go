package workspacescope

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
)

func TestDefaultResolverPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := t.Context()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE workspaces (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			settings TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL
		)`); err != nil {
		t.Fatalf("create users table: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `INSERT INTO workspaces (id, created_at) VALUES ($1, $2)`, "ws-postgres", now); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO users (id, settings, updated_at) VALUES ($1, $2, $3)`, "user-postgres", `{"workspace_id":"ws-postgres"}`, now); err != nil {
		t.Fatalf("insert active workspace setting: %v", err)
	}

	got, err := (DefaultResolver{}).Resolve(db)
	if err != nil {
		t.Fatalf("resolve Postgres workspace: %v", err)
	}
	if got != "ws-postgres" {
		t.Fatalf("resolved workspace = %q, want ws-postgres", got)
	}
}
