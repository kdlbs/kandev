package linear

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func TestStore_UpsertGetDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cfg := &LinearConfig{
		AuthMethod:     AuthMethodAPIKey,
		DefaultTeamKey: "ENG",
	}
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.AuthMethod != cfg.AuthMethod || got.DefaultTeamKey != cfg.DefaultTeamKey {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, cfg)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	cfg.DefaultTeamKey = "MOB"
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("update upsert: %v", err)
	}
	got2, _ := store.GetConfig(ctx)
	if got2.DefaultTeamKey != "MOB" {
		t.Errorf("expected team update, got %q", got2.DefaultTeamKey)
	}

	if err := store.DeleteConfig(ctx); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, err := store.GetConfig(ctx)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("expected nil after delete, got %+v", gone)
	}
}

func TestStore_GetConfig_Missing(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for missing config, got %+v", cfg)
	}
}

func TestStore_HasConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	has, err := store.HasConfig(ctx)
	if err != nil {
		t.Fatalf("has-config: %v", err)
	}
	if has {
		t.Errorf("expected HasConfig=false on empty store")
	}
	if err := store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	has, err = store.HasConfig(ctx)
	if err != nil {
		t.Fatalf("has-config after upsert: %v", err)
	}
	if !has {
		t.Errorf("expected HasConfig=true after upsert")
	}
}

func TestStore_UpdateAuthHealth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.UpsertConfig(ctx, &LinearConfig{
		AuthMethod: AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cfg, _ := store.GetConfig(ctx)
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", cfg.LastCheckedAt)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, true, "", "acme", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if !cfg.LastOk {
		t.Error("expected last_ok=true after successful probe")
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("expected org_slug=acme, got %q", cfg.OrgSlug)
	}
	if cfg.LastCheckedAt == nil || !cfg.LastCheckedAt.Equal(now) {
		t.Errorf("expected last_checked_at=%v, got %v", now, cfg.LastCheckedAt)
	}

	// Empty orgSlug should leave the existing slug intact.
	failAt := now.Add(time.Minute)
	if err := store.UpdateAuthHealth(ctx, false, "401 unauthorized", "", failAt); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if cfg.LastOk {
		t.Error("expected last_ok=false after failure")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected last_error preserved, got %q", cfg.LastError)
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("orgSlug should be preserved across failed probe, got %q", cfg.OrgSlug)
	}
}

func TestStore_MigrateLegacyPerWorkspaceTable(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE linear_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			default_team_key TEXT NOT NULL DEFAULT '',
			org_slug TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	older := time.Now().UTC().Add(-time.Hour)
	newer := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO linear_configs
		(workspace_id, auth_method, default_team_key, org_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"ws-old", AuthMethodAPIKey, "OLD", "old-slug", older, older); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO linear_configs
		(workspace_id, auth_method, default_team_key, org_slug, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"ws-new", AuthMethodAPIKey, "NEW", "new-slug", newer, newer); err != nil {
		t.Fatalf("seed new row: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if got := store.MigratedFromWorkspace(); got != "ws-new" {
		t.Errorf("expected migration source ws-new, got %q", got)
	}
	cfg, err := store.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("get singleton: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected singleton row after migration")
	}
	if cfg.DefaultTeamKey != "NEW" || cfg.OrgSlug != "new-slug" {
		t.Errorf("expected newest row promoted, got %+v", cfg)
	}
}

// Covers the original-schema upgrade path: the legacy table exists with
// `workspace_id` but lacks the auth-health columns and `org_slug` added in
// later releases. The migration must select only guaranteed-present columns
// and fall back to defaults for the missing ones, otherwise startup crashes
// with "no such column". Mirrors the Jira test.
func TestStore_MigrateLegacyPerWorkspaceTable_PreHealthColumns(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	// Original schema: no org_slug / last_checked_at / last_ok / last_error.
	if _, err := db.Exec(`
		CREATE TABLE linear_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			default_team_key TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO linear_configs
		(workspace_id, auth_method, default_team_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		"ws-1", AuthMethodAPIKey, "ENG", now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("NewStore on pre-health-columns schema: %v", err)
	}
	if got := store.MigratedFromWorkspace(); got != "ws-1" {
		t.Errorf("expected migration source ws-1, got %q", got)
	}
	cfg, err := store.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("get singleton: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected singleton row after migration")
	}
	if cfg.DefaultTeamKey != "ENG" {
		t.Errorf("expected DefaultTeamKey preserved, got %q", cfg.DefaultTeamKey)
	}
	if cfg.OrgSlug != "" {
		t.Errorf("expected OrgSlug='' (default) after migrating from pre-org-slug schema, got %q", cfg.OrgSlug)
	}
	if cfg.LastOk {
		t.Error("expected LastOk=false (default) after migrating from pre-health schema")
	}
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected LastCheckedAt=nil (default), got %v", cfg.LastCheckedAt)
	}
}

// Covers the sql.ErrNoRows branch: a user who installed a previous version,
// never configured Linear, and then upgrades hits this path.
func TestStore_MigrateLegacyPerWorkspaceTable_EmptyTable(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE linear_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			default_team_key TEXT NOT NULL DEFAULT '',
			org_slug TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("NewStore on empty legacy table: %v", err)
	}
	if got := store.MigratedFromWorkspace(); got != "" {
		t.Errorf("expected empty MigratedFromWorkspace, got %q", got)
	}
	cfg, err := store.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("get singleton: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil cfg on empty legacy table, got %+v", cfg)
	}
}
