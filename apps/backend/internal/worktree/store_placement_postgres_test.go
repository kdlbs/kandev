package worktree

import (
	"testing"

	"github.com/stretchr/testify/require"

	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

func TestSQLiteStore_CreateWorktreePlacementOnPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	store, err := NewSQLiteStore(db, db)
	require.NoError(t, err)
	testCreateWorktreePlacement(t, store)
}

func TestSQLiteStore_CreateWorktreeReactivationPlacementOnPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	_, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	store, err := NewSQLiteStore(db, db)
	require.NoError(t, err)
	testCreateWorktreeReactivationPlacement(t, store)
}
