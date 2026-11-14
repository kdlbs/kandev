package sqlite

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryRuntimeAccountingNewScopeAfterBackfill(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo, tasks := currentTargetDatabaseFixture(t, postgres)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			workspace := "after-backfill-workspace"
			require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: workspace, Name: "After backfill", OrgID: "org-a"}))
			createCurrentTargetSessionInWorkspace(t, tasks, workspace, "after-backfill-task", "after-backfill-session", "after-backfill-thread")
			page, err := repo.ListUsageCollectionTargets(t.Context(), "runtime-collector", pluginsdk.UsageCollectionTargetQueryV1{
				WorkspaceID: workspace, CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{SessionID: "after-backfill-session"},
			})
			require.NoError(t, err)
			require.True(t, page.CurrentTarget)
			require.Len(t, page.Targets, 1)
			require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
			item := contractObservation(t, page.Targets[0].SubjectBinding, "after-backfill-item", 1, "model", 100)
			item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(0), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
			item.Quality.CostCoverageComplete = true
			sealContractObservation(t, &item)
			require.Equal(t, "accepted", reportContractObservation(t, repo, "runtime-collector", item).Status)
			filter := models.SessionUsageFilter{WorkspaceID: workspace, GroupBy: "model"}
			before, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.True(t, before.ProjectionPending)
			// Startup migration is already complete. These are only ordinary
			// projection ticks; no second Backfill or native admission is used.
			completeFixtureProjection(t, repo)
			after, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.False(t, after.ProjectionPending, "new scopes need runtime source maintenance after startup migration")
			require.Equal(t, "ready", after.ProjectionStatus)
			require.Equal(t, models.Int64(100), after.Totals.TotalTokens)
			require.Equal(t, models.Int64(0), after.Totals.CostSubcents)
			require.False(t, after.HasNativeHistory)
			var native int
			require.NoError(t, repo.db.Get(&native, `SELECT COUNT(*) FROM task_usage_events`))
			require.Zero(t, native, "an external scope must not need a native event to become ready")
		})
	}
}

func TestUsageHistoryRuntimeAccountingPartialRepairPublicationBarrier(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo, tasks := currentTargetDatabaseFixture(t, postgres)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: nativeTurnsWorkspace, Name: "Runtime native", OrgID: "org-a"}))
			createCurrentTargetSessionInWorkspace(t, tasks, nativeTurnsWorkspace, nativeTurnsTask, nativeTurnsSession, "provider-thread")
			turns := []*taskmodels.Turn{{ID: "native-root-a", TaskID: nativeTurnsTask, TaskSessionID: nativeTurnsSession}, {ID: "native-root-b", TaskID: nativeTurnsTask, TaskSessionID: nativeTurnsSession}}
			for _, turn := range turns {
				require.NoError(t, tasks.CreateTurn(t.Context(), turn))
			}
			for i := range 4 {
				require.NoError(t, tasks.CreateTaskUsageEvent(t.Context(), nativeTurnsEvent(i, turns)))
			}
			var scope string
			require.NoError(t, repo.db.Get(&scope, repo.db.Rebind(`SELECT scope_id FROM usage_scopes WHERE workspace_id=?`), nativeTurnsWorkspace))
			// Model an imported legacy retained representation. Authoritative
			// ledger amounts/digests and default unknown scope readiness remain
			// untouched; the ordinary worker must repair these versioned rows.
			_, err := repo.db.Exec(repo.db.Rebind(`UPDATE session_usage_buckets SET turns=1,native_accounting_version=0 WHERE scope_id=? AND source='native'`), scope)
			require.NoError(t, err)
			generation, err := repo.StartUsageHistoryRebuild(t.Context(), scope)
			require.NoError(t, err)
			drainProjection(t, repo, scope, generation)
			filter := models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: "model"}
			before, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.True(t, before.ProjectionPending)
			_, err = repo.RunUsageHistoryProjectionBatch(t.Context(), 2)
			require.NoError(t, err)
			require.Equal(t, 2, nativeTurnsPending(t, repo.db), "one runtime tick repairs at most its requested batch")
			assertNativeAccountingSourceState(t, repo, scope, false)
			_, err = repo.RunUsageHistoryProjectionBatch(t.Context(), 2)
			require.NoError(t, err)
			require.Zero(t, nativeTurnsPending(t, repo.db))
			assertNativeAccountingSourceState(t, repo, scope, true)
			pending, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.True(t, pending.ProjectionPending, "source completion must wait for replay and atomic publication")
			require.Nil(t, pending.Totals.ReasoningTokens)
			completeFixtureProjection(t, repo)
			after, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.False(t, after.ProjectionPending)
			require.NotEqual(t, before.ProjectionGeneration, after.ProjectionGeneration)
			require.Equal(t, models.Int64(80), after.Totals.TotalTokens)
			require.Equal(t, models.Int64(20), after.Totals.ReasoningTokens)
			require.Nil(t, after.Totals.Turns)
		})
	}
}
