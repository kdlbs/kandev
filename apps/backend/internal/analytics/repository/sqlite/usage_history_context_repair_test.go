package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryContextRepairPreservesUnrelatedPublication(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(fmt.Sprintf("changed=%t", changed), func(t *testing.T) {
			repo, api := historyRetainedContractFixture(t)
			_, err := repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"unrelated-history"}}' WHERE id='session-1'`)
			require.NoError(t, err)
			page, err := api.ListUsageCollectionTargets(t.Context(), "repair-scale", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			require.Len(t, page.Targets, 1)
			const history = 2*maxSessionUsageBatch + 1
			seedContextRepairHistory(t, repo, page.Targets[0].SubjectBinding, history)
			createContextRepairSession(t, repo)
			if changed {
				_, err = api.ListUsageCollectionTargets(t.Context(), "repair-scale", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
				require.NoError(t, err)
				_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"affected-after"}}' WHERE id='repair-session'`)
				require.NoError(t, err)
			}
			var before int64
			require.NoError(t, repo.db.Get(&before, `SELECT work_version FROM usage_scopes WHERE workspace_id='ws-1'`))
			_, err = repo.db.Exec(`CREATE TABLE repair_test_updates(id TEXT NOT NULL)`)
			require.NoError(t, err)
			for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
				_, err = repo.db.Exec(`CREATE TRIGGER audit_` + table + ` AFTER UPDATE OF selection_partition,selection_coverage,selection_bundle ON ` + table + ` WHEN OLD.session_id='session-1' BEGIN INSERT INTO repair_test_updates VALUES (OLD.id); END`)
				require.NoError(t, err)
			}
			_, err = api.ListUsageCollectionTargets(t.Context(), "repair-scale", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			var after int64
			require.NoError(t, repo.db.Get(&after, `SELECT work_version FROM usage_scopes WHERE workspace_id='ws-1'`))
			assert.Equal(t, before, after, "a runtime context change must not invalidate the workspace work-key version")
			filter := models.SessionUsageFilter{WorkspaceID: "ws-1", SessionIDs: []string{"session-1"}, GroupBy: "model"}
			report, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			assert.Equal(t, "ready", report.ProjectionStatus, "unrelated complete usage must stay readable while context work is pending")
			assert.Equal(t, models.Int64(history), report.Totals.TotalTokens)
			for range 8 {
				_, err := repo.RunUsageHistoryProjectionBatch(t.Context(), 17)
				require.NoError(t, err)
			}
			var updates int
			require.NoError(t, repo.db.Get(&updates, `SELECT COUNT(*) FROM repair_test_updates`))
			assert.Zero(t, updates, "new/changed session lookup cannot rewrite the 401 unrelated source rows")
		})
	}
}

func seedContextRepairHistory(t *testing.T, repo *Repository, binding string, count int) {
	t.Helper()
	for start := 0; start < count; start += maxSessionUsageBatch {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV2}
		for i := start; i < min(start+maxSessionUsageBatch, count); i++ {
			item := contractObservation(t, binding, fmt.Sprintf("prior-%04d", i), 1, "prior-model", 1)
			item.ModelProvider = stringPtr("provider")
			item.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "prior-records", SegmentID: item.ItemID}
			sealContractObservation(t, &item)
			batch.Observations = append(batch.Observations, item)
		}
		result, err := repo.ReportUsageObservations(t.Context(), "repair-scale", batch)
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
	}
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
}

func createContextRepairSession(t *testing.T, repo *Repository) {
	t.Helper()
	tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	at := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "repair-task", WorkspaceID: "ws-1", Title: "Affected", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{
		ID: "repair-session", TaskID: "repair-task", StartedAt: at,
		Metadata: map[string]any{"acp": map[string]any{"session_id": "affected-before"}},
	}))
}
