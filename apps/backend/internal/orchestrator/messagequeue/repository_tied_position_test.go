package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// tiedBarrierQueue leaves a workflow barrier and agent row A1 sharing
// position 1 through reserve, reorder, and restore, then reports whether the
// listed FIFO order puts the barrier before A1.
func tiedBarrierQueue(t *testing.T, barrierID, targetID string) (*sqliteRepository, QueueSessionIdentity, bool) {
	t.Helper()
	ctx := context.Background()
	repo := newTestSQLiteRepo(t).(*sqliteRepository)
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	barrier := &QueuedMessage{ID: barrierID, TaskID: "task", SessionID: "session", QueuedBy: QueuedByWorkflow, Content: "barrier"}
	target := sortTestEntry("A1", "A")
	target.ID = targetID
	require.NoError(t, repo.InsertForSession(ctx, identity, barrier, 0))
	require.NoError(t, repo.InsertForSession(ctx, identity, target, 0))
	reserved, err := repo.ReserveHead(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, barrierID, reserved.ID)
	require.NoError(t, repo.ReorderEntriesForSession(ctx, identity, []string{targetID}))
	require.NoError(t, repo.RestoreForSession(ctx, identity, reserved, 0))
	listed, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.Equal(t, listed[0].Position, listed[1].Position, "precondition: the rows share a position")
	return repo, identity, listed[0].ID == barrierID
}

func enableSortedFolds(service *Service) {
	service.SetGlobalQueuePolicy(GlobalQueuePolicy{
		AutoMerge: AutoMergePolicy{Enabled: true}, AutoSortMerge: AutoSortMergePolicy{Enabled: true},
	})
}

// Folds follow the same total FIFO order that listing and draining use, so
// a sorted fold never crosses a barrier the queue shows between the rows.
func TestSortedFoldsFollowListedOrderAcrossTiedBarrier(t *testing.T) {
	for name, ids := range map[string][2]string{
		"barrier id first": {"a-barrier", "z-target"},
		"target id first":  {"z-barrier", "a-target"},
	} {
		t.Run(name+"/admission", func(t *testing.T) {
			ctx := context.Background()
			repo, identity, barrierFirst := tiedBarrierQueue(t, ids[0], ids[1])
			service := NewService(repo, 2, logger.Default())
			enableSortedFolds(service)
			_, err := service.QueueMessageWithMetadataForSession(ctx, identity, "A2", "", QueuedByAgent, false, nil,
				map[string]any{MetadataSenderTaskID: "A"})
			rows, listErr := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, listErr)
			if barrierFirst {
				require.NoError(t, err)
				require.Equal(t, []string{"barrier", "A1\n\nA2"}, sortTestContents(rows))
				return
			}
			require.ErrorIs(t, err, ErrQueueFull)
			require.Equal(t, []string{"A1", "barrier"}, sortTestContents(rows))
		})
		t.Run(name+"/compaction", func(t *testing.T) {
			ctx := context.Background()
			repo, identity, barrierFirst := tiedBarrierQueue(t, ids[0], ids[1])
			require.NoError(t, repo.InsertForSession(ctx, identity, sortTestEntry("A2", "A"), 0))
			service := NewService(repo, 10, logger.Default())
			enableSortedFolds(service)
			_, err := service.SetSessionAutoMerge(ctx, identity, false)
			require.NoError(t, err)
			_, err = service.SetSessionAutoMerge(ctx, identity, true)
			require.NoError(t, err)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			if barrierFirst {
				require.Equal(t, []string{"barrier", "A1\n\nA2"}, sortTestContents(rows))
				return
			}
			require.Equal(t, []string{"A1", "barrier", "A2"}, sortTestContents(rows))
		})
	}
}
