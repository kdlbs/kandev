package messagequeue

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListSessionsWithPendingListsEachLiveSessionOnce(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			if _, ok := repo.(*sqliteRepository); ok {
				seedLiveSessions(t, repo, "s1", "s2")
			}
			for _, row := range []struct{ session, body string }{{"s1", "a"}, {"s1", "b"}, {"s2", "c"}} {
				require.NoError(t, repo.Insert(ctx, &QueuedMessage{SessionID: row.session, TaskID: "t1", Content: row.body, QueuedBy: "user-1"}, 10))
			}
			lister, ok := repo.(pendingSessionLister)
			require.True(t, ok, "%T must list sessions with pending rows", repo)
			sessions, err := lister.ListSessionsWithPending(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"s1", "s2"}, sessions)
		})
	}
}

func TestListSessionsWithPendingSkipsRowsOfDeletedSessions(t *testing.T) {
	ctx := context.Background()
	repo := newTestSQLiteRepo(t)
	seedLiveSessions(t, repo, "live")
	require.NoError(t, repo.Insert(ctx, &QueuedMessage{SessionID: "live", TaskID: "t1", Content: "a", QueuedBy: "user-1"}, 10))
	require.NoError(t, repo.Insert(ctx, &QueuedMessage{SessionID: "deleted", TaskID: "t1", Content: "b", QueuedBy: "user-1"}, 10))

	sessions, err := repo.(pendingSessionLister).ListSessionsWithPending(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"live"}, sessions)
}

func TestListSessionsWithPendingSkipsReservedRows(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			ctx := context.Background()
			repo := factory.new(t)
			if _, ok := repo.(*sqliteRepository); ok {
				seedLiveSessions(t, repo, "reserved-only", "mixed")
			}
			reserved := map[string]interface{}{MetadataLifecycleReserved: true}
			for _, row := range []struct {
				session  string
				metadata map[string]interface{}
			}{{"reserved-only", reserved}, {"mixed", reserved}, {"mixed", nil}} {
				require.NoError(t, repo.Insert(ctx, &QueuedMessage{
					SessionID: row.session, TaskID: "t1", Content: "body", QueuedBy: "user-1", Metadata: row.metadata,
				}, 10))
			}
			sessions, err := repo.(pendingSessionLister).ListSessionsWithPending(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"mixed"}, sessions)
		})
	}
}
