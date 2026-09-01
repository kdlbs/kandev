package configsync

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	// Pin to one connection: each new connection to an in-memory SQLite DB
	// gets its own isolated database, which makes pooled access flaky.
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	require.NoError(t, err)
	return store
}

func testRequest() *SetConfigRequest {
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "kandev-config"}
	if err := req.Normalize(); err != nil {
		panic(err)
	}
	return req
}

func TestStore_GetConfigForWorkspace_MissingReturnsNil(t *testing.T) {
	store := setupTestStore(t)
	cfg, err := store.GetConfigForWorkspace(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)
}

func TestStore_UpsertAndGetConfigForWorkspace(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	req := testRequest()

	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "ws-1", cfg.WorkspaceID)
	assert.Equal(t, ProviderGitHub, cfg.Provider)
	assert.Equal(t, "acme", cfg.RepoOwner)
	assert.Equal(t, "kandev-config", cfg.RepoName)
	assert.Equal(t, DefaultBranch, cfg.Branch)
	assert.False(t, cfg.LastOk)
	assert.True(t, cfg.PollEnabled)

	got, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, cfg.RepoOwner, got.RepoOwner)
}

func TestStore_UpsertConfigForWorkspace_ResetsStatusOnReplace(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	req := testRequest()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	require.NoError(t, store.RecordSyncStatus(ctx, "ws-1", true, "", nil, "hash-1", time.Now().UTC()))

	req2 := &SetConfigRequest{RepoOwner: "acme", RepoName: "other-repo"}
	require.NoError(t, req2.Normalize())
	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req2)
	require.NoError(t, err)
	assert.Equal(t, "other-repo", cfg.RepoName)
	assert.False(t, cfg.LastOk)
	assert.Nil(t, cfg.LastSyncedAt)
	assert.Empty(t, cfg.LastHash)
}

func TestStore_RecordSyncStatus(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	at := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.RecordSyncStatus(ctx, "ws-1", true, "", []string{"warn1"}, "abc123", at))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.True(t, cfg.LastOk)
	assert.Equal(t, "abc123", cfg.LastHash)
	assert.Equal(t, []string{"warn1"}, cfg.LastWarnings)
	require.NotNil(t, cfg.LastSyncedAt)
}

func TestStore_ListConfigs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-a", testRequest())
	require.NoError(t, err)
	_, err = store.UpsertConfigForWorkspace(ctx, "ws-b", testRequest())
	require.NoError(t, err)

	configs, err := store.ListConfigs(ctx)
	require.NoError(t, err)
	assert.Len(t, configs, 2)
}

func TestStore_DeleteConfigForWorkspace_AlsoDeletesManifest(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "agent", "ceo", "agent-id-1", "agents/ceo.yml"))

	require.NoError(t, store.DeleteConfigForWorkspace(ctx, "ws-1"))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)

	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestStore_ManifestUpsertUpdateAndDelete(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "skill", "reviewer", "skill-1", "skills/reviewer/SKILL.md"))
	entries, err := store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "skill-1", entries[0].EntityID)
	assert.Equal(t, "skills/reviewer/SKILL.md", entries[0].SourcePath)

	// Upsert again with a new entity id (e.g. gone-out-of-band recreate).
	require.NoError(t, store.UpsertManifestEntry(ctx, "ws-1", "skill", "reviewer", "skill-2", "skills/reviewer/SKILL.md"))
	entries, err = store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "skill-2", entries[0].EntityID)

	require.NoError(t, store.DeleteManifestEntry(ctx, "ws-1", "skill", "reviewer"))
	entries, err = store.ListManifest(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Deleting a missing entry is a no-op.
	require.NoError(t, store.DeleteManifestEntry(ctx, "ws-1", "skill", "reviewer"))
}
