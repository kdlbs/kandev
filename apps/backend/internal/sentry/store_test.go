package sentry

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

	cfg := &SentryConfig{AuthMethod: AuthMethodAuthToken}
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
	if got.AuthMethod != cfg.AuthMethod {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, cfg)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	// Idempotent upsert does not duplicate rows.
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("update upsert: %v", err)
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
	has, _ := store.HasConfig(ctx)
	if has {
		t.Errorf("expected HasConfig=false on empty store")
	}
	if err := store.UpsertConfig(ctx, &SentryConfig{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	has, _ = store.HasConfig(ctx)
	if !has {
		t.Errorf("expected HasConfig=true after upsert")
	}
}

func TestStore_UpdateAuthHealth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.UpsertConfig(ctx, &SentryConfig{AuthMethod: AuthMethodAuthToken}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cfg, _ := store.GetConfig(ctx)
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", cfg.LastCheckedAt)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, true, "", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if !cfg.LastOk {
		t.Error("expected last_ok=true after successful probe")
	}
	if cfg.LastCheckedAt == nil || !cfg.LastCheckedAt.Equal(now) {
		t.Errorf("expected last_checked_at=%v, got %v", now, cfg.LastCheckedAt)
	}

	failAt := now.Add(time.Minute)
	if err := store.UpdateAuthHealth(ctx, false, "401 unauthorized", failAt); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	cfg, _ = store.GetConfig(ctx)
	if cfg.LastOk {
		t.Error("expected last_ok=false after failure")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected last_error preserved, got %q", cfg.LastError)
	}
}

// TestStore_UpsertGetConfig_RoundTripsURL asserts the instance URL persists
// through an upsert/get cycle.
func TestStore_UpsertGetConfig_RoundTripsURL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := &SentryConfig{AuthMethod: AuthMethodAuthToken, URL: "https://sentry.example.com"}
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := store.GetConfig(ctx)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.URL != "https://sentry.example.com" {
		t.Errorf("url not persisted: got %q", got.URL)
	}
}

// TestStore_MigratesURLColumn seeds a pre-self-hosted sentry_configs table (no
// url column) and verifies NewStore adds the column, backfilling the existing
// SaaS row to the sentry.io default rather than an empty string.
func TestStore_MigratesURLColumn(t *testing.T) {
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UTC()
	if _, err := db.Exec(`
		CREATE TABLE sentry_configs (
			id TEXT PRIMARY KEY CHECK(id = 'singleton'),
			auth_method TEXT NOT NULL,
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sentry_configs (id, auth_method, created_at, updated_at)
		VALUES ('singleton', ?, ?, ?)`, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store (migrate): %v", err)
	}
	cfg, err := store.GetConfig(context.Background())
	if err != nil || cfg == nil {
		t.Fatalf("get after migrate: %v / %v", err, cfg)
	}
	if cfg.URL != DefaultSentryURL {
		t.Errorf("expected legacy row backfilled to %q, got %q", DefaultSentryURL, cfg.URL)
	}
}
