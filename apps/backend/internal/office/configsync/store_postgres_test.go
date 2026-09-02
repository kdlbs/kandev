package configsync

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPostgresNewStore_SchemaInit is the PostgreSQL twin of setupTestStore:
// createTablesSQL previously declared its timestamp columns DATETIME, which
// is not a valid PostgreSQL type and made NewStore fail schema init on any
// Postgres-backed deployment (silently, since the caller treats init failure
// as non-fatal and just leaves config sync disabled). This exercises schema
// init plus a config/manifest round trip against a real Postgres backend so
// a dialect regression fails loudly instead of only degrading a feature with
// no error surfaced to the operator. Skips unless KANDEV_TEST_POSTGRES_DSN
// is set.
func TestPostgresNewStore_SchemaInit(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	ctx := context.Background()

	store, err := NewStore(db, db)
	require.NoError(t, err)

	req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "acme", RepoName: "kandev-config"}
	require.NoError(t, req.Normalize())

	cfg, err := store.UpsertConfigForWorkspace(ctx, "pg-ws-1", req)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "acme", cfg.RepoOwner)
	assert.Nil(t, cfg.LastSyncedAt)

	at := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.RecordSyncStatus(ctx, "pg-ws-1", true, "", []string{"warn1"}, "abc123", at))

	got, err := store.GetConfigForWorkspace(ctx, "pg-ws-1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.LastOk)
	require.NotNil(t, got.LastSyncedAt)

	require.NoError(t, store.UpsertManifestEntry(ctx, "pg-ws-1", "agent", "ceo", "agent-1", "agents/ceo.yml"))
	entries, err := store.ListManifest(ctx, "pg-ws-1")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "agent-1", entries[0].EntityID)
}
