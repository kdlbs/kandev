package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryMetricApplicabilityForNativeAndReportedCost(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, equivalent := range []bool{true, false} {
			t.Run(fmt.Sprintf("postgres=%t/equivalent_work_and_coverage=%t", postgres, equivalent), func(t *testing.T) {
				exerciseNativeReportedCostApplicability(t, postgres, equivalent)
			})
		}
	}
}

func TestUsageHistoryNativePointBoundaryDoesNotOverlapEndingInterval(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			retainSegmentNative(t, repo, 150)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)

			cost := reportedNativeCostObservation(t, binding, true)
			require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", cost).Status)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)

			var nativeTable, nativeID string
			err := repo.db.QueryRowx(repo.db.Rebind(`SELECT table_name,id FROM (
				SELECT 'session_usage_measurements' AS table_name,id FROM session_usage_measurements WHERE source='native' AND source_record_id=?
				UNION ALL
				SELECT 'session_usage_buckets' AS table_name,id FROM session_usage_buckets WHERE source='native' AND source_record_id=?) LIMIT 1`),
				"segment-native-event", "segment-native-event").Scan(&nativeTable, &nativeID)
			require.NoError(t, err)
			var reportedTable, reportedID string
			err = repo.db.QueryRowx(repo.db.Rebind(`SELECT table_name,id FROM (
				SELECT 'session_usage_measurements' AS table_name,id FROM session_usage_measurements WHERE item_id=?
				UNION ALL SELECT 'session_usage_buckets' AS table_name,id FROM session_usage_buckets WHERE item_id=?) LIMIT 1`),
				cost.ItemID, cost.ItemID).Scan(&reportedTable, &reportedID)
			require.NoError(t, err)
			// Provider-event reports require point coverage. Rewrite only the stored
			// coverage fields to exercise the overlap predicate's interval boundary.
			start := time.Date(2026, 1, 2, 11, 59, 59, 0, time.UTC)
			end := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
			_, err = repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE `+reportedTable+`
				SET coverage_start=?,coverage_end=?,coverage_key='interval:touching-end',selection_coverage='interval:touching-end' WHERE id=?`), start, end, reportedID)
			require.NoError(t, err)
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			native, err := repo.loadIndexedObservationTx(t.Context(), tx, nativeTable, nativeID)
			require.NoError(t, err)
			reported, err := repo.loadIndexedObservationTx(t.Context(), tx, reportedTable, reportedID)
			require.NoError(t, err)
			require.Equal(t, "interval:touching-end", reported.CoverageKey)
			overlap, err := repo.usageSegmentNativeOverlapTx(t.Context(), tx, native, reported.CostBundle, true)
			require.NoError(t, err)
			require.False(t, overlap, "an interval ending at the native point is outside the half-open coverage")
		})
	}
}

func exerciseNativeReportedCostApplicability(t *testing.T, postgres, equivalent bool) {
	t.Helper()
	repo := segmentContractFixture(t, postgres)
	binding := segmentBinding(t, repo, "collector-a", "session-1")
	retainSegmentNative(t, repo, 150)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)

	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "session"}
	assertNativeCostUnknown(t, repo, filter)
	cost := reportedNativeCostObservation(t, binding, equivalent)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", cost).Status)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)

	generation := assertReportedCostAggregate(t, repo, filter, equivalent)
	assertIndependentCanonicalSources(t, repo, filter, equivalent)
	assertHostMetricApplicability(t, repo, filter, generation, equivalent)
}

func assertNativeCostUnknown(t *testing.T, repo *Repository, filter models.SessionUsageFilter) {
	t.Helper()
	before, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, models.Int64(150), before.Totals.TotalTokens)
	require.Nil(t, before.Totals.CostSubcents, "native token evidence with no price remains unknown cost")
	rows, _, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].CostMetricAbsent)
	require.Nil(t, rows[0].CostSubcents)
}

func reportedNativeCostObservation(t *testing.T, binding string, equivalent bool) pluginsdk.UsageObservationV1 {
	t.Helper()
	identity, occurrence := "unrelated-provider-event", "2026-01-02T12:00:01Z"
	if equivalent {
		identity, occurrence = "segment-native-event", "2026-01-02T12:00:00Z"
	}
	model, provider := "Sonnet", "anthropic"
	amount := int64(1200)
	cost := pluginsdk.UsageObservationV1{
		SubjectBinding: binding, ItemID: "reported-native-cost", SourceRecordID: "reported-native-cost",
		SourceVersion: "metric-applicability-fixture-v1", Revision: 1,
		WorkIdentity: &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: identity},
		ModelID:      &model, ModelProvider: &provider,
		Cost:     &pluginsdk.UsageCostV1{Amount: &amount, Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported},
		Coverage: &pluginsdk.UsageCoverageV1{OccurrenceAt: &occurrence},
		Quality:  &pluginsdk.UsageObservationQualityV1{CostCoverageComplete: true},
	}
	sealContractObservation(t, &cost)
	return cost
}

func assertReportedCostAggregate(t *testing.T, repo *Repository, filter models.SessionUsageFilter, equivalent bool) string {
	t.Helper()
	after, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, models.Int64(150), after.Totals.TotalTokens)
	if equivalent {
		require.Equal(t, models.Int64(1200), after.Totals.CostSubcents,
			"a reported price joins native usage only when work identity and point coverage match")
	} else {
		require.Nil(t, after.Totals.CostSubcents,
			"an unrelated accepted price cannot erase the native event's unknown applicable cost")
	}
	return after.ProjectionGeneration
}

func assertIndependentCanonicalSources(t *testing.T, repo *Repository, filter models.SessionUsageFilter, equivalent bool) {
	t.Helper()
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), filter)
	require.NoError(t, err)
	tokenSources, costSources := 0, 0
	for _, fact := range facts {
		if fact.TokenSource != nil && fact.TokenSource.Source == "native" {
			tokenSources++
			require.Equal(t, "segment-native-event", fact.TokenSource.ItemID)
			require.Equal(t, "native-actor", fact.TokenSource.ActorID)
			require.Equal(t, models.Int64(150), fact.Measurement.TotalTokens)
		}
		if fact.CostSource != nil && fact.CostSource.Source == "plugin:collector-a" {
			costSources++
			require.Equal(t, "reported-native-cost", fact.CostSource.ItemID)
			require.Equal(t, models.Int64(1200), fact.Measurement.CostSubcents)
			require.False(t, fact.CostMetricAbsent)
			if equivalent {
				require.Equal(t, "native-actor", fact.CostSource.ActorID,
					"the Host resolves a proven provider event to the native actor")
			}
		}
	}
	require.Equal(t, 1, tokenSources)
	require.Equal(t, 1, costSources)
}

func assertHostMetricApplicability(t *testing.T, repo *Repository, filter models.SessionUsageFilter, generation string, equivalent bool) {
	t.Helper()
	rows, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, generation, state.Generation)
	var tokens, costs int64
	tokenKnown, costKnown := true, true
	for _, row := range rows {
		if !row.TokenMetricAbsent {
			if row.TotalTokens == nil {
				tokenKnown = false
			} else {
				tokens += *row.TotalTokens
			}
		}
		if !row.CostMetricAbsent {
			if row.CostSubcents == nil {
				costKnown = false
			} else {
				costs += *row.CostSubcents
			}
		}
	}
	require.True(t, tokenKnown)
	require.Equal(t, int64(150), tokens)
	if equivalent {
		require.True(t, costKnown)
		require.Equal(t, int64(1200), costs)
	} else {
		require.False(t, costKnown)
	}
}
