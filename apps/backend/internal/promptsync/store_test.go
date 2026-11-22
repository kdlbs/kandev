package promptsync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/reposync"
)

func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "promptsync.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func newTestStore(t *testing.T, database *sqlx.DB) *Store {
	t.Helper()
	store, err := NewStore(database, database)
	require.NoError(t, err)
	return store
}

func testRequest() *SetConfigRequest {
	req := &SetConfigRequest{WorkspaceID: "ws-1", Provider: reposync.ProviderGitLab, ProjectPath: "acme/eng"}
	if err := req.Normalize(); err != nil {
		panic(err)
	}
	return req
}

func TestStoreConfigLifecycle(t *testing.T) {
	database := openTestDB(t)
	store := newTestStore(t, database)
	_, err := NewStore(database, database)
	require.NoError(t, err, "schema init must be replayable")
	ctx := context.Background()

	cfg, err := store.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, cfg)

	cfg, err = store.Upsert(ctx, testRequest())
	require.NoError(t, err)
	assert.Equal(t, "ws-1", cfg.WorkspaceID)
	assert.Equal(t, DefaultPath, cfg.Path)
	assert.True(t, cfg.PollEnabled)

	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	next := now.Add(time.Minute)
	require.NoError(t, store.RecordFailure(ctx, "GitLab request failed with HTTP status 500", reposync.FailureDirective{
		Class: "transient", Consecutive: 2, NextAttemptAt: &next, CircuitClass: authcircuit.FailureClassTransient,
	}, now))
	cfg, err = store.Get(ctx)
	require.NoError(t, err)
	assert.False(t, cfg.LastOk)
	assert.Equal(t, 2, cfg.ConsecutiveFailures)
	require.NotNil(t, cfg.NextAttemptAt)
	assert.True(t, next.Equal(*cfg.NextAttemptAt))

	require.NoError(t, store.RecordSuccess(ctx, []string{"warn"}, "hash", now))
	cfg, err = store.Get(ctx)
	require.NoError(t, err)
	assert.True(t, cfg.LastOk)
	assert.Equal(t, []string{"warn"}, cfg.LastWarnings)
	assert.Zero(t, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.NextAttemptAt)

	require.NoError(t, store.RecordFailure(ctx, "suspended", reposync.FailureDirective{
		Class: "invalid_credentials", Consecutive: 1, Suspended: true, SuspensionReason: "bad token",
	}, now))
	_, err = store.Upsert(ctx, testRequest())
	require.NoError(t, err)
	cfg, err = store.Get(ctx)
	require.NoError(t, err)
	assert.False(t, cfg.PollSuspended, "saving the config clears recovery state")
	assert.Nil(t, cfg.LastSyncedAt)

	require.NoError(t, store.Delete(ctx))
	cfg, err = store.Get(ctx)
	require.NoError(t, err)
	assert.Nil(t, cfg)
}
