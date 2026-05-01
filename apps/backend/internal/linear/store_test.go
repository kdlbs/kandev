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
		WorkspaceID:    "ws-1",
		AuthMethod:     AuthMethodAPIKey,
		DefaultTeamKey: "ENG",
	}
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := store.GetConfig(ctx, "ws-1")
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
	got2, _ := store.GetConfig(ctx, "ws-1")
	if got2.DefaultTeamKey != "MOB" {
		t.Errorf("expected team update, got %q", got2.DefaultTeamKey)
	}

	if err := store.DeleteConfig(ctx, "ws-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, err := store.GetConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("expected nil after delete, got %+v", gone)
	}
}

func TestStore_GetConfig_Missing(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.GetConfig(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for missing config, got %+v", cfg)
	}
}

func TestStore_ListConfiguredWorkspaces(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mustUpsert := func(id string) {
		if err := store.UpsertConfig(ctx, &LinearConfig{
			WorkspaceID: id,
			AuthMethod:  AuthMethodAPIKey,
		}); err != nil {
			t.Fatalf("upsert %s: %v", id, err)
		}
	}
	mustUpsert("ws-b")
	mustUpsert("ws-a")
	ids, err := store.ListConfiguredWorkspaces(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ids) != 2 || ids[0] != "ws-a" || ids[1] != "ws-b" {
		t.Errorf("expected sorted [ws-a ws-b], got %v", ids)
	}
}

func TestStore_UpdateAuthHealth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.UpsertConfig(ctx, &LinearConfig{
		WorkspaceID: "ws-1",
		AuthMethod:  AuthMethodAPIKey,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	cfg, _ := store.GetConfig(ctx, "ws-1")
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", cfg.LastCheckedAt)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, "ws-1", true, "", "acme", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	cfg, _ = store.GetConfig(ctx, "ws-1")
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
	if err := store.UpdateAuthHealth(ctx, "ws-1", false, "401 unauthorized", "", failAt); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	cfg, _ = store.GetConfig(ctx, "ws-1")
	if cfg.LastOk {
		t.Error("expected last_ok=false after failure")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected last_error preserved, got %q", cfg.LastError)
	}
	if cfg.OrgSlug != "acme" {
		t.Errorf("orgSlug should be preserved across failed probe, got %q", cfg.OrgSlug)
	}

	// Update for missing workspace must not error.
	if err := store.UpdateAuthHealth(ctx, "missing", true, "", "x", now); err != nil {
		t.Errorf("expected no-op for missing row, got %v", err)
	}
}
