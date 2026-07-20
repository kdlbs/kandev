package gitlab

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func TestStoreCreatesWorkspaceScopedConfigTable(t *testing.T) {
	store := newTestStore(t)

	var createSQL string
	if err := store.ro.Get(&createSQL, `
		SELECT sql FROM sqlite_master
		WHERE type = 'table' AND name = 'gitlab_configs'
	`); err != nil {
		t.Fatalf("read gitlab_configs schema: %v", err)
	}
	if createSQL == "" {
		t.Fatal("gitlab_configs schema is empty")
	}
}

func TestStoreConfigIsolatesWorkspacesAndPreservesScheme(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "workspace-http")
	seedWorkspace(t, store, "workspace-https")

	if err := store.UpsertConfigForWorkspace(ctx, "workspace-http", &GitLabConfig{
		Host:       "http://gitlab.internal",
		AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("upsert HTTP config: %v", err)
	}
	if err := store.UpsertConfigForWorkspace(ctx, "workspace-https", &GitLabConfig{
		Host:       "https://gitlab.example.com",
		AuthMethod: AuthMethodGLab,
	}); err != nil {
		t.Fatalf("upsert HTTPS config: %v", err)
	}

	httpConfig, err := store.GetConfigForWorkspace(ctx, "workspace-http")
	if err != nil {
		t.Fatalf("get HTTP config: %v", err)
	}
	httpsConfig, err := store.GetConfigForWorkspace(ctx, "workspace-https")
	if err != nil {
		t.Fatalf("get HTTPS config: %v", err)
	}
	if httpConfig.Host != "http://gitlab.internal" {
		t.Fatalf("HTTP host = %q", httpConfig.Host)
	}
	if httpsConfig.Host != "https://gitlab.example.com" {
		t.Fatalf("HTTPS host = %q", httpsConfig.Host)
	}
}

func TestStoreConfigRevisionChangesOnlyWithConnectionIdentity(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "workspace-a")
	if err := store.UpsertConfigForWorkspace(ctx, "workspace-a", &GitLabConfig{
		Host: "https://old.gitlab.example", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if got := configRevision(t, store, "workspace-a"); got != 1 {
		t.Fatalf("initial revision = %d, want 1", got)
	}
	updated, err := store.UpdateConfigHealthForRevision(
		ctx, "workspace-a", "alice", true, "", time.Now().UTC(), 1,
	)
	if err != nil {
		t.Fatalf("update health: %v", err)
	}
	if !updated {
		t.Fatal("health update did not match initial revision")
	}
	if got := configRevision(t, store, "workspace-a"); got != 1 {
		t.Fatalf("revision after health update = %d, want 1", got)
	}
	if err := store.UpsertConfigForWorkspace(ctx, "workspace-a", &GitLabConfig{
		Host: "https://new.gitlab.example", AuthMethod: AuthMethodPAT,
	}); err != nil {
		t.Fatalf("replace config: %v", err)
	}
	if got := configRevision(t, store, "workspace-a"); got != 2 {
		t.Fatalf("revision after config replacement = %d, want 2", got)
	}
}

func TestStoreMigratesLegacyGitLabConfigRevision(t *testing.T) {
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "legacy-gitlab.db"))
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	sqlDB := sqlx.NewDb(dbConn, "sqlite3")
	if _, err := sqlDB.Exec(`
		CREATE TABLE workspaces (id TEXT PRIMARY KEY, created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP);
		CREATE TABLE gitlab_configs (
			workspace_id TEXT PRIMARY KEY,
			host TEXT NOT NULL,
			auth_method TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			last_checked_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		INSERT INTO workspaces (id) VALUES ('workspace-a');
		INSERT INTO gitlab_configs (
			workspace_id, host, auth_method, created_at, updated_at
		) VALUES ('workspace-a', 'https://gitlab.example', 'pat', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy schema: %v", err)
	}
	store, err := NewStore(sqlDB, sqlDB)
	if err != nil {
		t.Fatalf("migrate legacy store: %v", err)
	}
	if got := configRevision(t, store, "workspace-a"); got != 1 {
		t.Fatalf("migrated revision = %d, want 1", got)
	}
}

func configRevision(t *testing.T, store *Store, workspaceID string) int64 {
	t.Helper()
	var revision int64
	if err := store.ro.Get(&revision, `SELECT revision FROM gitlab_configs WHERE workspace_id = ?`, workspaceID); err != nil {
		t.Fatalf("read config revision: %v", err)
	}
	return revision
}
