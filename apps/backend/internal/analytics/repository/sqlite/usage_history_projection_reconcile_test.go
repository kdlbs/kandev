package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type projectionBatchRunner interface {
	RunUsageHistoryProjectionBatch(context.Context, int) (bool, error)
}

// @covers AC-COSTS-USAGE-HISTORY-007.1 AC-COSTS-USAGE-HISTORY-006.4
func TestUsageHistoryReconcileDurableLogAfterRestart(t *testing.T) {
	repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	item := contractObservation(t, binding, "projection-item", 2, "model", 25)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	_, err = repo.db.Exec(`DELETE FROM usage_dirty_partitions`)
	require.NoError(t, err)
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	runner, ok := any(restarted).(projectionBatchRunner)
	require.True(t, ok, "restart must reconcile committed sequence watermarks")
	for range 100 {
		done, err := runner.RunUsageHistoryProjectionBatch(context.Background(), 17)
		require.NoError(t, err)
		if done {
			break
		}
	}
	var total int64
	require.NoError(t, repo.db.Get(&total, repo.db.Rebind(`SELECT SUM(total_tokens) FROM usage_lifetime_summaries WHERE generation = ? AND metric_kind = 'tokens'`), generation))
	require.EqualValues(t, 25, total)
}
