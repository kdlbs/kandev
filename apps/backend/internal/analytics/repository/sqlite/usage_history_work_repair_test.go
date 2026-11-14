package sqlite

import (
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/db"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistorySharedTranscriptRepairsPublishedSubjectKeys(t *testing.T) {
	repo, api := historyContractFixture(t)
	sharedWorkSession(t, repo)
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, target := range page.Targets {
		id := "old-a"
		if i == 1 {
			id = "old-b"
		}
		require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", contractObservation(t, target.SubjectBinding, id, 1, "model", 100)).Status)
	}
	// These populated indexes model rows admitted by the previous implementation.
	_, err = repo.db.Exec(`UPDATE session_usage_measurements SET selection_partition = subject_id`)
	require.NoError(t, err)
	hasVersion, err := db.ColumnExistsContext(t.Context(), repo.db, usageMeasurementsTable, "selection_version")
	require.NoError(t, err)
	if hasVersion {
		_, err = repo.db.Exec(`UPDATE session_usage_measurements SET selection_version = 0`)
		require.NoError(t, err)
	}
	// Reopening must discover the repair even when no collector revisits the rows.
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	completeFixtureProjection(t, restarted)
	facts, _, err := restarted.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, facts, 1, "populated subject-only indexes need repair, not just newly admitted observations")
	require.EqualValues(t, 100, *facts[0].Measurement.TotalTokens)
	var raw int
	require.NoError(t, repo.db.Get(&raw, `SELECT COUNT(*) FROM session_usage_measurements`))
	require.Equal(t, 2, raw, "repair preserves source evidence")
}

func TestUsageHistoryOneSubjectDistinctProvenTranscriptContexts(t *testing.T) {
	repo, api := historyContractFixture(t)
	first, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	prior := contractObservation(t, first.Targets[0].SubjectBinding, "original-context", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", prior).Status)
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"distinct-context"}}' WHERE id='session-1'`)
	require.NoError(t, err)
	next, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.NotEqual(t, first.Targets[0].SubjectBinding, next.Targets[0].SubjectBinding)
	current := contractObservation(t, next.Targets[0].SubjectBinding, "distinct-context", 1, "model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", current).Status)
	completeFixtureProjection(t, repo)
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, facts, 2)
	require.EqualValues(t, 140, *facts[0].Measurement.TotalTokens+*facts[1].Measurement.TotalTokens)
}

func TestUsageHistoryCopiedTranscriptContextDoesNotBecomeAdditive(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	prior := contractObservation(t, page.Targets[0].SubjectBinding, "original-context", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", prior).Status)
	sharedWorkSession(t, repo)
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"forked-context"},"codex_fork_source_session_id":"session-1"}' WHERE id='session-shared'`)
	require.NoError(t, err)
	page, err = api.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for _, target := range page.Targets {
		if target.SessionID != nil && *target.SessionID == "session-shared" {
			copy := contractObservation(t, target.SubjectBinding, "copied-history", 1, "model", 130)
			require.Equal(t, "rejected", reportContractObservation(t, api, "collector-a", copy).Status)
		}
	}
	completeFixtureProjection(t, repo)
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.EqualValues(t, 100, *facts[0].Measurement.TotalTokens)
}
