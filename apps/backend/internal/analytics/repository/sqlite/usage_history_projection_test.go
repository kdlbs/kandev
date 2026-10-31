package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

type projectionDriver interface {
	StartUsageHistoryRebuild(context.Context, string) (string, error)
	AdvanceUsageHistoryProjection(context.Context, string, string, int) (bool, error)
}

// @covers AC-COSTS-USAGE-HISTORY-004.3 AC-COSTS-USAGE-HISTORY-006.3
func TestUsageHistoryProjectionAdmissionLog(t *testing.T) {
	repo, api := historyContractFixture(t)
	var tables int
	require.NoError(t, repo.db.Get(&tables, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_projection_log'`))
	require.Equal(t, 1, tables, "accepted observations need a transactional replay log")
	ctx := context.Background()
	targets, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "ordered", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	require.Equal(t, "unchanged", reportContractObservation(t, api, "collector-a", item).Status)
	var sequence int64
	require.NoError(t, repo.db.Get(&sequence, `SELECT MAX(commit_sequence) FROM usage_projection_log`))
	require.EqualValues(t, 1, sequence)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	item.Revision, item.Tokens.TotalTokens, item.Tokens.InputTokens = 2, models.Int64(50), models.Int64(50)
	sealContractObservation(t, &item)
	_, changes, err := repo.MutateUsageObservationsTx(ctx, tx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	require.NoError(t, tx.Rollback())
	require.NoError(t, repo.db.Get(&sequence, `SELECT MAX(commit_sequence) FROM usage_projection_log`))
	require.EqualValues(t, 1, sequence, "source rollback must roll back its sequence and replay payload")
}

func projectionFixture(t *testing.T) (*Repository, projectionDriver, string) {
	t.Helper()
	repo, driver, scope, _ := projectionFixtureWithIssuedBinding(t)
	return repo, driver, scope
}

func projectionFixtureWithIssuedBinding(t *testing.T) (*Repository, projectionDriver, string, string) {
	t.Helper()
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	targets, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "projection-item", 1, "model", 100)
	require.Equal(t, "accepted", reportContractObservation(t, api, "collector-a", item).Status)
	repairAllNativeTurns(t, repo, 200)
	driver, ok := any(repo).(projectionDriver)
	require.True(t, ok, "projection requires durable generation operations")
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	return repo, driver, scope, item.SubjectBinding
}

func drainProjection(t *testing.T, driver projectionDriver, scope, generation string) {
	t.Helper()
	for range 2000 {
		done, err := driver.AdvanceUsageHistoryProjection(context.Background(), scope, generation, 17)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("bounded projection did not finish")
}

func completeFixtureProjection(t *testing.T, repo *Repository) {
	t.Helper()
	for range 2000 {
		done, err := repo.RunUsageHistoryProjectionBatch(context.Background(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("projection backlog did not finish")
}

// @covers AC-COSTS-USAGE-HISTORY-006.4 AC-COSTS-USAGE-HISTORY-006.5
func TestUsageHistoryGenerationRebuildAndCorrection(t *testing.T) {
	repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
	ctx := context.Background()
	generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	var active string
	require.NoError(t, repo.db.Get(&active, `SELECT active_generation FROM usage_projection_scopes`))
	require.Equal(t, generation, active)
	var total int64
	require.NoError(t, repo.db.Get(&total, `SELECT SUM(total_tokens) FROM usage_lifetime_summaries WHERE metric_kind = 'tokens'`))
	require.EqualValues(t, 100, total)
	shadow, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	require.NotEqual(t, generation, shadow)
	require.NoError(t, repo.db.Get(&active, `SELECT active_generation FROM usage_projection_scopes`))
	require.Equal(t, generation, active, "an incomplete rebuild must preserve the complete generation")
	corrected := contractObservation(t, binding, "projection-item", 2, "new-model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", corrected).Status)
	drainProjection(t, driver, scope, shadow)
	require.NoError(t, repo.db.Get(&total, repo.db.Rebind(`SELECT SUM(total_tokens) FROM usage_lifetime_summaries WHERE generation = ? AND metric_kind = 'tokens'`), shadow))
	require.EqualValues(t, 40, total)
	var oldModels int
	require.NoError(t, repo.db.Get(&oldModels, repo.db.Rebind(`SELECT COUNT(*) FROM usage_lifetime_summaries WHERE generation = ? AND model = 'model'`), shadow))
	require.Zero(t, oldModels)
}
