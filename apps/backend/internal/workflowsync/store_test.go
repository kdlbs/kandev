package workflowsync

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/github"
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
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "flows"}
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

func TestStore_UpsertAndGetRoundtrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	assert.Equal(t, "acme", cfg.RepoOwner)
	assert.Equal(t, "flows", cfg.RepoName)
	assert.Equal(t, DefaultBranch, cfg.Branch)
	assert.Equal(t, DefaultPath, cfg.Path)
	assert.Equal(t, DefaultIntervalSeconds, cfg.IntervalSeconds)
	assert.Nil(t, cfg.LastSyncedAt)
	assert.False(t, cfg.LastOk)
}

func TestStore_UpsertResetsSyncStatus(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	require.NoError(t, store.RecordSyncStatus(ctx, "ws-1", true, "", []string{"w1"}, "hash-1", time.Now().UTC()))

	req := testRequest()
	req.RepoName = "other"
	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	assert.Equal(t, "other", cfg.RepoName)
	assert.Nil(t, cfg.LastSyncedAt, "changing the config resets sync status")
	assert.Empty(t, cfg.LastHash)
	assert.Empty(t, cfg.LastWarnings)
}

func TestStore_RecordSyncStatusRoundtrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	at := time.Now().UTC().Truncate(time.Second)
	warnings := []string{"workflow \"X\" not updated", "flows/broken.yml: bad yaml"}
	require.NoError(t, store.RecordSyncStatus(ctx, "ws-1", false, "boom", warnings, "hash-2", at))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg.LastSyncedAt)
	assert.False(t, cfg.LastOk)
	assert.Equal(t, "boom", cfg.LastError)
	assert.Equal(t, warnings, cfg.LastWarnings)
	assert.Equal(t, "hash-2", cfg.LastHash)
}

func TestStore_ListConfigs(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-b", testRequest())
	require.NoError(t, err)
	_, err = store.UpsertConfigForWorkspace(ctx, "ws-a", testRequest())
	require.NoError(t, err)

	configs, err := store.ListConfigs(ctx)
	require.NoError(t, err)
	require.Len(t, configs, 2)
	assert.Equal(t, "ws-a", configs[0].WorkspaceID)
	assert.Equal(t, "ws-b", configs[1].WorkspaceID)
}

func TestStore_DeleteConfigForWorkspace(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	require.NoError(t, store.DeleteConfigForWorkspace(ctx, "ws-1"))
	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Nil(t, cfg)

	// Deleting a missing config is a no-op.
	require.NoError(t, store.DeleteConfigForWorkspace(ctx, "ws-1"))
}

func TestStore_PollEnabledRoundtrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	assert.True(t, cfg.PollEnabled, "polling defaults to enabled")

	req := testRequest()
	disabled := false
	req.PollEnabled = &disabled
	cfg, err = store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	assert.False(t, cfg.PollEnabled)
}

func TestStore_WorkflowSyncRecoveryColumnsExist(t *testing.T) {
	store := setupTestStore(t)
	rows, err := store.db.Query(`PRAGMA table_info(workflow_sync_configs)`)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	columns := map[string]bool{}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		require.NoError(t, rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey))
		columns[name] = true
	}
	require.NoError(t, rows.Err())
	for _, name := range []string{
		"consecutive_failures", "next_attempt_at", "last_error_class",
		"poll_suspended", "poll_suspension_reason",
	} {
		assert.True(t, columns[name], "missing workflow-sync recovery column %s", name)
	}
}

func TestStore_RecordSyncFailureRoundtripAndConfigSaveReset(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	now := time.Date(2026, 8, 29, 7, 0, 0, 0, time.UTC)
	nextAttempt := now.Add(5 * time.Minute)
	require.NoError(t, store.RecordSyncFailure(ctx, "ws-1", "rate limited", failureDirective{
		class: string(github.FailureSecondaryRateLimit), consecutive: 3,
		nextAttemptAt: &nextAttempt,
	}, now))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, 3, cfg.ConsecutiveFailures)
	require.NotNil(t, cfg.NextAttemptAt)
	assert.Equal(t, nextAttempt, *cfg.NextAttemptAt)
	assert.Equal(t, string(github.FailureSecondaryRateLimit), cfg.LastErrorClass)

	cfg, err = store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)
	assert.Zero(t, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.NextAttemptAt)
	assert.Empty(t, cfg.LastErrorClass)
	assert.False(t, cfg.PollSuspended)
	assert.Empty(t, cfg.PollSuspensionReason)
}
