package sqlite

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func legacySegmentPair(t *testing.T, repo *Repository, providerSplit bool) ([]models.SessionUsageUpsertResult, string) {
	t.Helper()
	binding := segmentBinding(t, repo, "legacy-segments", "session-1")
	var input []models.SessionUsageMeasurement
	for i, model := range []string{"Sonnet", "Opus"} {
		row := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
		row.Source, row.SourceRecordID, row.UsageIdentity = "plugin:legacy-segments", "segment-thread", "session-model-cost"
		row.TranscriptID, row.Model, row.Provider = "segment-thread", model, "anthropic"
		if providerSplit && i == 1 {
			row.Model, row.Provider = "Sonnet", "bedrock"
		}
		amount := int64(i+1) * 100
		row.InputTokens, row.TotalTokens, row.CostSubcents = models.Int64(amount), models.Int64(amount), models.Int64(amount)
		row.PayloadDigest = "old-dto-" + model
		input = append(input, row)
	}
	writes, err := repo.UpsertSessionUsage(t.Context(), input)
	require.NoError(t, err)
	require.Len(t, writes, 2)
	for _, write := range writes {
		require.Equal(t, models.UsageWriteApplied, write.Status)
	}
	return writes, binding
}

func TestUsageHistorySegmentsLegacySupersessionPreservesLineage(t *testing.T) {
	repo := segmentContractFixture(t, false)
	writes, binding := legacySegmentPair(t, repo, false)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	requireSegmentTotals(t, repo, 300, 300)
	oldKey := segmentFactKey(t, repo, "Sonnet")
	item := contractObservation(t, binding, "new-stable-row-id", 1, "corrected-Sonnet", 80)
	item.SupersedesItemID = &writes[0].Measurement.ID
	item.ModelProvider = stringPtr("bedrock")
	item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(80), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisEstimated}
	item.Quality.CostCoverageComplete = true
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "legacy-segments", item).Status)
	completeFixtureProjection(t, repo)
	requireSegmentTotals(t, repo, 280, 280)
	require.Equal(t, oldKey, segmentFactKey(t, repo, "corrected-Sonnet"))
	// Repair must preserve the explicit supersession evidence without editing
	// the client's authenticated payload or digest.
	var id string
	require.NoError(t, repo.db.Get(&id, `SELECT observation_id FROM usage_observation_items WHERE item_id='new-stable-row-id'`))
	repairFrozenWorkRow(t, repo, usageMeasurementsTable, id)
	completeFixtureProjection(t, repo)
	requireSegmentTotals(t, repo, 280, 280)
	require.Equal(t, oldKey, segmentFactKey(t, repo, "corrected-Sonnet"))
}

func TestUsageHistorySegmentsLegacyProvidersAreNotInferredDisjoint(t *testing.T) {
	repo := segmentContractFixture(t, false)
	_, _ = legacySegmentPair(t, repo, true)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.True(t, report.Totals.Partial)
	require.NotEqual(t, models.Int64(300), report.Totals.TotalTokens, "old provider labels alone cannot prove two members of a model series")
}

func TestUsageHistorySegmentsLifetimeAlternativeIsMetricLocal(t *testing.T) {
	repo := segmentContractFixture(t, false)
	binding := segmentBinding(t, repo, "collector-a", "session-1")
	daily := multiModelAdapterObservation(t, binding, "daily-token-item", "Sonnet", "anthropic", 100, true)
	daily.Cost = nil
	daily.Quality.CostCoverageComplete = false
	sealContractObservation(t, &daily)
	lifetime := multiModelAdapterObservation(t, binding, "lifetime-cost-item", "Sonnet", "anthropic", 80, false)
	lifetime.Segment = daily.Segment
	lifetime.Tokens = nil
	lifetime.Quality.TokenCoverageComplete = false
	sealContractObservation(t, &lifetime)
	reportSegmentBatch(t, repo, "collector-a", daily, lifetime)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	requireSegmentTotals(t, repo, 100, 80)
}
