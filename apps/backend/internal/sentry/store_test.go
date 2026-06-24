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

// createTestConfig inserts a config and returns its generated id.
func createTestConfig(t *testing.T, store *Store, name, url string) string {
	t.Helper()
	cfg := &SentryConfig{Name: name, AuthMethod: AuthMethodAuthToken, URL: url}
	if err := store.CreateConfig(context.Background(), cfg); err != nil {
		t.Fatalf("create config: %v", err)
	}
	if cfg.ID == "" {
		t.Fatal("expected generated id on create")
	}
	return cfg.ID
}

func TestStore_CreateGetDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id := createTestConfig(t, store, "Prod", "https://sentry.io")
	got, err := store.GetConfig(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.ID != id || got.Name != "Prod" || got.AuthMethod != AuthMethodAuthToken {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	if err := store.DeleteConfig(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, err := store.GetConfig(ctx, id)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if gone != nil {
		t.Errorf("expected nil after delete, got %+v", gone)
	}
}

func TestStore_GetConfig_Missing(t *testing.T) {
	store := newTestStore(t)
	cfg, err := store.GetConfig(context.Background(), "nope")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for missing config, got %+v", cfg)
	}
}

// TestStore_MultipleInstancesCoexist is the core multi-instance assertion: two
// distinct instances persist side by side and ListConfigs returns both.
func TestStore_MultipleInstancesCoexist(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	idA := createTestConfig(t, store, "SaaS", "https://sentry.io")
	idB := createTestConfig(t, store, "Self-hosted", "https://sentry.acme.com")
	if idA == idB {
		t.Fatal("expected distinct instance ids")
	}
	configs, err := store.ListConfigs(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(configs))
	}
	byID := map[string]*SentryConfig{configs[0].ID: configs[0], configs[1].ID: configs[1]}
	if byID[idA].URL != "https://sentry.io" || byID[idB].URL != "https://sentry.acme.com" {
		t.Errorf("instance URLs not preserved: %+v", configs)
	}
}

func TestStore_HasAnyConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	has, _ := store.HasAnyConfig(ctx)
	if has {
		t.Errorf("expected HasAnyConfig=false on empty store")
	}
	createTestConfig(t, store, "Prod", "https://sentry.io")
	has, _ = store.HasAnyConfig(ctx)
	if !has {
		t.Errorf("expected HasAnyConfig=true after create")
	}
}

func TestStore_UpdateConfig_RoundTripsNameAndURL(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id := createTestConfig(t, store, "Old", "https://sentry.io")
	if err := store.UpdateConfig(ctx, &SentryConfig{ID: id, Name: "New", AuthMethod: AuthMethodAuthToken, URL: "https://sentry.example.com"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := store.GetConfig(ctx, id)
	if err != nil || got == nil {
		t.Fatalf("get: %v / %v", err, got)
	}
	if got.Name != "New" || got.URL != "https://sentry.example.com" {
		t.Errorf("update not persisted: %+v", got)
	}
}

func TestStore_UpdateAuthHealth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id := createTestConfig(t, store, "Prod", "https://sentry.io")

	cfg, _ := store.GetConfig(ctx, id)
	if cfg.LastCheckedAt != nil {
		t.Errorf("expected nil last_checked_at on fresh row, got %v", cfg.LastCheckedAt)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, id, true, "", now); err != nil {
		t.Fatalf("update ok: %v", err)
	}
	cfg, _ = store.GetConfig(ctx, id)
	if !cfg.LastOk {
		t.Error("expected last_ok=true after successful probe")
	}
	if cfg.LastCheckedAt == nil || !cfg.LastCheckedAt.Equal(now) {
		t.Errorf("expected last_checked_at=%v, got %v", now, cfg.LastCheckedAt)
	}

	failAt := now.Add(time.Minute)
	if err := store.UpdateAuthHealth(ctx, id, false, "401 unauthorized", failAt); err != nil {
		t.Fatalf("update fail: %v", err)
	}
	cfg, _ = store.GetConfig(ctx, id)
	if cfg.LastOk {
		t.Error("expected last_ok=false after failure")
	}
	if cfg.LastError != "401 unauthorized" {
		t.Errorf("expected last_error preserved, got %q", cfg.LastError)
	}
}

// TestStore_UpdateAuthHealth_PerInstance asserts a probe write targets only the
// addressed instance, never bleeding into a sibling instance's row.
func TestStore_UpdateAuthHealth_PerInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	idA := createTestConfig(t, store, "A", "https://sentry.io")
	idB := createTestConfig(t, store, "B", "https://sentry.acme.com")
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateAuthHealth(ctx, idA, true, "", now); err != nil {
		t.Fatalf("update A: %v", err)
	}
	a, _ := store.GetConfig(ctx, idA)
	b, _ := store.GetConfig(ctx, idB)
	if !a.LastOk {
		t.Error("expected A healthy")
	}
	if b.LastCheckedAt != nil {
		t.Error("expected B untouched by A's probe write")
	}
}

func TestStore_CountWatchesForInstance(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id := createTestConfig(t, store, "A", "https://sentry.io")
	if n, _ := store.CountWatchesForInstance(ctx, id); n != 0 {
		t.Fatalf("expected 0 watches, got %d", n)
	}
	w := &IssueWatch{
		WorkspaceID:    "ws-1",
		InstanceID:     id,
		WorkflowID:     "wf-1",
		WorkflowStepID: "step-1",
		Filter:         SearchFilter{OrgSlug: "org", ProjectSlug: "proj"},
	}
	if err := store.CreateIssueWatch(ctx, w); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if n, _ := store.CountWatchesForInstance(ctx, id); n != 1 {
		t.Fatalf("expected 1 watch, got %d", n)
	}
	if n, _ := store.CountWatchesForInstance(ctx, "other"); n != 0 {
		t.Fatalf("expected 0 watches for other instance, got %d", n)
	}
}

// TestStore_MigratesSingletonToInstance seeds the legacy single-tenant schema
// (CHECK(id='singleton'), no name column) plus a watch bound to it, then asserts
// NewStore promotes the row to a generated instance id, names it after its URL
// host, exposes the migrated id, and backfills the watch.
func TestStore_MigratesSingletonToInstance(t *testing.T) {
	db := newRawDB(t)
	now := time.Now().UTC()
	seedLegacyConfigs(t, db, true)
	if _, err := db.Exec(`INSERT INTO sentry_configs (id, auth_method, url, created_at, updated_at)
		VALUES ('singleton', ?, 'https://sentry.acme.com', ?, ?)`, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE sentry_issue_watches (
		id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, workflow_id TEXT NOT NULL,
		workflow_step_id TEXT NOT NULL, filter_json TEXT NOT NULL DEFAULT '{}',
		agent_profile_id TEXT NOT NULL DEFAULT '', executor_profile_id TEXT NOT NULL DEFAULT '',
		prompt TEXT NOT NULL DEFAULT '', enabled BOOLEAN NOT NULL DEFAULT 1,
		poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
		max_inflight_tasks INTEGER DEFAULT 5,
		last_polled_at DATETIME,
		last_error TEXT NOT NULL DEFAULT '', last_error_at DATETIME,
		created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`); err != nil {
		t.Fatalf("seed legacy watches: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sentry_issue_watches (id, workspace_id, workflow_id, workflow_step_id, created_at, updated_at)
		VALUES ('watch-1', 'ws-1', 'wf-1', 'step-1', ?, ?)`, now, now); err != nil {
		t.Fatalf("seed legacy watch row: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store (migrate): %v", err)
	}
	if store.MigratedSingletonID() == "" {
		t.Fatal("expected MigratedSingletonID to be set")
	}
	configs, err := store.ListConfigs(context.Background())
	if err != nil || len(configs) != 1 {
		t.Fatalf("expected 1 migrated instance, got %d (%v)", len(configs), err)
	}
	migrated := configs[0]
	if migrated.ID == legacySingletonID || migrated.ID != store.MigratedSingletonID() {
		t.Errorf("expected generated id, got %q", migrated.ID)
	}
	if migrated.Name != "sentry.acme.com" {
		t.Errorf("expected name derived from URL host, got %q", migrated.Name)
	}
	if migrated.URL != "https://sentry.acme.com" {
		t.Errorf("expected URL preserved, got %q", migrated.URL)
	}
	// The legacy watch must now point at the migrated instance.
	w, err := store.GetIssueWatch(context.Background(), "watch-1")
	if err != nil || w == nil {
		t.Fatalf("get migrated watch: %v / %v", err, w)
	}
	if w.InstanceID != store.MigratedSingletonID() {
		t.Errorf("expected watch backfilled to %q, got %q", store.MigratedSingletonID(), w.InstanceID)
	}
}

// TestStore_MigratesLegacyWithoutURLColumn covers the oldest schema (no url
// column): the url is backfilled to the SaaS default before the table rebuild,
// and the promoted instance is named after that default host.
func TestStore_MigratesLegacyWithoutURLColumn(t *testing.T) {
	db := newRawDB(t)
	now := time.Now().UTC()
	seedLegacyConfigs(t, db, false)
	if _, err := db.Exec(`INSERT INTO sentry_configs (id, auth_method, created_at, updated_at)
		VALUES ('singleton', ?, ?, ?)`, AuthMethodAuthToken, now, now); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store (migrate): %v", err)
	}
	configs, err := store.ListConfigs(context.Background())
	if err != nil || len(configs) != 1 {
		t.Fatalf("expected 1 migrated instance, got %d (%v)", len(configs), err)
	}
	if configs[0].URL != DefaultSentryURL {
		t.Errorf("expected url backfilled to %q, got %q", DefaultSentryURL, configs[0].URL)
	}
}

// TestStore_FreshInstall_NoMigration confirms a fresh DB needs no rebuild and
// records no migrated singleton id.
func TestStore_FreshInstall_NoMigration(t *testing.T) {
	store := newTestStore(t)
	if store.MigratedSingletonID() != "" {
		t.Errorf("expected no migration on fresh install, got %q", store.MigratedSingletonID())
	}
}

func newRawDB(t *testing.T) *sqlx.DB {
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

// seedLegacyConfigs creates the legacy singleton sentry_configs table, with or
// without the url column, to drive the migration tests.
func seedLegacyConfigs(t *testing.T, db *sqlx.DB, withURL bool) {
	t.Helper()
	urlCol := ""
	if withURL {
		urlCol = "url TEXT NOT NULL DEFAULT 'https://sentry.io',\n"
	}
	if _, err := db.Exec(`CREATE TABLE sentry_configs (
		id TEXT PRIMARY KEY CHECK(id = 'singleton'),
		auth_method TEXT NOT NULL,
		` + urlCol + `last_checked_at DATETIME,
		last_ok INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL)`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
}
