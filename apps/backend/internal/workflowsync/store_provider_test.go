package workflowsync

import (
	"context"
	"database/sql"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStore_GitLabConfigRoundtrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	req := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "acme/team/project"}
	require.NoError(t, req.Normalize())

	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	assert.Equal(t, ProviderGitLab, cfg.Provider)
	assert.Equal(t, "acme/team/project", cfg.ProjectPath)
	assert.Empty(t, cfg.RepoOwner)
	assert.Empty(t, cfg.RepoName)

	reread, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, ProviderGitLab, reread.Provider)
	assert.Equal(t, "acme/team/project", reread.ProjectPath)
}

func TestStore_GitHubConfigCarriesProvider(t *testing.T) {
	store := setupTestStore(t)
	cfg, err := store.UpsertConfigForWorkspace(context.Background(), "ws-1", testRequest())
	require.NoError(t, err)
	assert.Equal(t, ProviderGitHub, cfg.Provider)
	assert.Empty(t, cfg.ProjectPath)
}

// Switching provider replaces the row rather than leaving stale fields from
// the previous provider behind.
func TestStore_SwitchingProviderClearsPriorFields(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	gitlabReq := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "acme/project"}
	require.NoError(t, gitlabReq.Normalize())
	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", gitlabReq)
	require.NoError(t, err)

	assert.Equal(t, ProviderGitLab, cfg.Provider)
	assert.Equal(t, "acme/project", cfg.ProjectPath)
	assert.Empty(t, cfg.RepoOwner, "the GitHub owner must not survive the switch")
	assert.Empty(t, cfg.RepoName, "the GitHub name must not survive the switch")
}

func TestStore_ListConfigsCarriesProvider(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	gitlabReq := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "acme/project"}
	require.NoError(t, gitlabReq.Normalize())
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-gitlab", gitlabReq)
	require.NoError(t, err)
	_, err = store.UpsertConfigForWorkspace(ctx, "ws-github", testRequest())
	require.NoError(t, err)

	configs, err := store.ListConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 2)

	byWorkspace := map[string]*Config{}
	for _, cfg := range configs {
		byWorkspace[cfg.WorkspaceID] = cfg
	}
	assert.Equal(t, ProviderGitLab, byWorkspace["ws-gitlab"].Provider)
	assert.Equal(t, "acme/project", byWorkspace["ws-gitlab"].ProjectPath)
	assert.Equal(t, ProviderGitHub, byWorkspace["ws-github"].Provider)
}

// A database created before this feature has neither column and its rows carry
// the implicit GitHub meaning; the migration must add both and leave the row
// readable as GitHub.
func TestStore_MigratesLegacyRowToGitHub(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	// Pre-feature schema: no provider, no project_path.
	_, err = db.Exec(`
		CREATE TABLE workflow_sync_configs (
			workspace_id TEXT PRIMARY KEY,
			repo_owner TEXT NOT NULL,
			repo_name TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT 'main',
			path TEXT NOT NULL DEFAULT '',
			interval_seconds INTEGER NOT NULL DEFAULT 300,
			poll_enabled INTEGER NOT NULL DEFAULT 1,
			last_synced_at DATETIME,
			last_ok INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			last_warnings TEXT NOT NULL DEFAULT '[]',
			last_hash TEXT NOT NULL DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		INSERT INTO workflow_sync_configs
			(workspace_id, repo_owner, repo_name, branch, path, interval_seconds,
			 created_at, updated_at)
		VALUES ('ws-legacy', 'acme', 'flows', 'main', '.kandev/workflows', 300,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`)
	require.NoError(t, err)

	store, err := NewStore(db, db)
	require.NoError(t, err)

	cfg, err := store.GetConfigForWorkspace(context.Background(), "ws-legacy")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, ProviderGitHub, cfg.Provider, "a legacy row must read as GitHub")
	assert.Empty(t, cfg.ProjectPath)
	assert.Equal(t, "acme", cfg.RepoOwner)
	assert.Equal(t, "flows", cfg.RepoName)
}

// initSchema runs on every boot, so the migrations must tolerate replay.
func TestStore_SchemaReplayIsIdempotent(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	store, err := NewStore(db, db)
	require.NoError(t, err)
	req := &SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "acme/project"}
	require.NoError(t, req.Normalize())
	_, err = store.UpsertConfigForWorkspace(context.Background(), "ws-1", req)
	require.NoError(t, err)

	// Second construction against the same database re-runs initSchema.
	replayed, err := NewStore(db, db)
	require.NoError(t, err)

	cfg, err := replayed.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, ProviderGitLab, cfg.Provider)
	assert.Equal(t, "acme/project", cfg.ProjectPath)
}
