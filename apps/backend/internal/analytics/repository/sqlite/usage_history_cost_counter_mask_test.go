package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryCostCounterMaskSharedUnpricedAlternatives(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
			require.NoError(t, err)
			tasks.SetUsageHistoryRetainer(repo)
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "shared-task", WorkspaceID: "ws-1", Title: "Shared", CreatedAt: at, UpdatedAt: at}))
			require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "shared-session", TaskID: "shared-task", StartedAt: at, Metadata: map[string]any{"acp": map[string]any{"session_id": "segment-thread"}}}))
			for i, session := range []string{"session-1", "shared-session"} {
				collector := fmt.Sprintf("counter-collector-%d", i)
				binding := segmentBinding(t, repo, collector, session)
				lifetime := contractObservation(t, binding, collector+"-lifetime", 1, "model", 100)
				day := contractObservation(t, binding, collector+"-day", 1, "model", 100)
				day.Coverage = costCounterDayCoverage()
				sealContractObservation(t, &day)
				require.Equal(t, "accepted", reportContractObservation(t, repo, collector, lifetime).Status)
				require.Equal(t, "accepted", reportContractObservation(t, repo, collector, day).Status)
			}
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			for _, group := range []string{"model", "task", "session"} {
				report := assertCostCounterCanonicalParity(t, repo, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: group}, models.Int64(100), nil, 1)
				require.Len(t, report.Rows, 1, "a redundant dated missing-cost counter cannot become a ghost row")
				require.Empty(t, report.Rows[0].TaskID)
				require.Empty(t, report.Rows[0].SessionID)
				require.False(t, report.Rows[0].TaskAvailable)
				require.False(t, report.Rows[0].SessionAvailable)
			}
			assertCostCounterCanonicalParity(t, repo, costCounterDatedFilter(), models.Int64(100), nil, 1)
		})
	}
}

func costCounterDayCoverage() *pluginsdk.UsageCoverageV1 {
	return &pluginsdk.UsageCoverageV1{
		CoverageStart: stringPtr("2026-01-02T00:00:00Z"), CoverageEnd: stringPtr("2026-01-03T00:00:00Z"),
		SourceDate: stringPtr("2026-01-02"), SourceTimezone: stringPtr("UTC"),
	}
}

func costCounterDatedFilter() models.SessionUsageFilter {
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	return models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Timezone: "UTC", Start: &start, End: &end}
}

type costCounterMaskCase struct {
	name         string
	lifetime     bool
	lifetimeCost *pluginsdk.UsageCostV1
	dayCost      *pluginsdk.UsageCostV1
	dayTokens    *int64
	allTokens    *int64
	allCost      *int64
	allRows      int
}

func costCounterMaskCases() []costCounterMaskCase {
	unknown := &pluginsdk.UsageCostV1{Basis: pluginsdk.UsageCostBasisUnknown}
	known := &pluginsdk.UsageCostV1{Amount: models.Int64(20), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	zero := &pluginsdk.UsageCostV1{Amount: models.Int64(0), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	return []costCounterMaskCase{
		{name: "missing containers keep one unknown", lifetime: true, dayTokens: models.Int64(20), allTokens: models.Int64(100), allRows: 1},
		{name: "explicit unknowns keep one unknown", lifetime: true, lifetimeCost: unknown, dayCost: unknown, dayTokens: models.Int64(20), allTokens: models.Int64(100), allRows: 1},
		{name: "unknown lifetime cannot hide known day", lifetime: true, lifetimeCost: unknown, dayCost: known, dayTokens: models.Int64(20), allTokens: models.Int64(100), allCost: models.Int64(20), allRows: 2},
		{name: "known lifetime zero has authority", lifetime: true, lifetimeCost: zero, dayCost: known, dayTokens: models.Int64(20), allTokens: models.Int64(100), allCost: models.Int64(0), allRows: 1},
		{name: "sole all-unknown dated evidence survives", dayCost: unknown, allRows: 1},
		{name: "sole dated zero remains measured", dayCost: zero, dayTokens: models.Int64(20), allTokens: models.Int64(20), allCost: models.Int64(0), allRows: 1},
	}
}

func costCounterObservation(t *testing.T, binding string, amount *int64, cost *pluginsdk.UsageCostV1, daily bool) pluginsdk.UsageObservationV1 {
	t.Helper()
	item := multiModelAdapterObservation(t, binding, "counter-member", "Sonnet", "anthropic", 0, daily)
	item.Tokens = &pluginsdk.UsageTokenAmountsV1{InputTokens: amount, TotalTokens: amount}
	item.Cost = cost
	item.Quality.TokenCoverageComplete = amount != nil
	item.Quality.CostCoverageComplete = cost != nil && cost.Amount != nil
	sealContractObservation(t, &item)
	return item
}

func TestUsageHistoryCostCounterMaskUnknownKnownAndZero(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		for _, test := range costCounterMaskCases() {
			t.Run(fmt.Sprintf("postgres=%t/%s", postgres, test.name), func(t *testing.T) {
				repo := segmentContractFixture(t, postgres)
				binding := segmentBinding(t, repo, "collector-a", "session-1")
				if test.lifetime {
					item := costCounterObservation(t, binding, models.Int64(100), test.lifetimeCost, false)
					reportSegmentBatch(t, repo, "collector-a", item)
				}
				day := costCounterObservation(t, binding, test.dayTokens, test.dayCost, true)
				reportSegmentBatch(t, repo, "collector-a", day)
				finishFixtureRetention(t, repo)
				completeFixtureProjection(t, repo)
				report := assertCostCounterCanonicalParity(t, repo, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"}, test.allTokens, test.allCost, test.allRows)
				require.Len(t, report.Rows, test.allRows)
				var dayCost *int64
				if test.dayCost != nil {
					dayCost = test.dayCost.Amount
				}
				assertCostCounterCanonicalParity(t, repo, costCounterDatedFilter(), test.dayTokens, dayCost, 1)
			})
		}
	}
}

func assertCostCounterCanonicalParity(t *testing.T, repo *Repository, filter models.SessionUsageFilter, tokens, cost *int64, count int) *models.TokenUsageReport {
	t.Helper()
	report, err := repo.ListSessionUsage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.False(t, report.ProjectionPending)
	require.Equal(t, tokens, report.Totals.TotalTokens)
	require.Equal(t, cost, report.Totals.CostSubcents)
	rows, state, err := repo.ListRetainedCanonicalUsagePage(t.Context(), filter)
	require.NoError(t, err)
	require.Equal(t, report.ProjectionGeneration, state.Generation)
	require.Len(t, rows, count)
	var tokenSum, costSum int64
	tokenKnown, costKnown, tokenApplies, costApplies := true, true, false, false
	for _, row := range rows {
		if !row.TokenMetricAbsent {
			tokenApplies = true
			if row.TotalTokens == nil {
				tokenKnown = false
			} else {
				tokenSum += *row.TotalTokens
			}
		}
		if !row.CostMetricAbsent {
			costApplies = true
			if row.CostSubcents == nil {
				costKnown = false
			} else {
				costSum += *row.CostSubcents
			}
		}
	}
	if tokens == nil {
		require.True(t, tokenApplies, "genuinely unknown token evidence cannot be removed")
		require.False(t, tokenKnown)
	} else {
		require.True(t, tokenApplies && tokenKnown)
		require.Equal(t, *tokens, tokenSum)
	}
	if cost == nil {
		require.True(t, costApplies, "an unknown applicable cost is not an absent metric")
		require.False(t, costKnown)
	} else {
		require.True(t, costApplies && costKnown)
		require.Equal(t, *cost, costSum)
	}
	return report
}

func TestUsageHistoryCostCounterMaskPreservesCrossActorSplitMetrics(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			retainSegmentNative(t, repo, 100, func(native *usagehistory.NativeUsage) {
				native.ReportedTotalTokens, native.CostPresent, native.CostSubcents = models.Int64(100), true, 10
				native.CostSource, native.CostBasis, native.Estimated = usageCostSourceEstimated, models.UsageCostBasisEstimated, true
			})
			costCounterSecondActor(t, repo)
			binding := segmentBinding(t, repo, "collector-a", "session-1")
			cost := &pluginsdk.UsageCostV1{Amount: models.Int64(20), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
			day := costCounterObservation(t, binding, nil, cost, true)
			day.Tokens = nil
			sealContractObservation(t, &day)
			reportSegmentBatch(t, repo, "collector-a", day)
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)
			report := assertCostCounterCanonicalParity(t, repo, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"}, models.Int64(100), models.Int64(20), 2)
			require.Nil(t, report.Totals.EffectiveCostPerMillion, "aggregate cost cannot be allocated to one native actor")
			facts, _, err := repo.ListUsageCanonicalFacts(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
			require.NoError(t, err)
			var tokenOnly, costOnly int
			for _, fact := range facts {
				if fact.CostMetricAbsent {
					tokenOnly++
					require.Equal(t, "native-actor", fact.TokenSource.ActorID)
				}
				if fact.TokenMetricAbsent {
					costOnly++
					require.Equal(t, "mixed", fact.CostSource.ActorStatus)
					require.Empty(t, fact.CostSource.ActorID)
				}
			}
			require.Equal(t, 1, tokenOnly)
			require.Equal(t, 1, costOnly)
		})
	}
}

func costCounterSecondActor(t *testing.T, repo *Repository) {
	t.Helper()
	var taskIncarnation, sessionIncarnation string
	require.NoError(t, repo.db.QueryRowx(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE task_id='task-1' AND session_id='session-1'`).Scan(&taskIncarnation, &sessionIncarnation))
	at := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
		ID: "counter-second-actor", Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1",
		TaskIncarnationID: taskIncarnation, SessionID: "session-1", SessionIncarnationID: sessionIncarnation,
		ActorID: "another-actor", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at,
	}))
	require.NoError(t, tx.Commit())
}
