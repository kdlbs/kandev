package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-006.4 AC-COSTS-USAGE-HISTORY-007.1
func TestUsageHistoryProjectionOversizedGroupResumes(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	targets, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for start := 0; start < 221; start += 200 {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: 1}
		for i := start; i < start+200 && i < 221; i++ {
			batch.Observations = append(batch.Observations, contractObservation(t, targets.Targets[0].SubjectBinding, fmt.Sprintf("large-%03d", i), 1, "model", int64(100+i)))
		}
		result, err := api.ReportUsageObservations(ctx, "collector-a", batch)
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
	}
	selected := selectContractPartition(t, repo, "large-000")
	require.True(t, selected.Preparing, "foreground selector must retain its source bound")
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	for range 100 {
		_, err := repo.AdvanceUsageHistoryProjection(ctx, scope, generation, 17)
		require.NoError(t, err)
		var resolutions int
		require.NoError(t, repo.db.Get(&resolutions, `SELECT COUNT(*) FROM usage_projection_resolutions`))
		if resolutions > 0 {
			break
		}
	}
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	drainProjection(t, restarted, scope, generation)
	var total int64
	require.NoError(t, repo.db.Get(&total, `SELECT SUM(total_tokens) FROM usage_lifetime_summaries WHERE metric_kind = 'tokens'`))
	require.EqualValues(t, 100, total, "bounded winner reduction must match the selector policy")
	var resolutions int
	require.NoError(t, repo.db.Get(&resolutions, `SELECT COUNT(*) FROM usage_projection_resolutions`))
	require.Zero(t, resolutions)
}
