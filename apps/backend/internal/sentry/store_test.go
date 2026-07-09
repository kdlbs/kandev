package sentry

import (
	"context"
	"database/sql"
	"errors"
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
	// Enforce foreign keys as production does (DSN _foreign_keys=on) so the
	// ON DELETE RESTRICT net for in-use instances is exercised in tests.
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}

func testInstance(workspaceID, name string) *SentryConfig {
	return &SentryConfig{
		WorkspaceID: workspaceID,
		Name:        name,
		AuthMethod:  AuthMethodAuthToken,
		URL:         DefaultSentryURL,
	}
}

func TestStore_CreateGetInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	cfg := testInstance("ws-1", "SaaS")
	cfg.URL = "https://sentry.example.com"
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected ID assigned")
	}
	if cfg.CreatedAt.IsZero() || cfg.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	got, err := store.GetInstance(ctx, cfg.ID)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.WorkspaceID != "ws-1" || got.Name != "SaaS" || got.URL != "https://sentry.example.com" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestStore_GetInstance_Missing(t *testing.T) {
	store := newTestStore(t)
	got, err := store.GetInstance(context.Background(), "ghost")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing instance, got %+v", got)
	}
}

func TestStore_ListInstances_ScopedByWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, c := range []*SentryConfig{
		testInstance("ws-1", "A"),
		testInstance("ws-1", "B"),
		testInstance("ws-2", "C"),
	} {
		if err := store.CreateInstance(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
	}
	got, err := store.ListInstances(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 instances for ws-1, got %d", len(got))
	}
	for _, c := range got {
		if c.WorkspaceID != "ws-1" {
			t.Errorf("workspace leaked into list: %s", c.WorkspaceID)
		}
	}
}

// TestStore_UniqueNamePerWorkspace pins acceptance (h): (workspace_id, name) is
// unique, but the same name is allowed in a different workspace.
func TestStore_UniqueNamePerWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.CreateInstance(ctx, testInstance("ws-1", "Prod")); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if err := store.CreateInstance(ctx, testInstance("ws-1", "Prod")); !errors.Is(err, ErrDuplicateInstanceName) {
		t.Fatalf("expected ErrDuplicateInstanceName for duplicate name in same workspace, got %v", err)
	}
	// Same name in a different workspace is fine.
	if err := store.CreateInstance(ctx, testInstance("ws-2", "Prod")); err != nil {
		t.Fatalf("same name in other workspace should succeed, got %v", err)
	}
}

func TestStore_UpdateInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := testInstance("ws-1", "Old")
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	created := cfg.CreatedAt
	cfg.Name = "New"
	cfg.URL = "https://sentry.example.com"
	if err := store.UpdateInstance(ctx, cfg); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := store.GetInstance(ctx, cfg.ID)
	if got.Name != "New" || got.URL != "https://sentry.example.com" {
		t.Errorf("update did not persist: %+v", got)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("update must not change created_at: %v vs %v", got.CreatedAt, created)
	}

	// Update to a name already taken in the workspace is rejected.
	other := testInstance("ws-1", "Taken")
	if err := store.CreateInstance(ctx, other); err != nil {
		t.Fatalf("create other: %v", err)
	}
	cfg.Name = "Taken"
	if err := store.UpdateInstance(ctx, cfg); !errors.Is(err, ErrDuplicateInstanceName) {
		t.Errorf("expected ErrDuplicateInstanceName on rename collision, got %v", err)
	}

	// Update of a missing row is ErrInstanceNotFound.
	ghost := testInstance("ws-1", "Ghost")
	ghost.ID = "does-not-exist"
	if err := store.UpdateInstance(ctx, ghost); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound, got %v", err)
	}
}

func TestStore_DeleteInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := testInstance("ws-1", "Prod")
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := store.DeleteInstance(ctx, cfg.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	got, _ := store.GetInstance(ctx, cfg.ID)
	if got != nil {
		t.Errorf("expected instance gone, got %+v", got)
	}
	if err := store.DeleteInstance(ctx, cfg.ID); !errors.Is(err, ErrInstanceNotFound) {
		t.Errorf("expected ErrInstanceNotFound on second delete, got %v", err)
	}
}

// TestStore_DeleteInstance_FKRestrictHolds pins the DB-level safety net from
// acceptance (f): a watch referencing an instance blocks its deletion via ON
// DELETE RESTRICT, surfaced as ErrInstanceInUse.
func TestStore_DeleteInstance_FKRestrictHolds(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := testInstance("ws-1", "Prod")
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("create instance: %v", err)
	}
	w := newTestIssueWatch("ws-1")
	w.SentryInstanceID = cfg.ID
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if err := store.DeleteInstance(ctx, cfg.ID); !errors.As(err, &ErrInstanceInUse{}) {
		t.Fatalf("expected ErrInstanceInUse from FK RESTRICT, got %v", err)
	}
	n, err := store.CountWatchesForInstance(ctx, cfg.ID)
	if err != nil || n != 1 {
		t.Fatalf("expected 1 referencing watch, got %d (err %v)", n, err)
	}
}

func TestStore_HasConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	has, _ := store.HasConfig(ctx)
	if has {
		t.Error("expected HasConfig=false on empty store")
	}
	if err := store.CreateInstance(ctx, testInstance("ws-1", "A")); err != nil {
		t.Fatalf("create: %v", err)
	}
	has, _ = store.HasConfig(ctx)
	if !has {
		t.Error("expected HasConfig=true after create")
	}
}

func TestStore_UpdateAuthHealthForInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	cfg := testInstance("ws-1", "A")
	if err := store.CreateInstance(ctx, cfg); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, _ := store.GetInstance(ctx, cfg.ID)
	if got.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", got.LastCheckedAt)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealthForInstance(ctx, cfg.ID, true, "", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	got, _ = store.GetInstance(ctx, cfg.ID)
	if !got.LastOk || got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(now) {
		t.Errorf("unexpected health state after success: %+v", got)
	}
	if err := store.UpdateAuthHealthForInstance(ctx, cfg.ID, false, "401 unauthorized", now.Add(time.Minute)); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	got, _ = store.GetInstance(ctx, cfg.ID)
	if got.LastOk || got.LastError != "401 unauthorized" {
		t.Errorf("expected failure recorded, got %+v", got)
	}
}
