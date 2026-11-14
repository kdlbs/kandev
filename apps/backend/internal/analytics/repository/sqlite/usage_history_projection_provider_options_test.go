package sqlite

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryProviderOptionBackfillWaitsForWorkerAfterRestart(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	page, err := repo.ListUsageCollectionTargets(context.Background(), "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "provider-options-restart", 1, "provider-options-model", 10)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	repairAllNativeTurns(t, repo, 200)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	driver := projectionDriver(repo)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	finishFixtureRetention(t, repo)

	_, err = repo.db.Exec(repo.db.Rebind(`DELETE FROM usage_projection_provider_options WHERE scope_id=? AND generation=?`), scope, generation)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE usage_projection_history SET provider_options_ready=0,provider_options_version=0,
		provider_options_build_id='',provider_options_cursor='',provider_options_phase='clear' WHERE scope_id=? AND generation=?`), scope, generation)
	require.NoError(t, err)

	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	var options int
	require.NoError(t, repo.db.Get(&options, repo.db.Rebind(`SELECT COUNT(*) FROM usage_projection_provider_options WHERE scope_id=? AND generation=?`), scope, generation))
	require.Zero(t, options, "schema initialization must leave resumable provider work for the projection worker")

	report, err := restarted.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Limit: 25})
	require.NoError(t, err)
	require.Equal(t, "preparing", report.ProjectionStatus)
	require.True(t, report.ProjectionPending)
	require.Empty(t, report.Providers, "an incomplete dimension must not be reported as a complete provider list")

	for range 200 {
		done, err := restarted.RunUsageHistoryProjectionBatch(context.Background(), 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	require.NoError(t, repo.db.Get(&options, repo.db.Rebind(`SELECT COUNT(*) FROM usage_projection_provider_options WHERE scope_id=? AND generation=?`), scope, generation))
	require.Positive(t, options, "the bounded worker must resume and finish the provider dimension")
	complete, err := restarted.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Limit: 25})
	require.NoError(t, err)
	require.Contains(t, complete.Providers, "__unknown_provider__")
}

func TestUsageHistoryProviderOptionRefreshBoundsOversizedWork(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	ctx := context.Background()
	page, err := repo.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	const factCount = 401
	for first := 0; first < factCount; first += 200 {
		end := min(first+200, factCount)
		observations := make([]pluginsdk.UsageObservationV1, 0, end-first)
		for index := first; index < end; index++ {
			itemID := fmt.Sprintf("oversized-provider-%03d", index)
			item := contractObservation(t, page.Targets[0].SubjectBinding, itemID, 1, "oversized-model", 1)
			item.ModelProvider = stringPtr(fmt.Sprintf("provider-%03d", index))
			item.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "oversized-provider-v1", SegmentID: itemID}
			sealContractObservation(t, &item)
			observations = append(observations, item)
		}
		result, err := repo.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV2, Observations: observations})
		require.NoError(t, err)
		require.Len(t, result.Results, len(observations))
		for _, accepted := range result.Results {
			require.Equal(t, "accepted", accepted.Status)
		}
	}
	repairAllNativeTurns(t, repo, 200)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	driver := projectionDriver(repo)
	generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	var previous int
	for range 5000 {
		done, err := driver.AdvanceUsageHistoryProjection(ctx, scope, generation, 200)
		require.NoError(t, err)
		var current int
		require.NoError(t, repo.db.Get(&current, repo.db.Rebind(`SELECT COUNT(*) FROM usage_projection_provider_options WHERE scope_id=? AND generation=?`), scope, generation))
		if current > previous {
			t.Logf("provider dimensions advanced %d to %d", previous, current)
		}
		require.LessOrEqual(t, current-previous, 400, "one projection operation may derive at most 200 facts, with two source memberships per fact")
		previous = current
		if done {
			break
		}
	}
	var projected struct {
		Facts int `db:"facts"`
		Works int `db:"works"`
	}
	require.NoError(t, repo.db.Get(&projected, repo.db.Rebind(`SELECT COUNT(*) AS facts, COUNT(DISTINCT work_key) AS works FROM usage_canonical_facts WHERE scope_id=? AND generation=?`), scope, generation))
	require.Equal(t, factCount, projected.Facts, "the fixture must retain all 401 canonical facts")
	require.Equal(t, 1, projected.Works, "the fixture must exercise an oversized single work")
	require.Equal(t, factCount*2, previous, "both published source memberships for every fact must be present after refresh")
}

func TestUsageHistoryProviderOptionBackfillSerializesPublication(t *testing.T) {
	repo, _ := currentTargetDatabaseFixture(t, false)
	ctx := context.Background()
	page, err := repo.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "concurrent-provider", 1, "old-model", 10)
	item.ModelProvider = stringPtr("old-provider")
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	repairAllNativeTurns(t, repo, 200)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	driver := projectionDriver(repo)
	generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	finishFixtureRetention(t, repo)

	_, err = repo.db.Exec(repo.db.Rebind(`DELETE FROM usage_projection_provider_options WHERE scope_id=? AND generation=?`), scope, generation)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE usage_projection_history SET provider_options_ready=0,provider_options_cursor='',provider_options_phase='clear'
		WHERE scope_id=? AND generation=?`), scope, generation)
	require.NoError(t, err)

	correction := contractObservation(t, item.SubjectBinding, item.ItemID, 2, "new-model", 20)
	correction.ModelProvider = stringPtr("new-provider")
	sealContractObservation(t, &correction)
	start := make(chan struct{})
	errors := make(chan error, 2)
	go func() {
		<-start
		_, err := driver.AdvanceUsageHistoryProjection(ctx, scope, generation, 200)
		errors <- err
	}()
	go func() {
		<-start
		_, err := repo.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV1, Observations: []pluginsdk.UsageObservationV1{correction}})
		errors <- err
	}()
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	completeFixtureProjection(t, repo)

	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Limit: 25})
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.False(t, report.ProjectionPending)
	require.Equal(t, []string{"new-provider"}, report.Providers)
}
