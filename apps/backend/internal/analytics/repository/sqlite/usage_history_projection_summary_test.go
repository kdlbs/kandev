package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func projectionMixedActors(t *testing.T, repo *Repository) string {
	t.Helper()
	ctx := context.Background()
	point := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	for _, actor := range []string{"actor-a", "actor-b"} {
		require.NoError(t, repo.AdmitBindingTx(ctx, tx, usagehistory.Binding{ID: actor, Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1", TaskIncarnationID: "2026-01-01T00:00:00Z", SessionID: "session-1", SessionIncarnationID: "session-1", ActorID: actor, ActorStatus: "attributed", ActorKind: "user", ActorSource: "authenticated-execution", CapturedAt: point}))
	}
	require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{BindingID: "actor-a", UsageEventID: "native-actor-a", TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "provider", InputTokens: 100, OutputTokens: models.Int64(0), TotalTokens: 100, UsageCompleteness: "complete", OccurredAt: point, CollectedAt: point}))
	require.NoError(t, tx.Commit())
	targets, err := repo.ListUsageCollectionTargets(ctx, "collector-a", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	cost := contractObservation(t, targets.Targets[0].SubjectBinding, "reported-cost", 1, "model", 150)
	cost.ModelProvider = stringPtr("provider")
	cost.Quality.TokenCoverageComplete, cost.Quality.CostCoverageComplete = false, true
	cost.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(500), Currency: stringPtr("USD"), Basis: pluginsdk.UsageCostBasisReported}
	cost.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr(point.Format(time.RFC3339Nano))}
	sealContractObservation(t, &cost)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", cost).Status)
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	return scope
}

// @covers AC-COSTS-USAGE-HISTORY-006.1 AC-COSTS-USAGE-HISTORY-006.6
func TestUsageHistorySummaryIndependentMetricActors(t *testing.T) {
	repo, _ := historyContractFixture(t)
	scope := projectionMixedActors(t, repo)
	generation, err := repo.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	var tokens, cost int64
	require.NoError(t, repo.db.Get(&tokens, `SELECT SUM(total_tokens) FROM usage_daily_summaries WHERE actor_id = 'actor-a' AND metric_kind = 'tokens'`))
	require.EqualValues(t, 100, tokens)
	require.NoError(t, repo.db.Get(&cost, `SELECT SUM(cost_subcents) FROM usage_daily_summaries WHERE actor_status = 'mixed' AND metric_kind = 'cost'`))
	require.EqualValues(t, 500, cost)
	var duplicated int
	require.NoError(t, repo.db.Get(&duplicated, `SELECT COUNT(*) FROM usage_daily_summaries WHERE (metric_kind = 'cost' AND total_tokens IS NOT NULL) OR (metric_kind = 'tokens' AND cost_subcents IS NOT NULL)`))
	require.Zero(t, duplicated)
	var rate sql.NullInt64
	require.NoError(t, repo.db.Get(&rate, `SELECT rate_tokens FROM usage_daily_summaries WHERE metric_kind = 'cost'`))
	require.False(t, rate.Valid, "mixed-user cost has no allocated token denominator")
	var payload string
	require.NoError(t, repo.db.Get(&payload, `SELECT payload FROM usage_canonical_facts LIMIT 1`))
	var fact models.UsageCanonicalFact
	require.NoError(t, decodeUsageProjection(payload, &fact))
	require.NotNil(t, fact.Measurement.OutputTokens, "measured zero must survive durable encoding")
	require.Zero(t, *fact.Measurement.OutputTokens)
	facts, _, err := repo.ListUsageCanonicalFacts(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.Equal(t, "actor-a", facts[0].TokenSource.ActorID)
	require.Equal(t, "mixed", facts[0].CostSource.ActorStatus)
	summaries, metadata, err := repo.ListUsageDailySummaries(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", Timezone: "UTC"})
	require.NoError(t, err)
	require.True(t, metadata.HasDated)
	require.Len(t, summaries, 2)
	for _, summary := range summaries {
		if summary.MetricKind == "tokens" {
			require.NotNil(t, summary.OutputTokens)
			require.Zero(t, *summary.OutputTokens)
			require.EqualValues(t, 1, summary.CacheReadMissing)
			require.Nil(t, summary.CacheReadTokens)
		} else {
			require.Nil(t, summary.EffectiveCostPerMillion())
		}
	}
}

func TestUsageHistoryAllTimeProviderSummaryPreservesInt64(t *testing.T) {
	for _, postgres := range []bool{false, true} {
		t.Run(fmt.Sprintf("postgres=%t", postgres), func(t *testing.T) {
			repo := segmentContractFixture(t, postgres)
			_, err := NewWithDB(repo.db, repo.ro)
			require.NoError(t, err, "projection/report indexes must replay on the same database")
			segmentBinding(t, repo, "precision-collector", "session-1")
			var taskIncarnation, sessionIncarnation string
			query := repo.db.Rebind(`SELECT task_incarnation_id,session_incarnation_id FROM usage_subjects WHERE task_id=? AND session_id=?`)
			require.NoError(t, repo.db.QueryRowx(query, "task-1", "session-1").Scan(&taskIncarnation, &sessionIncarnation))
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			tx, err := repo.db.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = tx.Rollback() }()
			require.NoError(t, repo.AdmitBindingTx(t.Context(), tx, usagehistory.Binding{
				ID: "precision-native-execution", Kind: usagehistory.BindingKindExecution, WorkspaceID: "ws-1", TaskID: "task-1",
				TaskIncarnationID: taskIncarnation, SessionID: "session-1", SessionIncarnationID: sessionIncarnation,
				ActorID: "precision-actor", ActorStatus: "attributed", ActorKind: "user", ActorSource: "execution", CapturedAt: at,
			}))
			for index, amount := range []int64{math.MaxInt64 - 1, 1} {
				point := at.AddDate(0, 0, index+1).Add(12 * time.Hour)
				native := usagehistory.NativeUsage{
					BindingID: "precision-native-execution", UsageEventID: fmt.Sprintf("precision-native-%d", index),
					TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "native-provider",
					InputTokens: amount, OutputTokens: models.Int64(0), TotalTokens: amount, UsageCompleteness: "complete",
					CostPresent: true, CostBasis: models.UsageCostBasisReported, CostSubcents: 1, OccurredAt: point, CollectedAt: point,
				}
				require.NoError(t, repo.RetainNativeUsageTx(t.Context(), tx, native))
			}
			require.NoError(t, tx.Commit())
			finishFixtureRetention(t, repo)
			completeFixtureProjection(t, repo)

			filter := models.SessionUsageFilter{WorkspaceID: "ws-1", Timezone: "America/New_York", GroupBy: "model", Provider: "native-provider"}
			var scope, generation string
			require.NoError(t, repo.db.Get(&scope, repo.db.Rebind(`SELECT scope_id FROM usage_projection_scopes LIMIT 1`)))
			require.NoError(t, repo.db.Get(&generation, repo.db.Rebind(`SELECT active_generation FROM usage_projection_scopes WHERE scope_id=?`), scope))
			proof, err := repo.db.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			defer func() { _ = proof.Rollback() }()
			safe, err := repo.usageReportAllTimeSummarySafe(t.Context(), proof, scope, generation, filter)
			require.NoError(t, err)
			require.True(t, safe, "this provider report must exercise the all-time native summary")
			require.NoError(t, proof.Rollback())

			report, err := repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.Len(t, report.Rows, 1)
			require.Equal(t, "native-provider", report.Rows[0].Provider)
			require.Equal(t, models.Int64(math.MaxInt64), report.Totals.TotalTokens)
			require.Equal(t, models.Int64(2), report.Totals.CostSubcents)
		})
	}
}

func TestUsageHistorySummaryRateMatchesIndependentSources(t *testing.T) {
	fact := models.UsageCanonicalFact{
		Measurement: models.SessionUsageMeasurement{Coverage: "complete", CostCoverage: "complete", CostBasis: "reported", Currency: "USD", TotalTokens: models.Int64(100), CostSubcents: models.Int64(500)},
		TokenSource: &models.UsageMetricSource{Source: "native", ItemID: "native-event", ActorID: "user", ActorStatus: "attributed", ActorKind: "user"},
		CostSource:  &models.UsageMetricSource{Source: "plugin:collector", ItemID: "external-event", ActorID: "user", ActorStatus: "attributed", ActorKind: "user"},
	}
	require.True(t, usageProjectionRateAvailable(fact), "complete equivalent coverage may select different token and cost sources")
	fact.Partial = true
	require.False(t, usageProjectionRateAvailable(fact))
}

func TestUsageHistorySummaryPreservesConsumptionCalendars(t *testing.T) {
	point := time.Date(2026, 3, 28, 20, 0, 0, 0, time.UTC)
	fact := models.UsageCanonicalFact{CoverageKind: "point", Measurement: models.SessionUsageMeasurement{CoverageStart: &point, SourceTimezone: "Asia/Kathmandu"}}
	periods, err := usageProjectionPeriods(fact)
	require.NoError(t, err)
	require.Equal(t, []usageProjectionPeriod{{Calendar: "UTC", Date: "2026-03-28"}, {Calendar: "Asia/Kathmandu", Date: "2026-03-29"}}, periods)
	fact.CoverageKind, fact.Measurement.SourceDate, fact.Measurement.SourceTimezone = "calendar", "2026-03-29", "Europe/Lisbon"
	periods, err = usageProjectionPeriods(fact)
	require.NoError(t, err)
	require.Equal(t, []usageProjectionPeriod{{Calendar: "Europe/Lisbon", Date: "2026-03-29"}}, periods, "a coarse DST source day cannot be split into UTC days")
	fact.CoverageKind = "lifetime"
	periods, err = usageProjectionPeriods(fact)
	require.NoError(t, err)
	require.Equal(t, []usageProjectionPeriod{{}}, periods, "collection time must never create a lifetime consumption date")
}
