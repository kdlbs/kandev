package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-006.3 AC-COSTS-USAGE-HISTORY-006.5
func TestUsageHistoryProjectionPublicationCrashPoints(t *testing.T) {
	for _, point := range []struct{ name, statement string }{
		{"facts", `BEFORE INSERT ON usage_canonical_facts`},
		{"pointer", `BEFORE UPDATE ON usage_projection_applied`},
		{"summary", `BEFORE INSERT ON usage_lifetime_summaries`},
		{"revision", `BEFORE UPDATE ON usage_projection_generations WHEN NEW.revision <> OLD.revision`},
		{"work", `BEFORE DELETE ON usage_projection_work`},
	} {
		t.Run(point.name, func(t *testing.T) {
			repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
			ctx := context.Background()
			generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
			require.NoError(t, err)
			drainProjection(t, driver, scope, generation)
			item := contractObservation(t, binding, "projection-item", 2, "model", 40)
			require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
			_, err = repo.db.Exec(`CREATE TRIGGER projection_crash ` + point.statement + ` BEGIN SELECT RAISE(ABORT, 'projection crash'); END`)
			require.NoError(t, err)
			failed := false
			for range 30 {
				_, err = driver.AdvanceUsageHistoryProjection(ctx, scope, generation, 17)
				if err != nil {
					require.Contains(t, err.Error(), "projection crash")
					failed = true
					break
				}
			}
			require.True(t, failed, "the production publication must reach the crash point")
			rows, state, err := repo.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			require.EqualValues(t, 100, *rows[0].TotalTokens)
			require.True(t, state.Pending)
			_, err = repo.db.Exec(`DROP TRIGGER projection_crash`)
			require.NoError(t, err)
			restarted, err := NewWithDB(repo.db, repo.ro)
			require.NoError(t, err)
			drainProjection(t, restarted, scope, generation)
			rows, _, err = restarted.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			require.EqualValues(t, 40, *rows[0].TotalTokens)
		})
	}
}

// @covers AC-COSTS-USAGE-HISTORY-006.2 AC-COSTS-USAGE-HISTORY-006.4
func TestUsageHistoryGenerationReplayTombstonesAndConsumerWatermarks(t *testing.T) {
	repo, _, scope, binding := projectionFixtureWithIssuedBinding(t)
	ctx := context.Background()
	active, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, active)
	shadow, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	_, err = repo.AdvanceUsageHistoryProjection(ctx, scope, shadow, 1)
	require.NoError(t, err)
	corrected := contractObservation(t, binding, "projection-item", 2, "corrected", 70)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", corrected).Status)
	replacement := contractObservation(t, binding, "replacement", 1, "new-model", 60)
	replacement.SupersedesItemID = stringPtr("projection-item")
	sealContractObservation(t, &replacement)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", replacement).Status)
	tombstone := pluginsdk.UsageObservationTombstoneV1{SubjectBinding: binding, ItemID: "replacement", Revision: 2}
	tombstone.PayloadDigest, err = tombstone.ComputePayloadDigestV1()
	require.NoError(t, err)
	result, err := repo.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Tombstones: []pluginsdk.UsageObservationTombstoneV1{tombstone}})
	require.NoError(t, err)
	require.Equal(t, "accepted", result.Results[0].Status)
	drainProjection(t, repo, scope, active)
	var retained int
	require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM usage_projection_log`))
	require.Equal(t, 4, retained, "the active consumer cannot prune the shadow's replay input")
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	drainProjection(t, restarted, scope, shadow)
	var count int
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT COUNT(*) FROM usage_lifetime_summaries WHERE generation = ?`), shadow))
	require.Zero(t, count)
	clean, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, clean)
	require.NoError(t, repo.db.Get(&count, repo.db.Rebind(`SELECT fact_count FROM usage_projection_generations WHERE generation = ?`), clean))
	require.Zero(t, count, "clean rebuild and ordered tombstone replay must agree")
}

func TestUsageHistoryProjectionLongSessionPagesAndRestart(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	targets, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for start := 0; start < 205; start += 200 {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: 1}
		for i := start; i < start+200 && i < 205; i++ {
			item := contractObservation(t, targets.Targets[0].SubjectBinding, fmt.Sprintf("history-%03d", i), 1, "model", 1)
			point := time.Date(2021, 1, 1+i, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
			item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
			sealContractObservation(t, &item)
			batch.Observations = append(batch.Observations, item)
		}
		_, err := api.ReportUsageObservations(ctx, "collector-a", batch)
		require.NoError(t, err)
	}
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	for range 20 {
		_, err = repo.AdvanceUsageHistoryProjection(ctx, scope, generation, 17)
		require.NoError(t, err)
	}
	restarted, err := NewWithDB(repo.db, repo.ro)
	require.NoError(t, err)
	drainProjection(t, restarted, scope, generation)
	var total int64
	require.NoError(t, repo.db.Get(&total, `SELECT SUM(total_tokens) FROM usage_daily_summaries WHERE metric_kind = 'tokens'`))
	require.EqualValues(t, 205, total)
	verifySummaryPages(t, repo)
}

func verifySummaryPages(t *testing.T, repo *Repository) {
	t.Helper()
	offset, tokens, publication := 0, int64(0), ""
	for range 100 {
		rows, metadata, err := repo.ListUsageDailySummaries(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", Timezone: "UTC", Limit: 17, Offset: offset})
		require.NoError(t, err)
		if publication == "" {
			publication = metadata.Generation
		}
		require.Equal(t, publication, metadata.Generation)
		for _, row := range rows {
			if row.TotalTokens != nil {
				tokens += *row.TotalTokens
			}
		}
		if !metadata.HasMore {
			require.EqualValues(t, 205, tokens)
			return
		}
		require.Greater(t, metadata.NextOffset, offset)
		offset = metadata.NextOffset
	}
	t.Fatal("summary pagination did not finish")
}

func TestUsageHistoryProjectionWorkIdentityCorrectionPublishesAtomically(t *testing.T) {
	repo, _, scope, binding := projectionFixtureWithIssuedBinding(t)
	ctx := context.Background()
	generation, err := repo.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	item := contractObservation(t, binding, "projection-item", 2, "model", 40)
	item.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_thread", ID: "corrected-thread"}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	for range 40 {
		done, err := repo.AdvanceUsageHistoryProjection(ctx, scope, generation, 1)
		require.NoError(t, err)
		rows, _, err := repo.ListCanonicalUsagePage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1"})
		require.NoError(t, err)
		var total int64
		for _, row := range rows {
			total += *row.TotalTokens
		}
		require.Contains(t, []int64{100, 40}, total, "old and new work partitions must change in the same publication")
		if done {
			require.EqualValues(t, 40, total)
			return
		}
	}
	t.Fatal("work-identity correction did not finish")
}
