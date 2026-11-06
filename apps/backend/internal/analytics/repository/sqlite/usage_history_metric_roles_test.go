package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func metricRoleObservation(t *testing.T, repo *Repository, kind string) pluginsdk.UsageObservationV1 {
	t.Helper()
	item := multiModelAdapterObservation(t, segmentBinding(t, repo, "collector-a", "session-1"), kind, "Sonnet", "anthropic", 100, kind == "cost_only_day")
	item.Segment = nil
	item.Cost.Amount, item.Cost.Basis = models.Int64(20), pluginsdk.UsageCostBasisReported
	item.Quality.TokenCoverageComplete = false
	switch kind {
	case "cost_only_day":
		item.Tokens = nil
	case "zero_lifetime":
		item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: models.Int64(0), TotalTokens: models.Int64(0)}
		item.Quality.TokenCoverageComplete = true
	default:
		item.Tokens = &pluginsdk.UsageTokenAmountsV1{}
	}
	if kind == "unknown_unpriced" {
		item.Cost, item.Quality.CostCoverageComplete = nil, false
	}
	sealContractObservation(t, &item)
	return item
}

func retainMetricRoleNative(t *testing.T, repo *Repository) {
	t.Helper()
	retainSegmentNative(t, repo, 100, func(native *usagehistory.NativeUsage) {
		native.ReportedTotalTokens, native.CostPresent, native.CostSubcents = models.Int64(100), true, 10
		native.CostSource, native.CostBasis, native.Estimated = usageCostSourceEstimated, models.UsageCostBasisEstimated, true
	})
}

func TestUsageHistoryMetricRolesSchema1AmountsAndDefaults(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, kind := range []string{"cost_only_day", "unknown_lifetime", "unknown_unpriced", "unknown_alone", "zero_lifetime"} {
			t.Run(fmt.Sprintf("postgres=%t/%s", postgres, kind), func(t *testing.T) {
				repo := segmentContractFixture(t, postgres)
				if kind != "unknown_alone" {
					retainMetricRoleNative(t, repo)
				}
				item := metricRoleObservation(t, repo, kind)
				require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
				finishFixtureRetention(t, repo)
				completeFixtureProjection(t, repo)
				report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
				require.NoError(t, err)
				require.Equal(t, "ready", report.ProjectionStatus)
				require.False(t, report.ProjectionPending)
				cost, tokens := int64(20), int64(100)
				if kind == "unknown_unpriced" {
					cost = 10
				}
				if kind == "zero_lifetime" {
					tokens = 0
				}
				require.Equal(t, models.Int64(cost), report.Totals.CostSubcents)
				if kind == "unknown_alone" {
					require.Nil(t, report.Totals.TotalTokens)
				} else {
					require.Equal(t, models.Int64(tokens), report.Totals.TotalTokens)
				}
				require.Nil(t, report.Totals.EffectiveCostPerMillion)
				rows, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
				require.NoError(t, err)
				require.Equal(t, report.ProjectionGeneration, state.Generation)
				if kind == "unknown_alone" {
					require.Len(t, rows, 1)
					require.False(t, rows[0].TokenMetricAbsent)
					require.Nil(t, rows[0].TotalTokens)
				} else {
					var sum int64
					for _, row := range rows {
						if !row.TokenMetricAbsent {
							require.NotNil(t, row.TotalTokens)
							sum += *row.TotalTokens
						}
					}
					require.Equal(t, tokens, sum)
				}
			})
		}
	}
}

func TestUsageHistoryMetricRolesOversizedRestart(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			retainMetricRoleNative(t, repo)
			item := metricRoleObservation(t, repo, "cost_only_day")
			for start := 0; start < 222; start += maxSessionUsageBatch {
				batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: 1}
				for i := start; i < min(start+maxSessionUsageBatch, 222); i++ {
					copy := item
					copy.ItemID = fmt.Sprintf("cost-role-copy-%03d", i)
					sealContractObservation(t, &copy)
					batch.Observations = append(batch.Observations, copy)
				}
				result, err := repo.ReportUsageObservations(t.Context(), "collector-a", batch)
				require.NoError(t, err)
				require.Equal(t, "accepted", result.Status)
			}
			selected := selectContractPartition(t, repo, "cost-role-copy-000")
			require.True(t, selected.Preparing)
			require.LessOrEqual(t, selected.VisitedRows, 201)
			finishFixtureRetention(t, repo)
			var scope string
			require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
			generation, err := repo.StartUsageHistoryRebuild(t.Context(), scope)
			require.NoError(t, err)
			checkpoint := false
			for range 200 {
				_, err = repo.AdvanceUsageHistoryProjection(t.Context(), scope, generation, 17)
				require.NoError(t, err)
				var count int
				require.NoError(t, repo.db.Get(&count, `SELECT COUNT(*) FROM usage_projection_resolutions`))
				if count > 0 {
					checkpoint = true
					break
				}
			}
			require.True(t, checkpoint)
			restarted, err := NewWithDB(repo.db, repo.ro)
			require.NoError(t, err)
			drainProjection(t, restarted, scope, generation)
			requireSegmentTotals(t, restarted, 100, 20)
		})
	}
}

func TestUsageHistoryMetricRolesLegacyNilAndZeroStayApplicable(t *testing.T) {
	repo := segmentContractFixture(t, false)
	_ = segmentBinding(t, repo, "legacy-roles", "session-1")
	var input []models.SessionUsageMeasurement
	for i, model := range []string{"unknown", "zero"} {
		row := usageFixtureMeasurement(time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC))
		row.Source, row.SourceRecordID, row.UsageIdentity, row.TranscriptID = "plugin:legacy-roles", "segment-thread", "session-model-cost", "segment-thread"
		row.Model, row.PayloadDigest = model, model
		row.InputTokens, row.OutputTokens, row.CacheReadTokens, row.CacheWriteTokens, row.ReasoningTokens, row.TotalTokens, row.Turns, row.CostSubcents = nil, nil, nil, nil, nil, nil, nil, nil
		if i == 1 {
			row.InputTokens, row.TotalTokens, row.CostSubcents = models.Int64(0), models.Int64(0), models.Int64(0)
			row.CostBasis = models.UsageCostBasisReported
		}
		input = append(input, row)
	}
	result, err := repo.upsertSessionUsageFixture(t.Context(), input)
	require.NoError(t, err)
	require.Len(t, result, 2)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	rows, _, err := repo.ListRetainedCanonicalUsagePage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.False(t, row.TokenMetricAbsent)
		require.False(t, row.CostMetricAbsent)
		if row.Model == "unknown" {
			require.Nil(t, row.TotalTokens)
			require.Nil(t, row.CostSubcents)
		} else {
			require.Equal(t, models.Int64(0), row.TotalTokens)
			require.Equal(t, models.Int64(0), row.CostSubcents)
		}
	}
}
