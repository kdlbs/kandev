package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/common/usagehistory"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

func reportJSON(t *testing.T, report *models.TokenUsageReport) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(report)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(encoded, &result))
	return result
}

// @covers AC-COSTS-USAGE-HISTORY-006.5 AC-COSTS-USAGE-HISTORY-005.3
func TestUsageHistoryReportSavedPublication(t *testing.T) {
	repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
	ctx := context.Background()
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"}
	report, err := repo.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, "preparing", reportJSON(t, report)["projection_status"])
	require.Empty(t, report.Rows)
	generation, err := driver.StartUsageHistoryRebuild(ctx, scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	completeReportMigration(t, repo)
	report, err = repo.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
	first := reportJSON(t, report)["projection_generation"]
	require.NotEmpty(t, first)
	item := contractObservation(t, binding, "projection-item", 2, "new-model", 40)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", item).Status)
	report, err = repo.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
	require.Equal(t, first, reportJSON(t, report)["projection_generation"])
	require.Equal(t, true, reportJSON(t, report)["projection_pending"])
	drainProjection(t, driver, scope, generation)
	report, err = repo.ListSessionUsage(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 40, *report.Totals.TotalTokens)
	require.NotEqual(t, first, reportJSON(t, report)["projection_generation"])
}

// @covers AC-COSTS-USAGE-HISTORY-002.3 AC-COSTS-USAGE-HISTORY-007.4 AC-COSTS-USAGE-HISTORY-003.7
func TestUsageHistoryReportDeletedIndependentActorsAndExactWire(t *testing.T) {
	repo, _ := historyContractFixture(t)
	scope := projectionMixedActors(t, repo)
	generation, err := repo.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	completeReportMigration(t, repo)
	_, err = repo.db.Exec(`DELETE FROM task_sessions`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`DELETE FROM tasks`)
	require.NoError(t, err)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "task"})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
	require.EqualValues(t, 500, *report.Totals.CostSubcents)
	row := reportJSON(t, report)["rows"].([]any)[0].(map[string]any)
	require.Equal(t, false, row["task_available"])
	require.Equal(t, "actor-a", row["token_source"].(map[string]any)["actor_id"])
	require.Equal(t, "mixed", row["cost_source"].(map[string]any)["actor_status"])
	require.Nil(t, row["effective_cost_per_million"])
}

func seedReportPoints(t *testing.T, repo *Repository, points []string) string {
	t.Helper()
	page, err := repo.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, point := range points {
		item := contractObservation(t, page.Targets[0].SubjectBinding, point, 1, "model", int64(i+1)*10)
		item.ModelProvider = stringPtr("provider")
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: stringPtr(point)}
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	}
	var scope string
	require.NoError(t, repo.db.Get(&scope, `SELECT scope_id FROM usage_scopes LIMIT 1`))
	generation, err := repo.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, repo, scope, generation)
	completeReportMigration(t, repo)
	return generation
}

// @covers AC-COSTS-USAGE-HISTORY-005.1 AC-COSTS-USAGE-HISTORY-005.3 AC-COSTS-USAGE-HISTORY-005.4
func TestUsageHistoryReportColdCalendarAndCompleteTotals(t *testing.T) {
	repo, _ := historyContractFixture(t)
	seedReportPoints(t, repo, []string{"2026-03-28T20:00:00Z", "2026-03-29T19:00:00Z", "2026-03-29T20:00:00Z"})
	start := time.Date(2026, 3, 28, 18, 15, 0, 0, time.UTC)
	end := time.Date(2026, 3, 30, 18, 15, 0, 0, time.UTC)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Timezone: "Asia/Kathmandu", Start: &start, End: &end, Limit: 1, SortBy: "period", SortDirection: "asc"})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, "2026-03-29", report.Rows[0].Period)
	require.EqualValues(t, 10, *report.Rows[0].TotalTokens)
	require.EqualValues(t, 60, *report.Totals.TotalTokens)
	require.Equal(t, 2, report.TotalRows)
	require.True(t, report.HasMore)
	require.Equal(t, []string{"provider"}, report.Providers)
}

// @covers AC-COSTS-USAGE-HISTORY-003.7 AC-COSTS-USAGE-HISTORY-005.3
func TestUsageHistoryReportDecimalCurrenciesAndRateSort(t *testing.T) {
	repo, api := historyContractFixture(t)
	targets, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	for i, currency := range []string{"USD", "EUR"} {
		point := []string{"2026-03-29T01:00:00Z", "2026-03-29T02:00:00Z"}[i]
		item := contractObservation(t, targets.Targets[0].SubjectBinding, point, 1, "model", 9007199254740993)
		item.ModelProvider = stringPtr("provider")
		item.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "provider_event", ID: point}
		item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
		item.Cost = &pluginsdk.UsageCostV1{Amount: models.Int64(12345), Currency: &currency, Basis: pluginsdk.UsageCostBasisReported}
		item.Quality.CostCoverageComplete = true
		sealContractObservation(t, &item)
		require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	}
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.NoError(t, err)
	require.Nil(t, report.Totals.CostSubcents, "different currencies cannot be added")
	require.Nil(t, report.Totals.EffectiveCostPerMillion, "a multi-currency denominator is unavailable")
	wire := reportJSON(t, report)["totals"].(map[string]any)
	require.Nil(t, wire["total_tokens"], "an unsafe numeric field must be null")
	require.Equal(t, "18014398509481986", wire["amounts_decimal"].(map[string]any)["total_tokens"])
	currencies := map[string]bool{}
	for _, row := range report.Rows {
		if row.CostSubcents != nil {
			currencies[row.Currency] = true
			require.EqualValues(t, 12345, *row.CostSubcents)
		}
	}
	require.Equal(t, map[string]bool{"USD": true, "EUR": true}, currencies)
}

func TestUsageHistoryReportForeignCoarseCalendarBoundary(t *testing.T) {
	repo, api := historyContractFixture(t)
	targets, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, targets.Targets[0].SubjectBinding, "source-day", 1, "model", 100)
	start, end, date, calendar := "2026-03-29T00:00:00Z", "2026-03-29T23:00:00Z", "2026-03-29", "Europe/Lisbon"
	item.Coverage = &pluginsdk.UsageCoverageV1{CoverageStart: &start, CoverageEnd: &end, SourceDate: &date, SourceTimezone: &calendar}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	from := time.Date(2026, 3, 29, 6, 0, 0, 0, time.UTC)
	until := from.Add(time.Hour)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Timezone: "Asia/Kathmandu", Start: &from, End: &until})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.EqualValues(t, 100, *report.Rows[0].TotalTokens, "a coarse day must not be proportionally allocated")
	require.Equal(t, "Europe/Lisbon", report.Rows[0].Calendar)
	require.Equal(t, "source", report.Rows[0].CalendarBasis)
	require.Equal(t, "partial", report.Rows[0].Coverage)
	require.Nil(t, report.Rows[0].EffectiveCostPerMillion)
}

func TestUsageHistoryReportArchivedIncarnationAndPageTotals(t *testing.T) {
	repo, _ := historyContractFixture(t)
	seedReportPoints(t, repo, []string{"2026-03-29T01:00:00Z", "2026-03-29T02:00:00Z"})
	_, err := repo.db.Exec(`UPDATE tasks SET archived_at='2026-03-30T00:00:00Z' WHERE id='task-1'`)
	require.NoError(t, err)
	filter := models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "task", Offset: 99, Limit: 1}
	report, err := repo.ListSessionUsage(context.Background(), filter)
	require.NoError(t, err)
	require.Empty(t, report.Rows)
	require.EqualValues(t, 30, *report.Totals.TotalTokens)
	require.Equal(t, 1, report.TotalRows)
	filter.Offset = 0
	report, err = repo.ListSessionUsage(context.Background(), filter)
	require.NoError(t, err)
	wire := reportJSON(t, report)["rows"].([]any)[0].(map[string]any)
	require.Equal(t, true, wire["task_available"])
	require.Equal(t, true, wire["task_archived"])
	_, err = repo.db.Exec(`UPDATE tasks SET created_at='2026-03-31T00:00:00Z' WHERE id='task-1'`)
	require.NoError(t, err)
	report, err = repo.ListSessionUsage(context.Background(), filter)
	require.NoError(t, err)
	wire = reportJSON(t, report)["rows"].([]any)[0].(map[string]any)
	require.Equal(t, false, wire["task_available"], "reused live IDs cannot make historical incarnations navigable")
}

func TestUsageHistoryReportSortsAuthoritativeRates(t *testing.T) {
	repo, _ := historyContractFixture(t)
	ctx := context.Background()
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	for i, model := range []string{"high-rate", "low-rate"} {
		tokens, cost := int64(100), int64(500)
		if i == 1 {
			tokens, cost = 10000, 1000
		}
		point := time.Date(2026, 3, 29, 1+i, 0, 0, 0, time.UTC)
		require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{UsageEventID: model, TaskID: "task-1", SessionID: "session-1", Model: model, Provider: "provider", InputTokens: tokens, TotalTokens: tokens, UsageCompleteness: "complete", CostPresent: true, CostBasis: "reported", CostSubcents: cost, OccurredAt: point, CollectedAt: point}))
	}
	require.NoError(t, tx.Commit())
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", SortBy: "rate", SortDirection: "desc", Limit: 1})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, "high-rate", report.Rows[0].Model)
	require.NotNil(t, report.Rows[0].EffectiveCostPerMillion)
	require.Equal(t, float64(500), *report.Rows[0].EffectiveCostPerMillion)
}

func TestUsageHistoryReportWaitsForMigrationWatermarks(t *testing.T) {
	repo, driver, scope := projectionFixture(t)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	_, err = repo.db.Exec(`INSERT INTO usage_history_migration_state(source_key,high_water,last_key,complete,updated_at) VALUES ('native','100','20',0,CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, "preparing", report.ProjectionStatus)
	require.Empty(t, report.Rows, "a manually built projection cannot cut over incomplete retention")
	completeReportMigration(t, repo)
	report, err = repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, "ready", report.ProjectionStatus)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
}

func completeReportMigration(t *testing.T, repo *Repository) {
	t.Helper()
	for _, source := range []string{"native", "measurements", "buckets"} {
		_, err := repo.db.Exec(`INSERT INTO usage_history_migration_state(source_key,high_water,last_key,complete,updated_at) VALUES (?,'','',1,CURRENT_TIMESTAMP) ON CONFLICT(source_key) DO UPDATE SET complete=1`, source)
		require.NoError(t, err)
	}
}

func TestUsageHistoryReportExcludesStagingAndNeverReadsObservations(t *testing.T) {
	repo, _ := historyContractFixture(t)
	seedReportPoints(t, repo, []string{"2026-03-29T01:00:00Z"})
	_, err := repo.db.Exec(`INSERT INTO usage_canonical_facts(scope_id,generation,build_id,fact_key,work_key,subject_id,task_id,session_id,coverage_kind,coverage_start,coverage_end,source_date,calendar,model,provider,token_source,cost_source,payload,collected_at) SELECT scope_id,generation,'unpublished',fact_key,work_key,subject_id,task_id,session_id,coverage_kind,coverage_start,coverage_end,source_date,calendar,model,provider,token_source,cost_source,payload,collected_at FROM usage_canonical_facts`)
	require.NoError(t, err)
	_, err = repo.db.Exec(`INSERT INTO usage_projection_contributions SELECT scope_id,generation,'unpublished',fact_key,work_key,summary_key,coverage_kind,coverage_start,coverage_end,metric_kind,actor_id,actor_status,actor_kind,source,model,provider,agent_client,semantics,currency,calendar,usage_date,input_tokens,output_tokens,cache_read_tokens,cache_write_tokens,reasoning_tokens,900000,source_total_tokens,turns,cost_subcents,rate_tokens,partial,estimated,stale,cost_basis FROM usage_projection_contributions`)
	require.NoError(t, err)
	ctx := context.Background()
	conn, err := repo.db.Conn(ctx)
	require.NoError(t, err)
	require.NoError(t, conn.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(op int, table, _, _ string) int {
			if op == sqlite3.SQLITE_READ && (table == "session_usage_measurements" || table == "session_usage_buckets" || table == "task_usage_events") {
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	}))
	require.NoError(t, conn.Close())
	t.Cleanup(func() {
		connection, err := repo.db.Conn(ctx)
		if err != nil {
			return
		}
		_ = connection.Raw(func(raw any) error { raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(nil); return nil })
		_ = connection.Close()
	})
	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "task"})
	require.NoError(t, err)
	require.EqualValues(t, 10, *report.Totals.TotalTokens)
	require.Len(t, report.Rows, 1)
}

func TestUsageHistoryReportUnknownProviderFilterAgreesWithOptions(t *testing.T) {
	repo, api := historyContractFixture(t)
	page, err := api.ListUsageCollectionTargets(context.Background(), "collector", pluginsdk.UsageCollectionTargetQueryV1{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	item := contractObservation(t, page.Targets[0].SubjectBinding, "unknown-provider", 1, "exact-model", 27)
	point := "2026-03-29T01:00:00Z"
	item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &point}
	sealContractObservation(t, &item)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector", item).Status)
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model", Provider: "__unknown_provider__"})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.EqualValues(t, 27, *report.Totals.TotalTokens)
	require.Equal(t, []string{"__unknown_provider__"}, report.Providers)
	require.Empty(t, report.Rows[0].Provider, "an exact model label must not invent a provider")
}

func TestUsageHistoryReportRejectsChangedPagePublication(t *testing.T) {
	repo, driver, scope, binding := projectionFixtureWithIssuedBinding(t)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	completeReportMigration(t, repo)
	first, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1"})
	require.NoError(t, err)
	require.Equal(t, "accepted", reportContractObservation(t, repo, "collector-a", contractObservation(t, binding, "projection-item", 2, "model", 40)).Status)
	drainProjection(t, driver, scope, generation)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", Offset: 1, ProjectionGeneration: first.ProjectionGeneration})
	require.Error(t, err, "a later page must not silently change the dataset publication")
	require.Nil(t, report)
}

func TestUsageHistoryReportLifetimeSummaryRemainsUndated(t *testing.T) {
	repo, driver, scope := projectionFixture(t)
	generation, err := driver.StartUsageHistoryRebuild(context.Background(), scope)
	require.NoError(t, err)
	drainProjection(t, driver, scope, generation)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(context.Background(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day"})
	require.NoError(t, err)
	require.Empty(t, report.Rows)
	require.True(t, report.Totals.Undated)
	require.False(t, report.HasRangeData)
	require.EqualValues(t, 100, *report.Totals.TotalTokens)
}

func TestUsageHistoryReportMixedBasisAndMonthlyRateAvailability(t *testing.T) {
	repo, _ := historyContractFixture(t)
	ctx := context.Background()
	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	for i, month := range []time.Month{time.January, time.February} {
		point := time.Date(2026, month, 20, 12, 0, 0, 0, time.UTC)
		cost, basis := int64(500), "reported"
		if i == 1 {
			cost, basis = 100, "estimated"
		}
		require.NoError(t, repo.RetainNativeUsageTx(ctx, tx, usagehistory.NativeUsage{UsageEventID: month.String(), TaskID: "task-1", SessionID: "session-1", Model: "model", Provider: "provider", InputTokens: 100, TotalTokens: 100, UsageCompleteness: "complete", CostPresent: true, CostBasis: basis, CostSubcents: cost, OccurredAt: point, CollectedAt: point}))
	}
	require.NoError(t, tx.Commit())
	completeFixtureProjection(t, repo)
	completeReportMigration(t, repo)
	report, err := repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "model"})
	require.NoError(t, err)
	require.Equal(t, "mixed", report.Totals.CostBasis)
	start, end := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC), time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	report, err = repo.ListSessionUsage(ctx, models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "month", Start: &start, End: &end, SortBy: "rate", SortDirection: "desc", Limit: 1})
	require.NoError(t, err)
	require.Len(t, report.Rows, 1)
	require.Equal(t, "2026-02", report.Rows[0].Period, "the clipped January rate is unavailable before sorting and pagination")
	require.NotNil(t, report.Rows[0].EffectiveCostPerMillion)
}

func TestUsageHistoryReportChartDoesNotReadLiveTaskRows(t *testing.T) {
	repo, _ := historyContractFixture(t)
	seedReportPoints(t, repo, []string{"2026-03-29T01:00:00Z"})
	conn, err := repo.db.Connx(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	err = conn.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(action int, table, _, _ string) int {
			if action == sqlite3.SQLITE_READ && table == "tasks" {
				return sqlite3.SQLITE_DENY
			}
			return sqlite3.SQLITE_OK
		})
		return nil
	})
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	report, err := repo.ListSessionUsage(t.Context(), models.SessionUsageFilter{WorkspaceID: "ws-1", GroupBy: "day", Timezone: "Asia/Kathmandu"})
	require.NoError(t, err, "chart rows do not identify a live task and cannot perform a task lookup per grouped row")
	require.Len(t, report.Rows, 1)
	require.EqualValues(t, 10, *report.Totals.TotalTokens)
	require.Empty(t, report.Rows[0].TaskID)
	require.Empty(t, report.Rows[0].SessionID)
}
