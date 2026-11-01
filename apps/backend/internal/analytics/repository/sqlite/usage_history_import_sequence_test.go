package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-IMPORT-002.4 AC-COSTS-USAGE-IMPORT-002.5
func TestLifetimeThenDatedImportCountsOnce(t *testing.T) {
	repo, api := historyContractFixture(t)
	ctx := context.Background()
	page, err := api.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, page.Targets, 1)
	binding := page.Targets[0].SubjectBinding

	report := func(item pluginsdk.UsageObservationV1) pluginsdk.UsageObservationWriteResultV1 {
		t.Helper()
		digest, err := item.ComputePayloadDigestV1()
		require.NoError(t, err)
		item.PayloadDigest = digest
		result, err := api.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{item}})
		require.NoError(t, err)
		require.Equal(t, pluginsdk.UsageReportStatusAccepted, result.Status)
		require.Len(t, result.Results, 1)
		return result.Results[0]
	}

	lifetimeTokens := int64(100)
	lifetime := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: "lifetime-item", SourceRecordID: "lifetime-item", SourceVersion: "import-v1", Revision: 1,
		Tokens:  &pluginsdk.UsageTokenAmountsV1{InputTokens: &lifetimeTokens, TotalTokens: &lifetimeTokens},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true},
	}
	first := report(lifetime)
	require.Equal(t, pluginsdk.UsageObservationStatusAccepted, first.Status)

	start, end := "2026-01-02T00:00:00Z", "2026-01-03T00:00:00Z"
	sourceDate, timezone := "2026-01-02", "UTC"
	datedTokens := int64(40)
	dated := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: "dated-item", SourceRecordID: "dated-item", SourceVersion: "import-v1", Revision: 1,
		Tokens:  &pluginsdk.UsageTokenAmountsV1{InputTokens: &datedTokens, TotalTokens: &datedTokens},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true},
		Coverage: &pluginsdk.UsageCoverageV1{
			CoverageStart: &start, CoverageEnd: &end, SourceDate: &sourceDate, SourceTimezone: &timezone,
		},
	}
	require.Equal(t, pluginsdk.UsageObservationStatusAccepted, report(dated).Status)

	for {
		done, err := repo.BackfillUsageSelection(ctx, 200)
		require.NoError(t, err)
		if done {
			break
		}
	}
	completeReportMigration(t, repo)
	completeFixtureProjection(t, repo)
	all, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.NoError(t, err)
	require.Equal(t, "ready", all.ProjectionStatus)
	require.NotNil(t, all.Totals.TotalTokens)
	require.EqualValues(t, 100, *all.Totals.TotalTokens, "a partial dated result must not add to or reduce the fuller lifetime total")

	startTime := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	endTime := startTime.AddDate(0, 0, 1)
	day, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", Start: &startTime, End: &endTime, GroupBy: "day"})
	require.NoError(t, err)
	require.NotNil(t, day.Totals.TotalTokens)
	require.EqualValues(t, 40, *day.Totals.TotalTokens)

	// Replaying an unchanged lifetime report must not revise the accepted
	// observation or change the retained total.
	var beforeRevision int64
	require.NoError(t, repo.db.Get(&beforeRevision, `SELECT revision FROM usage_observation_items WHERE source = 'plugin:collector-a' AND item_id = 'lifetime-item'`))
	replay := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: "lifetime-item", SourceRecordID: "lifetime-item", SourceVersion: "import-v1", Revision: 1,
		Tokens:  &pluginsdk.UsageTokenAmountsV1{InputTokens: &lifetimeTokens, TotalTokens: &lifetimeTokens},
		Quality: &pluginsdk.UsageObservationQualityV1{TokenCoverageComplete: true},
	}
	digest, err := replay.ComputePayloadDigestV1()
	require.NoError(t, err)
	replay.PayloadDigest = digest
	replayed, err := api.ReportUsageObservations(ctx, "collector-a", pluginsdk.UsageObservationBatchV1{SchemaVersion: 1, Observations: []pluginsdk.UsageObservationV1{replay}})
	require.NoError(t, err)
	require.Equal(t, pluginsdk.UsageObservationStatusUnchanged, replayed.Results[0].Status)
	var afterRevision int64
	require.NoError(t, repo.db.Get(&afterRevision, `SELECT revision FROM usage_observation_items WHERE source = 'plugin:collector-a' AND item_id = 'lifetime-item'`))
	require.Equal(t, beforeRevision, afterRevision)
}
