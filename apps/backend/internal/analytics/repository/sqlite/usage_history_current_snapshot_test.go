package sqlite

import (
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryCurrentTargetKeepsImportSnapshotFrozen(t *testing.T) {
	runCurrentTargetFrozenSnapshot(t, false)
}

func TestUsageHistoryPostgresCurrentTargetKeepsImportSnapshotFrozen(t *testing.T) {
	runCurrentTargetFrozenSnapshot(t, true)
}

func runCurrentTargetFrozenSnapshot(t *testing.T, postgres bool) {
	repo, tasks := currentTargetDatabaseFixture(t, postgres)
	createCurrentTargetSession(t, tasks, "other-task", "other-session", "other-transcript")
	query := pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", Limit: 1}
	first := preparedCurrentFixturePage(t, repo, query)
	require.Len(t, first.Targets, 1)
	require.True(t, first.HasMore)
	require.False(t, first.CurrentTarget)
	require.NoError(t, tasks.UpdateSessionMetadata(t.Context(), "session-1", map[string]any{"acp": map[string]any{"session_id": "updated-transcript"}}))
	current := currentTargetForTest(t, repo, "collector-a", "ws-1", currentUsageTargetSelector{SessionID: "session-1"})
	require.Equal(t, "updated-transcript", current.ExternalSessionID)
	createCurrentTargetSession(t, tasks, "later-task", "later-session", "later-transcript")
	query.SnapshotID = first.SnapshotID
	before, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", query)
	require.NoError(t, err)
	require.Equal(t, first, before, "point refresh must not mutate frozen import membership or metadata")
	query.Cursor = first.NextCursor
	second, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", query)
	require.NoError(t, err)
	require.Len(t, second.Targets, 1)
	require.False(t, second.HasMore)
	require.Equal(t, first.SnapshotUpperBound, second.SnapshotUpperBound)
	require.NotEqual(t, *first.Targets[0].SessionID, *second.Targets[0].SessionID)
	require.NotEqual(t, "later-session", *second.Targets[0].SessionID)
}

func preparedCurrentFixturePage(t *testing.T, repo *Repository, query pluginsdk.UsageCollectionTargetQueryV1) pluginsdk.UsageCollectionTargetPageV1 {
	t.Helper()
	for range 4 {
		page, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", query)
		require.NoError(t, err)
		if !page.Preparing {
			return page
		}
		require.Empty(t, page.Targets)
		require.Empty(t, page.SnapshotID)
		require.Empty(t, page.NextCursor)
	}
	t.Fatal("two-session fixture preparation did not finish its bounded pages")
	return pluginsdk.UsageCollectionTargetPageV1{}
}
