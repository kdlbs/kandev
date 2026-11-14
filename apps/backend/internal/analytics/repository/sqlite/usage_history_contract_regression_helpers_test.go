package sqlite

import (
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func freshProjectionBinding(t *testing.T, repo *Repository) string {
	t.Helper()
	tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "projection-task-B", WorkspaceID: "ws-1", Title: "B", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "projection-session-B", TaskID: "projection-task-B", StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": "projection-source-B"}}}))
	page, err := repo.ListUsageCollectionTargets(t.Context(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{TaskID: "projection-task-B", SessionID: "projection-session-B"}})
	require.NoError(t, err)
	require.True(t, page.CurrentTarget)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
	return page.Targets[0].SubjectBinding
}

func assertUnallocatedAggregateCost(t *testing.T, repo *Repository, exact pluginsdk.UsageObservationV1) {
	t.Helper()
	aggregate := exact
	aggregate.ItemID, aggregate.SourceRecordID, aggregate.WorkIdentity = "aggregate-cost", "aggregate-cost", nil
	sealContractObservation(t, &aggregate)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", aggregate).Status)
	selected := selectContractPartition(t, repo, aggregate.ItemID)
	require.Len(t, selected.Facts, 1)
	fact := selected.Facts[0]
	require.Equal(t, "mixed", fact.CostSource.ActorStatus, "aggregate source has two possible actors")
	require.Empty(t, fact.CostSource.ActorID)
	require.False(t, usageProjectionRateAvailable(fact), "mixed aggregate cost has no allocated token denominator")
	tombstone := pluginsdk.UsageObservationTombstoneV1{SubjectBinding: aggregate.SubjectBinding, ItemID: aggregate.ItemID, Revision: 2}
	var err error
	tombstone.PayloadDigest, err = tombstone.ComputePayloadDigestV1()
	require.NoError(t, err)
	result, err := repo.ReportUsageObservations(t.Context(), "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Tombstones: []pluginsdk.UsageObservationTombstoneV1{tombstone}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Status)
}
