package messagequeue

import (
	"context"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func TestRepositories_PurgeTaskRemovesPendingMove(t *testing.T) {
	factories := []struct {
		name string
		new  func(*testing.T) Repository
	}{
		{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
		{name: "sqlite", new: newTestSQLiteRepo},
	}
	for _, factory := range factories {
		t.Run(factory.name, func(t *testing.T) {
			repo := factory.new(t)
			ctx := context.Background()
			require.NoError(t, repo.SetPendingMove(ctx, "session-purge", &PendingMove{
				TaskID: "task-purge",
			}))

			_, err := repo.PurgeTask(ctx, "task-purge")
			require.NoError(t, err)

			move, err := repo.GetPendingMove(ctx, "session-purge")
			require.NoError(t, err)
			require.Nil(t, move)
		})
	}
}

func TestSQLitePurgeTaskInTransactionRemovesPendingMove(t *testing.T) {
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	ctx := context.Background()
	require.NoError(t, repo.SetPendingMove(ctx, "session-purge-tx", &PendingMove{
		TaskID: "task-purge-tx",
	}))

	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	removed, err := PurgeTaskInTransaction(ctx, tx, repo.db, "task-purge-tx", []string{"session-purge-tx"})
	require.NoError(t, err)
	require.Zero(t, removed)
	require.NoError(t, tx.Commit())

	move, err := repo.GetPendingMove(ctx, "session-purge-tx")
	require.NoError(t, err)
	require.Nil(t, move)
}
