package sqlite

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/analytics/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// These exercise owned callers, not just the new helpers. They must be run
// after the coordinator/Worker3 integrates the call-site handoff.
func TestUsageHistoryNativeAccountingCallsitesFresh(t *testing.T) {
	exerciseNativeAccountingCallsites(t, false)
}

func TestUsageHistoryNativeAccountingCallsitesFreshPostgres(t *testing.T) {
	exerciseNativeAccountingCallsites(t, true)
}

func exerciseNativeAccountingCallsites(t *testing.T, postgres bool) {
	t.Helper()
	f := newNativeTurnsFixture(t, postgres, 4)
	sources := nativeTurnsSourceSnapshot(t, f.repo)
	require.Len(t, sources, 4)
	for _, row := range sources {
		require.Nil(t, row.Measurement.Turns, "two responses for root A, root B, and missing Kandev turn identity all lack an additive turn amount")
		require.EqualValues(t, 0, *row.Measurement.ReasoningTokens, "accepted raw thought evidence stays intact for native digest/replay")
		require.EqualValues(t, 5, *row.Measurement.ReasoningOutputTokens)
	}
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	for range 2 {
		facts, _, err := f.repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace})
		require.NoError(t, err)
		require.Len(t, facts, 4)
		for _, fact := range facts {
			require.Nil(t, fact.Measurement.Turns)
			require.EqualValues(t, 5, *fact.Measurement.ReasoningTokens)
			require.True(t, fact.Measurement.ReasoningInsideOutput)
			require.EqualValues(t, 10, *fact.Measurement.OutputTokens)
			require.EqualValues(t, 20, *fact.Measurement.TotalTokens)
			require.EqualValues(t, 20, *fact.Measurement.SourceTotalTokens)
		}
	}
	report, err := f.repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: usageWorkFallback})
	require.NoError(t, err)
	require.Nil(t, report.Totals.Turns)
	require.EqualValues(t, 20, *report.Totals.ReasoningTokens)
	require.EqualValues(t, 80, *report.Totals.TotalTokens)
	require.EqualValues(t, 40, *report.Totals.OutputTokens)
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	var wire struct {
		Totals struct {
			Reasoning *int64 `json:"reasoning_tokens"`
			Total     *int64 `json:"total_tokens"`
			Turns     *int64 `json:"turns"`
		} `json:"totals"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.EqualValues(t, 20, *wire.Totals.Reasoning)
	require.EqualValues(t, 80, *wire.Totals.Total)
	require.Nil(t, wire.Totals.Turns)
	for _, group := range []string{usageGroupModel, usageGroupDay} {
		grouped, err := f.repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: group})
		require.NoError(t, err)
		for _, row := range grouped.Rows {
			require.Nil(t, row.Turns, group)
		}
	}
}

func TestUsageHistoryNativeAccountingCallsitesLegacyPublishedReadGuard(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 4)
	f.addHostTurns(t)
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	poisonPublishedNativeTurns(t, f.repo)
	facts, _, err := f.repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace})
	require.NoError(t, err)
	for _, fact := range facts {
		if nativeCanonicalTokenSource(fact) {
			require.Nil(t, fact.Measurement.Turns)
			require.EqualValues(t, 5, *fact.Measurement.ReasoningTokens, "an old canonical payload still carries the private raw subset")
		} else {
			require.EqualValues(t, 7, *fact.Measurement.Turns)
		}
	}
	filter := models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, GroupBy: usageWorkFallback, IncludeUndated: true}
	report, err := f.repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Nil(t, report.Totals.Turns)
	require.Nil(t, report.Totals.ReasoningTokens, "old SQL summaries cannot claim known zero while repair/publication is pending")
	summaries, _, err := f.repo.ListUsageDailySummaries(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace, Timezone: "UTC"})
	require.NoError(t, err)
	for _, summary := range summaries {
		if summary.Source == usageSourceNative && summary.MetricKind == projectionMetricTokens {
			require.Nil(t, summary.Turns)
			require.Nil(t, summary.ReasoningTokens)
		}
	}
	repairAllNativeTurns(t, f.repo, 2)
	report, err = f.repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Nil(t, report.Totals.ReasoningTokens, "source correction is not a publication watermark")
	completeFixtureProjection(t, f.repo)
	report, err = f.repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Nil(t, report.Totals.Turns)
	require.EqualValues(t, 20, *report.Totals.ReasoningTokens)
	require.EqualValues(t, 150, *report.Totals.TotalTokens)
}

func TestUsageHistoryNativeAccountingCallsitesMixedTokenCostSource(t *testing.T) {
	repo, _ := historyContractFixture(t)
	require.NoError(t, repo.ensureNativeUsageAccountingRepairSchema(t.Context()))
	scope := projectionMixedActors(t, repo)
	generation, err := repo.StartUsageHistoryRebuild(t.Context(), scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	poisonPublishedNativeTurns(t, repo)
	facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, usageMixedValue, facts[0].Measurement.Source)
	require.Equal(t, usageSourceNative, facts[0].TokenSource.Source)
	require.NotEqual(t, usageSourceNative, facts[0].CostSource.Source)
	require.Nil(t, facts[0].Measurement.Turns, "the guard must inspect token provenance, not shared measurement.Source")
	require.EqualValues(t, 100, *facts[0].Measurement.TotalTokens)
	require.EqualValues(t, 500, *facts[0].Measurement.CostSubcents)
}

func TestUsageHistoryNativeAccountingCallsitesReasoningUnknownZeroAndMixed(t *testing.T) {
	f := newNativeTurnsFixture(t, false, 0)
	a, err := f.tasks.GetTurn(t.Context(), "native-root-a")
	require.NoError(t, err)
	b, err := f.tasks.GetTurn(t.Context(), "native-root-b")
	require.NoError(t, err)
	for i := range 4 {
		event := nativeTurnsEvent(i, []*taskmodels.Turn{a, b})
		switch i {
		case 1:
			event.TokensThought, event.ReasoningOutputTokens = nil, nil
		case 2:
			event.ReasoningOutputTokens = models.Int64(0)
		case 3:
			event.TokensThought = models.Int64(2)
			event.TokensTotal, event.ReportedTotalTokens = 22, models.Int64(22)
		}
		require.NoError(t, f.tasks.CreateTaskUsageEvent(t.Context(), event))
	}
	finishNativeTurnsBackfill(t, f.repo)
	completeFixtureProjection(t, f.repo)
	facts, _, err := f.repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: nativeTurnsWorkspace})
	require.NoError(t, err)
	require.Len(t, facts, 4)
	for _, fact := range facts {
		m := fact.Measurement
		switch m.SourceRecordID {
		case "native-response-000":
			require.EqualValues(t, 5, *m.ReasoningTokens)
			require.True(t, m.ReasoningInsideOutput)
		case "native-response-001":
			require.Nil(t, m.ReasoningTokens)
		case "native-response-002":
			require.NotNil(t, m.ReasoningTokens)
			require.Zero(t, *m.ReasoningTokens)
			require.True(t, m.ReasoningInsideOutput)
		case "native-response-003":
			require.Nil(t, m.ReasoningTokens)
			require.False(t, m.ReasoningInsideOutput)
			require.EqualValues(t, 22, *m.TotalTokens)
			require.EqualValues(t, 5, *m.ReasoningOutputTokens)
		default:
			t.Fatalf("unexpected native reasoning fixture %q", m.SourceRecordID)
		}
		require.EqualValues(t, 10, *m.OutputTokens)
	}
}
