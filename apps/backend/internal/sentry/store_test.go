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

	cfg := &SentryConfig{
		AuthMethod:         AuthMethodAuthToken,
		DefaultOrgSlug:     "acme",
		DefaultProjectSlug: "frontend",
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
	if got.AuthMethod != cfg.AuthMethod || got.DefaultOrgSlug != cfg.DefaultOrgSlug ||
		got.DefaultProjectSlug != cfg.DefaultProjectSlug {
		t.Errorf("round-trip mismatch: %+v vs %+v", got, cfg)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	// Idempotent upsert updates the project slug without duplicating rows.
	cfg.DefaultProjectSlug = "backend"
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("update upsert: %v", err)
	}
	got2, _ := store.GetConfig(ctx)
	if got2.DefaultProjectSlug != "backend" {
		t.Errorf("expected project update, got %q", got2.DefaultProjectSlug)
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
