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

	"github.com/kandev/kandev/internal/common/authcircuit"
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
	failAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.RecordSyncStatus(
		ctx, "ws-1", false, "boom", nil, "", failAt,
		authcircuit.State{FailureClass: authcircuit.FailureClassConfig, ConsecutiveFailures: 3, NextRetryAt: &failAt},
	))

	req := testRequest()
	req.RepoName = "other"
	cfg, err := store.UpsertConfigForWorkspace(ctx, "ws-1", req)
	require.NoError(t, err)
	assert.Equal(t, "other", cfg.RepoName)
	assert.Nil(t, cfg.LastSyncedAt, "changing the config resets sync status")
	assert.Empty(t, cfg.LastHash)
	assert.Empty(t, cfg.LastWarnings)
	assert.Empty(t, cfg.FailureClass, "an explicit config change always resets an open circuit")
	assert.Zero(t, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.NextRetryAt)
	assert.Equal(t, req.fingerprint(), cfg.ConfigFingerprint)
}

func TestStore_RecordSyncStatusRoundtrip(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	at := time.Now().UTC().Truncate(time.Second)
	warnings := []string{"workflow \"X\" not updated", "flows/broken.yml: bad yaml"}
	require.NoError(t, store.RecordSyncStatus(
		ctx, "ws-1", false, "boom", warnings, "hash-2", at,
		authcircuit.State{FailureClass: authcircuit.FailureClassTransient, ConsecutiveFailures: 1, NextRetryAt: &at},
	))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.NotNil(t, cfg.LastSyncedAt)
	assert.False(t, cfg.LastOk)
	assert.Equal(t, "boom", cfg.LastError)
	assert.Equal(t, warnings, cfg.LastWarnings)
	assert.Equal(t, "hash-2", cfg.LastHash)
	assert.Equal(t, authcircuit.FailureClassTransient, cfg.FailureClass)
	assert.Equal(t, 1, cfg.ConsecutiveFailures)
	require.NotNil(t, cfg.NextRetryAt)
	assert.True(t, at.Equal(*cfg.NextRetryAt))
}

// TestStore_RecordSyncStatus_SuccessClearsCircuit confirms a subsequent
// successful sync clears a previously-recorded circuit-open state, matching
// authcircuit.State.RecordSuccess's contract.
func TestStore_RecordSyncStatus_SuccessClearsCircuit(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	failAt := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.RecordSyncStatus(
		ctx, "ws-1", false, "boom", nil, "", failAt,
		authcircuit.State{FailureClass: authcircuit.FailureClassAuth, ConsecutiveFailures: 2, NextRetryAt: &failAt},
	))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	require.Equal(t, authcircuit.FailureClassAuth, cfg.FailureClass)

	okAt := failAt.Add(time.Minute)
	require.NoError(t, store.RecordSyncStatus(ctx, "ws-1", true, "", nil, "hash-ok", okAt, authcircuit.State{}))

	cfg, err = store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, cfg.FailureClass)
	assert.Zero(t, cfg.ConsecutiveFailures)
	assert.Nil(t, cfg.NextRetryAt)
}

// TestStore_RecordCircuitState confirms the standalone circuit-only writer
// used by the credential-fingerprint reset path persists exactly the given
// state, including the fingerprint (which RecordSyncStatus never touches).
func TestStore_RecordCircuitState(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	_, err := store.UpsertConfigForWorkspace(ctx, "ws-1", testRequest())
	require.NoError(t, err)

	require.NoError(t, store.RecordCircuitState(ctx, "ws-1", authcircuit.State{
		FailureClass:        authcircuit.FailureClassNone,
		ConsecutiveFailures: 0,
		Fingerprint:         "active:3",
	}))

	cfg, err := store.GetConfigForWorkspace(ctx, "ws-1")
	require.NoError(t, err)
	assert.Empty(t, cfg.FailureClass)
	assert.Equal(t, "active:3", cfg.CredentialFingerprint)
}

// TestStore_addCircuitColumns_Idempotent confirms re-running the migration
// (as happens on every backend boot) is a no-op, matching
// addPollEnabledColumn/addProviderColumns.
func TestStore_addCircuitColumns_Idempotent(t *testing.T) {
	store := setupTestStore(t)
	require.NoError(t, store.addCircuitColumns())
	require.NoError(t, store.addCircuitColumns())
}

func TestStore_addCircuitColumns_UsesPostgresTimestampType(t *testing.T) {
	rawDB, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	rawDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(rawDB, "pgx")
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TABLE workflow_sync_configs (workspace_id TEXT PRIMARY KEY)`)
	require.NoError(t, err)

	store := &Store{db: db, ro: db}
	require.NoError(t, store.addCircuitColumns())

	var columnType string
	err = db.Get(&columnType, `SELECT type FROM pragma_table_info('workflow_sync_configs') WHERE name = 'next_retry_at'`)
	require.NoError(t, err)
	assert.Equal(t, "TIMESTAMPTZ", columnType)
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
