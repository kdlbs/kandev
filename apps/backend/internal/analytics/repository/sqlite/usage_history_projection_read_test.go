package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-006.3 AC-COSTS-USAGE-HISTORY-006.5
func TestUsageHistoryProjectionSavedCanonicalPending(t *testing.T) {
	repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
	ctx := context.Background()
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1"}
	rows, state, err := repo.ListCanonicalUsagePage(ctx, filter)
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Equal(t, "preparing", state.Status)
	require.True(t, state.Pending)
	generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	_, complete, err := repo.ListCanonicalUsagePage(ctx, filter)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(complete.Generation, generation+"."))
	correction := contractObservation(t, binding, "projection-item", 2, "new-model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", correction).Status)
	rows, state, err = repo.ListCanonicalUsagePage(ctx, filter)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 100, *rows[0].TotalTokens)
	require.Equal(t, complete.Generation, state.Generation)
	require.True(t, state.Pending)
	drainProjection(t, driver, scope, generation)
	rows, state, err = repo.ListCanonicalUsagePage(ctx, filter)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 40, *rows[0].TotalTokens)
	require.False(t, state.Pending)
	require.NotEqual(t, complete.Generation, state.Generation, "live publication must invalidate a cross-page identity")
	var tombstone int
	require.NoError(t, repo.db.Get(&tombstone, `SELECT tombstone FROM usage_projection_items LIMIT 1`))
	require.Zero(t, tombstone, "the after-image must win over the same-sequence before-image")
	var dirty int
	require.NoError(t, repo.db.Get(&dirty, `SELECT COUNT(*) FROM usage_dirty_partitions`))
	require.Zero(t, dirty, "publication must complete durable dirty work atomically")
}

// @covers AC-COSTS-USAGE-HISTORY-007.1 AC-COSTS-USAGE-HISTORY-006.4
func TestUsageHistoryGenerationCapturedTransferAndErasure(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	api := retainedUsageAPI(repo)
	ctx := context.Background()
	targets, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "captured", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	_, err = repo.db.Exec(`UPDATE workspaces SET org_id = 'new-org' WHERE id = 'ws-1'`)
	require.NoError(t, err)
	newBinding := freshProjectionBinding(t, repo)
	current := contractObservation(t, newBinding, "current-org-B", 1, "model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", current).Status)
	var currentScope string
	require.NoError(t, repo.db.Get(&currentScope, `SELECT scope_id FROM usage_observation_items WHERE item_id='current-org-B'`))
	require.NotEqual(t, scope, currentScope)
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err, "private projection must retain captured ownership after transfer")
	drainProjection(t, repo, scope, generation)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = repo.SelectUsagePartitionTx(ctx, tx, models.UsageSelectionQuery{Partition: models.UsagePartition{ScopeID: scope}})
	require.ErrorIs(t, err, ErrUsageNotVisible)
	require.NoError(t, tx.Rollback())
	shadow, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	require.NoError(t, repo.EraseOrganizationUsage(ctx, "org-a"))
	for _, table := range []string{"usage_projection_scopes", "usage_projection_log", "usage_projection_partitions", "usage_projection_source_versions", "usage_projection_generations", "usage_projection_checkpoints", "usage_projection_items", "usage_projection_work", "usage_projection_applied", "usage_projection_resolutions", "usage_projection_replay_groups", "usage_projection_builds", "usage_canonical_facts", "usage_projection_contributions", "usage_projection_provider_options", "usage_projection_summary_queue", "usage_daily_summaries", "usage_lifetime_summaries", "usage_projection_history"} {
		var count int
		require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`), scope))
		require.Zero(t, count, table)
	}
	_, err = repo.AdvanceUsageHistoryProjection(ctx, scope, shadow, 17)
	require.ErrorIs(t, err, ErrUsageNotVisible)
	completeFixtureProjection(t, repo)
	rows, _, err := repo.ListCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 40, *rows[0].TotalTokens, "erasing captured A must preserve fresh B")
}

func TestUsageHistoryGenerationResetRejectsStaleCheckpoint(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	ctx := context.Background()
	page, err := repo.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	initial := contractObservation(t, page.Targets[0].SubjectBinding, "projection-item", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", initial).Status)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	active, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, active)
	shadow, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	item := contractObservation(t, initial.SubjectBinding, "projection-item", 2, "model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	_, err = repo.AdvanceUsageHistoryProjection(ctx, scope, active, 17)
	require.NoError(t, err)
	var groups int
	require.NoError(t, repo.db.Get(&groups, `SELECT COUNT(*) FROM usage_projection_replay_groups`))
	require.Positive(t, groups, "reset must exercise a pending publication group")
	require.NoError(t, repo.ResetWorkspaceUsageHistory(ctx, "ws-1"))
	var tables []string
	require.NoError(t, repo.db.Select(&tables, `SELECT name FROM sqlite_master WHERE type = 'table' AND (name LIKE 'usage_projection_%' OR name IN ('usage_canonical_facts', 'usage_daily_summaries', 'usage_lifetime_summaries'))`))
	require.Len(t, tables, 19)
	for _, table := range tables {
		var count int
		require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE scope_id = ?`), scope))
		require.Zero(t, count, table)
	}
	item = contractObservation(t, freshProjectionBinding(t, repo), "projection-item", 1, "model", 25)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	_, err = repo.AdvanceUsageHistoryProjection(ctx, scope, shadow, 17)
	require.ErrorIs(t, err, ErrUsageNotVisible)
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	rows, _, err := repo.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 25, *rows[0].TotalTokens)
}
