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
func tiedBarrierQueue(t *testing.T, repo Repository, barrierID, targetID string) (Repository, QueueSessionIdentity, bool) {
	t.Helper()
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	barrier := &QueuedMessage{ID: barrierID, TaskID: "task", SessionID: "session", QueuedBy: QueuedByWorkflow, Content: "barrier"}
	target := sortTestEntry("A1", "A")
	target.ID = targetID
	require.NoError(t, repo.InsertForSession(ctx, identity, barrier, 0))
	require.NoError(t, repo.InsertForSession(ctx, identity, target, 0))
	tied := repo.(tiedPositionRepository)
	reserved, err := tied.ReserveHead(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, barrierID, reserved.ID)
	require.NoError(t, tied.ReorderEntriesForSession(ctx, identity, []string{targetID}))
	require.NoError(t, tied.RestoreForSession(ctx, identity, reserved, 0))
	listed, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	require.Equal(t, listed[0].Position, listed[1].Position, "precondition: the rows share a position")
	return repo, identity, listed[0].ID == barrierID
}

type tiedPositionRepository interface {
	ReserveHead(context.Context, string) (*QueuedMessage, error)
	ReorderEntriesForSession(context.Context, QueueSessionIdentity, []string) error
	RestoreForSession(context.Context, QueueSessionIdentity, *QueuedMessage, int) error
}

func tiedRepos() map[string]func(*testing.T) Repository {
	return map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() },
		"sqlite": newTestSQLiteRepo,
	}
}

func enableSortedFolds(service *Service) {
	service.SetGlobalAutoMergePolicy(AutoMergePolicy{Enabled: true})
}

// Folds follow the same total FIFO order that listing and draining use, so
// a sorted fold never crosses a barrier the queue shows between the rows.
func TestSortedFoldsFollowListedOrderAcrossTiedBarrier(t *testing.T) {
	for backend, makeRepo := range tiedRepos() {
		for name, ids := range map[string][2]string{
			"barrier id first": {"a-barrier", "z-target"},
			"target id first":  {"z-barrier", "a-target"},
		} {
			name = backend + "/" + name
			t.Run(name+"/admission", func(t *testing.T) {
				ctx := context.Background()
				repo, identity, barrierFirst := tiedBarrierQueue(t, makeRepo(t), ids[0], ids[1])
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
				repo, identity, barrierFirst := tiedBarrierQueue(t, makeRepo(t), ids[0], ids[1])
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
}

// tiedSourceQueue lists A1, barrier, A2 with the barrier and A2 sharing a
// position: the barrier is taken, the rest reordered, and the barrier restored.
func tiedSourceQueue(t *testing.T, repo Repository, barrier *QueuedMessage) (*Service, QueueSessionIdentity, *QueuedMessage) {
	t.Helper()
	ctx := context.Background()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	first := sortTestEntry("A1", "A")
	source := sortTestEntry("A2", "A")
	source.ID = "z-source"
	for _, row := range []*QueuedMessage{first, barrier, source} {
		require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
	}
	service := NewService(repo, 10, logger.Default())
	taken, found, err := service.TakeQueuedEntryForSession(ctx, identity, barrier.ID)
	require.NoError(t, err)
	require.True(t, found)
	tied := repo.(tiedPositionRepository)
	require.NoError(t, tied.ReorderEntriesForSession(ctx, identity, []string{first.ID, source.ID}))
	require.NoError(t, tied.RestoreForSession(ctx, identity, taken, 0))
	listed, err := repo.ListBySession(ctx, identity.SessionID)
	require.NoError(t, err)
	require.Equal(t, []string{first.ID, barrier.ID, source.ID}, []string{listed[0].ID, listed[1].ID, listed[2].ID})
	require.Equal(t, listed[1].Position, listed[2].Position, "precondition: barrier and source share a position")
	return service, identity, &listed[2]
}

// A source tied with the row above it must still see that row as its
// predecessor, both for an automatic fold and for manual Merge with above.
func TestFoldsTreatTiedRowAboveSourceAsPredecessor(t *testing.T) {
	for backend, makeRepo := range tiedRepos() {
		t.Run(backend+"/automatic fold stops at tied barrier", func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			barrier := &QueuedMessage{ID: "b-barrier", TaskID: "task", SessionID: "session", QueuedBy: QueuedByWorkflow, Content: "barrier"}
			service, identity, source := tiedSourceQueue(t, repo, barrier)
			enableSortedFolds(service)
			policy := service.resolveAdmissionAutoMergePolicy(ctx, &identity, identity.SessionID)
			_, merged, err := repo.(autoMergeAdmissionRepository).AutoMergeIntoAboveForSessionWithPolicy(ctx, identity, source.ID, *policy)
			require.NoError(t, err)
			require.False(t, merged)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "barrier", "A2"}, sortTestContents(rows))
		})
		t.Run(backend+"/manual merge uses tied row above", func(t *testing.T) {
			ctx := context.Background()
			repo := makeRepo(t)
			above := sortTestEntry("A-tied", "A")
			above.ID = "b-above"
			service, identity, source := tiedSourceQueue(t, repo, above)
			service.SetMergeEnabled(true)
			_, err := service.MergeIntoAboveForSession(ctx, identity, source.ID, QueuedByUser)
			require.NoError(t, err)
			rows, err := repo.ListBySession(ctx, identity.SessionID)
			require.NoError(t, err)
			require.Equal(t, []string{"A1", "A-tied\n\nA2"}, sortTestContents(rows))
		})
	}
}
