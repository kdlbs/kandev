package slack

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

	cfg := &SlackConfig{
		AuthMethod:          AuthMethodCookie,
		CommandPrefix:       "!kandev",
		UtilityAgentID:      "ua-1",
		PollIntervalSeconds: 30,
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
	if got.UtilityAgentID != "ua-1" || got.CommandPrefix != "!kandev" || got.PollIntervalSeconds != 30 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Error("timestamps not set")
	}

	cfg.UtilityAgentID = "ua-2"
	cfg.CommandPrefix = "!todo"
	if err := store.UpsertConfig(ctx, cfg); err != nil {
		t.Fatalf("update upsert: %v", err)
	}
	got2, _ := store.GetConfig(ctx)
	if got2.UtilityAgentID != "ua-2" || got2.CommandPrefix != "!todo" {
		t.Errorf("expected updates, got %+v", got2)
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
		t.Fatalf("get: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for missing config, got %+v", cfg)
	}
}

func TestStore_HasConfig(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	has, err := store.HasConfig(ctx)
	if err != nil || has {
		t.Fatalf("expected no config, got has=%v err=%v", has, err)
	}
	_ = store.UpsertConfig(ctx, &SlackConfig{
		AuthMethod:          AuthMethodCookie,
		UtilityAgentID:      "ua",
		PollIntervalSeconds: 30,
	})
	has, err = store.HasConfig(ctx)
	if err != nil || !has {
		t.Fatalf("expected has=true after upsert, got has=%v err=%v", has, err)
	}
}

func TestStore_UpdateAuthHealth(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.UpsertConfig(ctx, &SlackConfig{
		AuthMethod:          AuthMethodCookie,
		UtilityAgentID:      "ua",
		PollIntervalSeconds: 30,
	})

	now := time.Now().UTC()
	if err := store.UpdateAuthHealth(ctx, true, "", "T0001", "U0001", now); err != nil {
		t.Fatalf("update auth health: %v", err)
	}
	got, _ := store.GetConfig(ctx)
	if !got.LastOk || got.SlackTeamID != "T0001" || got.SlackUserID != "U0001" {
		t.Errorf("expected health to be recorded, got %+v", got)
	}

	if err := store.UpdateAuthHealth(ctx, false, "expired", "", "", now.Add(time.Minute)); err != nil {
		t.Fatalf("update auth health 2: %v", err)
	}
	got2, _ := store.GetConfig(ctx)
	if got2.LastOk {
		t.Error("expected lastOk false")
	}
	if got2.LastError != "expired" {
		t.Errorf("expected error message, got %q", got2.LastError)
	}
	if got2.SlackTeamID != "T0001" || got2.SlackUserID != "U0001" {
		t.Errorf("expected captured ids preserved across failure, got team=%q user=%q",
			got2.SlackTeamID, got2.SlackUserID)
	}
}

func TestStore_UpdateLastSeenTS(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	_ = store.UpsertConfig(ctx, &SlackConfig{
		AuthMethod:          AuthMethodCookie,
		UtilityAgentID:      "ua",
		PollIntervalSeconds: 30,
	})
	if err := store.UpdateLastSeenTS(ctx, "1714659000.000100"); err != nil {
		t.Fatalf("update ts: %v", err)
	}
	got, _ := store.GetConfig(ctx)
	if got.LastSeenTS != "1714659000.000100" {
		t.Errorf("expected ts persisted, got %q", got.LastSeenTS)
	}
}

func TestStore_LegacyPerWorkspaceMigration(t *testing.T) {
	// Simulate a deployment running the pre-singleton schema: a slack_configs
	// table keyed by workspace_id, with the most-recently-updated row to be
	// promoted into the singleton on upgrade.
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE slack_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			command_prefix TEXT NOT NULL DEFAULT '',
			utility_agent_id TEXT NOT NULL DEFAULT '',
			poll_interval_seconds INTEGER NOT NULL DEFAULT 30,
			slack_team_id TEXT NOT NULL DEFAULT '',
			slack_user_id TEXT NOT NULL DEFAULT '',
			last_seen_ts TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	now := time.Now().UTC()
	older := now.Add(-time.Hour)
	if _, err := db.Exec(`
		INSERT INTO slack_configs (workspace_id, auth_method, command_prefix, utility_agent_id,
			poll_interval_seconds, slack_team_id, slack_user_id, last_seen_ts,
			last_checked_at, last_ok, last_error, created_at, updated_at)
		VALUES
			('ws-old', 'cookie', '!kandev', 'ua-old', 30, 'T-old', 'U-old', '100.0', ?, 1, '', ?, ?),
			('ws-new', 'cookie', '!kandev', 'ua-new', 60, 'T-new', 'U-new', '200.0', ?, 1, '', ?, ?)`,
		older, older, older, now, now, now); err != nil {
		t.Fatalf("seed legacy rows: %v", err)
	}

	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if got := store.MigratedFromWorkspace(); got != "ws-new" {
		t.Errorf("expected migration to promote ws-new (most recently updated), got %q", got)
	}
	cfg, err := store.GetConfig(context.Background())
	if err != nil || cfg == nil {
		t.Fatalf("get post-migration: cfg=%v err=%v", cfg, err)
	}
	if cfg.UtilityAgentID != "ua-new" || cfg.CommandPrefix != "!kandev" || cfg.PollIntervalSeconds != 60 {
		t.Errorf("expected ws-new fields preserved, got %+v", cfg)
	}
	if cfg.SlackTeamID != "T-new" || cfg.SlackUserID != "U-new" || cfg.LastSeenTS != "200.0" {
		t.Errorf("expected ws-new runtime state preserved, got %+v", cfg)
	}
}

func TestStore_LegacyEmptyTableMigration(t *testing.T) {
	// A deployment that had the legacy table created but never inserted a
	// row should still upgrade cleanly without crashing on the empty SELECT.
	raw, _ := sql.Open("sqlite3", ":memory:")
	raw.SetMaxOpenConns(1)
	db := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE slack_configs (
			workspace_id TEXT PRIMARY KEY,
			auth_method TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`); err != nil {
		t.Fatalf("create empty legacy: %v", err)
	}
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatalf("new store on empty legacy: %v", err)
	}
	if store.MigratedFromWorkspace() != "" {
		t.Error("expected no migration when legacy table is empty")
	}
	cfg, _ := store.GetConfig(context.Background())
	if cfg != nil {
		t.Errorf("expected no config post empty-legacy upgrade, got %+v", cfg)
	}
}
