package messagequeue

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.10
// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.11
func TestAutoSortMergeSessionSwitchCompaction(t *testing.T) {
	for backend, makeRepo := range map[string]func(*testing.T) Repository{
		"memory": func(*testing.T) Repository { return NewMemoryRepository() }, "sqlite": newTestSQLiteRepo, "postgres": newTestPostgresRepo,
	} {
		t.Run(backend, func(t *testing.T) {
			repo := makeRepo(t)
			ctx := context.Background()
			identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
			seedQueueSessionIdentity(t, repo, identity)
			service := NewService(repo, 2, logger.Default())
			_, err := service.SetSessionAutoMerge(ctx, identity, false)
			require.NoError(t, err)
			rows := []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A"), sortTestEntry("B2", "B"), {TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U1"}, {TaskID: "task", SessionID: "session", QueuedBy: QueuedByUser, Content: "U2"}, sortTestEntry("A3", "A")}
			for _, row := range rows {
				require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
			}
			require.NoError(t, repo.SetAutoRunForSession(ctx, identity, false))
			result, err := service.SetSessionAutoMerge(ctx, identity, true)
			require.NoError(t, err)
			require.Equal(t, 4, result.CompactedCount)
			after, err := repo.ListBySession(ctx, "session")
			require.NoError(t, err)
			require.Equal(t, []string{"A1\n\nA2\n\nA3", "B1\n\nB2", "U1\n\nU2"}, sortTestContents(after))
			require.Equal(t, rows[0].ID, after[0].ID)
			require.Equal(t, rows[1].Position, after[1].Position)
			autoRun, err := repo.GetAutoRun(ctx, "session")
			require.NoError(t, err)
			require.False(t, autoRun)
		})
	}
}

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.13
func TestAutoSortMergeCompactionOnlyOnSessionEffectiveRise(t *testing.T) {
	repo := NewMemoryRepository()
	identity := QueueSessionIdentity{TaskID: "task", SessionID: "session", SessionIncarnationID: "inc"}
	seedQueueSessionIdentity(t, repo, identity)
	ctx := context.Background()
	for _, row := range []*QueuedMessage{sortTestEntry("A1", "A"), sortTestEntry("B1", "B"), sortTestEntry("A2", "A")} {
		require.NoError(t, repo.InsertForSession(ctx, identity, row, 0))
	}
	service := NewService(repo, 10, logger.Default())
	for _, value := range []bool{true, false} {
		result, err := service.SetSessionAutoMerge(ctx, identity, value)
		require.NoError(t, err)
		require.Zero(t, result.CompactedCount)
		after, err := repo.ListBySession(ctx, "session")
		require.NoError(t, err)
		require.Equal(t, []string{"A1", "B1", "A2"}, sortTestContents(after))
	}
	result, err := service.SetSessionAutoMerge(ctx, identity, true)
	require.NoError(t, err)
	require.Equal(t, 1, result.CompactedCount)
	result, err = service.SetSessionAutoMerge(ctx, identity, false)
	require.NoError(t, err)
	require.Zero(t, result.CompactedCount)
	after, err := repo.ListBySession(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, []string{"A1\n\nA2", "B1"}, sortTestContents(after))
}
