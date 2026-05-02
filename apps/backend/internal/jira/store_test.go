package jira

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

	cfg := &JiraConfig{
		SiteURL:           "https://acme.atlassian.net",
		Email:             "user@example.com",
		AuthMethod:        AuthMethodAPIToken,
		DefaultProjectKey: "PROJ",
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
	if got.SiteURL != cfg.SiteURL || got.Email != cfg.Email {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, cfg)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	// Update-in-place
	cfg.Email = "other@example.com"
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("update upsert: %v", err)
	}
	got2, _ := store.GetConfig(ctx)
	if got2.Email != "other@example.com" {
		t.Errorf("expected email update, got %q", got2.Email)
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
		t.Errorf("expected HasConfig=false on empty store, got true")
	}
	if err := store.UpsertConfig(ctx, &JiraConfig{
		SiteURL:    "https://acme.atlassian.net",
		Email:      "user@example.com",
		AuthMethod: AuthMethodAPIToken,
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
	if err := store.UpsertConfig(ctx, &JiraConfig{
		SiteURL:    "https://acme.atlassian.net",
		Email:      "u@example.com",
		AuthMethod: AuthMethodAPIToken,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	// Initial state: no health recorded yet.
	cfg, _ := store.GetConfig(ctx)
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", cfg.LastCheckedAt)
	}
	if cfg.LastOk {
		t.Error("expected last_ok=false on fresh row")
	}

	// Record success.
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, true, "", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if !cfg.LastOk {
		t.Error("expected last_ok=true after successful probe")
	}
	if cfg.LastError != "" {
		t.Errorf("expected empty last_error, got %q", cfg.LastError)
	}
	if cfg.LastCheckedAt == nil || !cfg.LastCheckedAt.Equal(now) {
		t.Errorf("expected last_checked_at=%v, got %v", now, cfg.LastCheckedAt)
	}

	// Record failure — error string is preserved, ok flips back to false.
	failAt := now.Add(time.Minute)
	if err := store.UpdateAuthHealth(ctx, false, "step-up required", failAt); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if cfg.LastOk {
		t.Error("expected last_ok=false after failure")
	}
	if cfg.LastError != "step-up required" {
		t.Errorf("expected last_error preserved, got %q", cfg.LastError)
	}
	if cfg.LastCheckedAt == nil || !cfg.LastCheckedAt.Equal(failAt) {
		t.Errorf("expected last_checked_at=%v, got %v", failAt, cfg.LastCheckedAt)
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

	// Build the legacy schema and seed two per-workspace rows; the most
	// recently-updated row should win the singleton slot.
	if _, err := db.Exec(`
		CREATE TABLE jira_configs (
			workspace_id TEXT PRIMARY KEY,
			site_url TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL,
			default_project_key TEXT NOT NULL DEFAULT '',
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
	if _, err := db.Exec(`INSERT INTO jira_configs
		(workspace_id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ws-old", "https://old.atlassian.net", "old@example.com", AuthMethodAPIToken, "OLD", older, older); err != nil {
		t.Fatalf("seed old row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO jira_configs
		(workspace_id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ws-new", "https://new.atlassian.net", "new@example.com", AuthMethodAPIToken, "NEW", newer, newer); err != nil {
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
	if cfg.SiteURL != "https://new.atlassian.net" {
		t.Errorf("expected newest row promoted, got SiteURL=%q", cfg.SiteURL)
	}
}

// Covers the sql.ErrNoRows branch: a user who installed a previous version,
// never configured Jira, and then upgrades hits this path. The legacy table
// exists but contains zero rows.
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
		CREATE TABLE jira_configs (
			workspace_id TEXT PRIMARY KEY,
			site_url TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL,
			default_project_key TEXT NOT NULL DEFAULT '',
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

// Covers the original-schema upgrade path: the legacy table exists with
// `workspace_id` but lacks the auth-health columns added in a later release.
// The migration must select only guaranteed-present columns and fall back to
// defaults for the missing ones, otherwise startup crashes with "no such column".
func TestStore_MigrateLegacyPerWorkspaceTable_PreHealthColumns(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	// Original schema: no last_checked_at / last_ok / last_error.
	if _, err := db.Exec(`
		CREATE TABLE jira_configs (
			workspace_id TEXT PRIMARY KEY,
			site_url TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			auth_method TEXT NOT NULL,
			default_project_key TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO jira_configs
		(workspace_id, site_url, email, auth_method, default_project_key, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"ws-1", "https://acme.atlassian.net", "u@example.com", AuthMethodAPIToken, "PROJ", now, now); err != nil {
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
	if cfg.SiteURL != "https://acme.atlassian.net" {
		t.Errorf("expected SiteURL preserved, got %q", cfg.SiteURL)
	}
	if cfg.LastOk {
		t.Error("expected LastOk=false (default) after migrating from pre-health schema")
	}
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected LastCheckedAt=nil (default), got %v", cfg.LastCheckedAt)
	}
}
