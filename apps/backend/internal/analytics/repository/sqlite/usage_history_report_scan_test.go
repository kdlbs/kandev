//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-COSTS-USAGE-HISTORY-005.2 AC-COSTS-USAGE-HISTORY-005.5
func TestUsageHistoryReportRecentVisitsExcludeUnrelatedYears(t *testing.T) {
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{Native: 10_000, External: 1_000})
	fixture.seedRecent(t)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, observer))
	type measuredReport struct {
		report *models.TokenUsageReport
		visits int64
	}
	before := map[string]measuredReport{}
	for _, scenario := range fixture.cases {
		if !strings.HasPrefix(scenario.Name, "recent/") {
			continue
		}
		report, statements := observeRetainedReport(t, fixture, observer, scenario)
		before[scenario.Name] = measuredReport{report: report, visits: retainedReportVisits(statements)}
	}
	fixture.seedOlder(t)
	for _, scenario := range fixture.cases {
		baseline, ok := before[scenario.Name]
		if !ok {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			report, statements := observeRetainedReport(t, fixture, observer, scenario)
			require.Equal(t, baseline.report.Rows, report.Rows)
			require.Equal(t, baseline.report.Totals, report.Totals)
			require.Equal(t, baseline.report.Providers, report.Providers)
			visits := retainedReportVisits(statements)
			t.Logf("all report SQL: baseline NVISIT=%d, after four unrelated years NVISIT=%d", baseline.visits, visits)
			for _, statement := range statements {
				for _, loop := range statement.SQLiteLoops {
					if strings.Contains(loop.Name, "idx_usage_contribution") || loop.Name == "idx_usage_daily_date" {
						t.Logf("actual plan: %s; NLOOP=%d NVISIT=%d", loop.Plan, loop.Loops, loop.Visits)
					}
				}
			}
			require.LessOrEqual(t, visits, baseline.visits+128, "all statements, including totals/options/metadata, must exclude unrelated years; measured SQLite NVISIT is not a result-row count")
		})
	}
}

// This scaled reproduction keeps the full C8 query shapes while using the
// ordinary bounded fixture seeder. The independent producer-recipe oracle
// validates every result before the plan assertions report where the work is
// spent. The full one-million-observation benchmark remains the capacity gate.
func TestUsageHistoryDeadlineCasesUseBoundedReportSources(t *testing.T) {
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{Native: 2_000, External: 200})
	fixture.seedRecent(t)
	fixture.seedOlder(t)
	fixture.validateStorage(t, false)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, observer))

	cases := map[string]bool{
		"five_years/America/New_York": false,
		"five_years/Asia/Kathmandu":   false,
		"five_years/Europe/Lisbon":    false,
		"native":                      false,
		"deleted_task":                false,
	}
	for _, scenario := range fixture.cases {
		if _, needed := cases[scenario.Name]; !needed {
			continue
		}
		cases[scenario.Name] = true
		t.Run(scenario.Name, func(t *testing.T) { assertUsageHistoryDeadlineScenario(t, fixture, observer, scenario) })
	}
	for name, found := range cases {
		require.True(t, found, "the scaled reproduction must execute the C8 failed filter %q", name)
	}
}

type usageDeadlineSQLitePlan struct {
	pointVisits, unboundedPointVisits, summaryVisits, lifetimeWorkVisits, lifetimeIndexVisits int64
	subjectFirst, lifetimeIndexPlanned                                                        bool
	indexed                                                                                   []string
}

func assertUsageHistoryDeadlineScenario(t *testing.T, fixture *usageBenchFixture, observer *usageBenchObserver, scenario *usageBenchCase) {
	t.Helper()
	assertUsageHistoryDeadlineSourceParity(t, fixture, scenario)
	_, statements := observeRetainedReport(t, fixture, observer, scenario)
	source := usageHistoryDeadlineReportSource(t, statements, scenario.Name)
	plan := usageHistoryDeadlineSQLitePlanFor(t, source, scenario.Name)
	t.Logf("published facts=%d contributions=%d daily summaries=%d; point-index visits=%d; unbounded point-index visits=%d; daily-summary visits=%d; canonical-work visits=%d; lifetime-index visits=%d; subject-first=%t; indexed loops=%v",
		fixture.storage.PublishedFacts, fixture.storage.PublishedContributions, fixture.storage.DailySummaries,
		plan.pointVisits, plan.unboundedPointVisits, plan.summaryVisits, plan.lifetimeWorkVisits, plan.lifetimeIndexVisits, plan.subjectFirst, plan.indexed)
	assertUsageHistoryDeadlinePlan(t, fixture, scenario, source, plan)
}

func assertUsageHistoryDeadlineSourceParity(t *testing.T, fixture *usageBenchFixture, scenario *usageBenchCase) {
	t.Helper()
	scope, generation := fixture.subjects[0].scope, fixture.subjects[0].generation
	switch scenario.Name {
	case "five_years/America/New_York", "five_years/Asia/Kathmandu", "five_years/Europe/Lisbon":
		requireUsageReportSourcesEqual(t, fixture, scenario.Filter,
			reportSummarySourceFallback(scope, generation, scenario.Filter), reportUTCMonthSummarySource(scope, generation, scenario.Filter))
	case "native":
		tx, err := fixture.database.BeginTxx(t.Context(), nil)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback() }()
		safe, err := fixture.repo.usageReportAllTimeSummarySafe(t.Context(), tx, scope, generation, scenario.Filter)
		require.NoError(t, err)
		require.True(t, safe, "the native all-time summary proof must be exercised")
		require.NoError(t, tx.Rollback())
		requireUsageReportSourcesEqual(t, fixture, scenario.Filter,
			reportFactSource(scope, generation, scenario.Filter, true), reportAllTimeSummarySource(scope, generation, scenario.Filter))
	}
}

func usageHistoryDeadlineReportSource(t *testing.T, statements []usageBenchStatement, scenario string) *usageBenchStatement {
	t.Helper()
	for i := range statements {
		if strings.Contains(statements[i].SQL, "INSERT INTO usage_report_range") ||
			scenario == "deleted_task" && strings.Contains(statements[i].SQL, "SELECT COUNT(*) FROM (WITH base AS") {
			return &statements[i]
		}
	}
	require.FailNow(t, "the captured statement must contain the actual report source")
	return nil
}

func usageHistoryDeadlineSQLitePlanFor(t *testing.T, source *usageBenchStatement, scenario string) usageDeadlineSQLitePlan {
	t.Helper()
	var plan usageDeadlineSQLitePlan
	for _, loop := range source.SQLiteLoops {
		if loop.Name == "idx_usage_contribution_points" && !strings.Contains(loop.Plan, "coverage_start") {
			plan.unboundedPointVisits += loop.Visits
		}
		if scenario == "native" && loop.Name == "idx_usage_contribution_model_provider_points" {
			t.Logf("native point-period plan: %s; NLOOP=%d NVISIT=%d SELECT=%d PARENT=%d", loop.Plan, loop.Loops, loop.Visits, loop.SelectID, loop.ParentID)
		}
		if strings.HasPrefix(loop.Name, "idx_usage_contribution_") && strings.Contains(loop.Plan, "coverage_start") {
			plan.pointVisits += loop.Visits
		}
		plan.lifetimeWorkVisits += loopVisitsWhen(loop.Name == "idx_usage_canonical_work", loop.Visits)
		plan.lifetimeIndexVisits += loopVisitsWhen(loop.Name == "idx_usage_canonical_lifetime", loop.Visits)
		plan.lifetimeIndexPlanned = plan.lifetimeIndexPlanned || loop.Name == "idx_usage_canonical_lifetime_work"
		plan.summaryVisits += loopVisitsWhen(loop.Name == "idx_usage_daily_date", loop.Visits)
		plan.subjectFirst = plan.subjectFirst || loop.Name == "idx_usage_canonical_subject"
		if loop.Visits > 0 && strings.HasPrefix(loop.Name, "idx_usage_") {
			plan.indexed = append(plan.indexed, fmt.Sprintf("%s=%d", loop.Name, loop.Visits))
		}
	}
	return plan
}

func loopVisitsWhen(match bool, visits int64) int64 {
	if match {
		return visits
	}
	return 0
}

func assertUsageHistoryDeadlinePlan(t *testing.T, fixture *usageBenchFixture, scenario *usageBenchCase, source *usageBenchStatement, plan usageDeadlineSQLitePlan) {
	t.Helper()
	switch scenario.Name {
	case "five_years/America/New_York", "five_years/Asia/Kathmandu", "five_years/Europe/Lisbon":
		require.Contains(t, source.SQL, "usage_daily_summaries")
		require.Positive(t, plan.summaryVisits)
		require.Zero(t, plan.unboundedPointVisits, "monthly UTC edge matching must seek bounded calendar-day ranges")
		require.Less(t, plan.pointVisits, fixture.storage.PublishedContributions/4,
			"the report must combine full UTC days from summaries and probe only bounded edge/mixed keys")
	case "native":
		require.Contains(t, source.SQL, "usage_daily_summaries")
		require.Positive(t, plan.summaryVisits)
	case "deleted_task":
		require.True(t, plan.subjectFirst)
		require.True(t, plan.lifetimeIndexPlanned)
		require.Less(t, plan.pointVisits, fixture.storage.PublishedContributions/4)
		require.Less(t, plan.lifetimeWorkVisits, fixture.storage.PublishedFacts/4)
	}
}

func TestUsageHistoryDeadlineCasesPostgres(t *testing.T) {
	if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
		t.Skip("actual PostgreSQL deadline-filter checks require the private test DSN")
	}
	t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
	fixture := newUsageBenchFixture(t, "postgres", usageBenchSize{Native: 2_000, External: 200})
	fixture.seedRecent(t)
	fixture.validateStorage(t, false)
	fixture.seedOlder(t)
	fixture.validateStorage(t, true)

	for _, scenario := range fixture.cases {
		if scenario.Name != "five_years/America/New_York" && scenario.Name != "five_years/Asia/Kathmandu" &&
			scenario.Name != "five_years/Europe/Lisbon" && scenario.Name != "native" && scenario.Name != "deleted_task" {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			scope, generation := fixture.subjects[0].scope, fixture.subjects[0].generation
			switch scenario.Name {
			case "five_years/America/New_York", "five_years/Asia/Kathmandu", "five_years/Europe/Lisbon":
				requireUsageReportSourcesEqual(t, fixture, scenario.Filter,
					reportSummarySourceFallback(scope, generation, scenario.Filter),
					reportUTCMonthSummarySource(scope, generation, scenario.Filter))
			case "native":
				requireUsageReportSourcesEqual(t, fixture, scenario.Filter,
					reportFactSource(scope, generation, scenario.Filter, true),
					reportAllTimeSummarySource(scope, generation, scenario.Filter))
			case "deleted_task":
				requireUsageReportSourcesEqual(t, fixture, scenario.Filter,
					reportFactSource(scope, generation, scenario.Filter, true),
					reportFactSourceForEngine(scope, generation, scenario.Filter, true, true))
			}
			fixture.observer.start()
			fixture.readAndValidate(t, scenario)
			statements, err := fixture.observer.finish()
			require.NoError(t, err)
			require.NoError(t, explainUsageBenchPostgres(t.Context(), fixture.database, statements))
			require.NoError(t, validateUsageBenchStatements("postgres", statements))
			totals, err := usageBenchStatementTotals(statements)
			require.NoError(t, err)
			t.Logf("PostgreSQL report plans: statements=%d reads=%d scan rows=%g rejected=%g",
				totals.Statements, totals.ReadStatements, totals.PostgresRows, totals.PostgresRejected)
			if scenario.Name == "deleted_task" {
				assertPostgresDeletedTaskReportBound(t, fixture, statements)
			}
			usageBenchExplainDeadlineSource(t, fixture, scenario)
		})
	}
}

func usageBenchExplainDeadlineSource(t *testing.T, fixture *usageBenchFixture, scenario *usageBenchCase) {
	t.Helper()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	scope, generation := fixture.subjects[0].scope, fixture.subjects[0].generation
	metadata, err := fixture.repo.usageProjectionMetadataTx(t.Context(), tx, scope)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.usageReportGrid(t.Context(), tx, scenario.Filter, metadata))
	var source usageReportQuery
	switch scenario.Name {
	case "five_years/America/New_York", "five_years/Asia/Kathmandu", "five_years/Europe/Lisbon":
		require.NoError(t, fixture.repo.prepareUsageReportUTCMonthDimensions(t.Context(), tx, scope, generation, scenario.Filter))
		source = reportUTCMonthSummarySource(scope, generation, scenario.Filter)
	case "native":
		safe, err := fixture.repo.usageReportAllTimeSummarySafe(t.Context(), tx, scope, generation, scenario.Filter)
		require.NoError(t, err)
		require.True(t, safe)
		source = reportAllTimeSummarySource(scope, generation, scenario.Filter)
	case "deleted_task":
		source = reportFactSourceForEngine(scope, generation, scenario.Filter, true, true)
	default:
		t.Fatalf("unexpected PostgreSQL deadline scenario %q", scenario.Name)
	}
	query := `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) SELECT COUNT(*),SUM(total_tokens),SUM(cost_subcents),COUNT(DISTINCT actor_id),COUNT(DISTINCT currency) FROM (` + source.sql + `) usage_deadline_source`
	var encoded []byte
	require.NoError(t, tx.GetContext(t.Context(), &encoded, fixture.database.Rebind(query), source.args...))
	statement := usageBenchStatement{SQL: query, PostgresPlan: encoded}
	totals, err := usageBenchStatementTotals([]usageBenchStatement{statement})
	require.NoError(t, err)
	t.Logf("PostgreSQL exact-source plan: scenario=%s scan rows=%g rejected=%g", scenario.Name, totals.PostgresRows, totals.PostgresRejected)
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(encoded, &roots))
	var logScans func(usageBenchPostgresNode)
	logScans = func(node usageBenchPostgresNode) {
		if strings.Contains(node.Type, "Scan") {
			rows, rejected := node.Rows*node.Loops, (node.Filtered+node.Rechecked)*node.Loops
			if rows+rejected >= 1_000 {
				t.Logf("PostgreSQL source scan: scenario=%s type=%s relation=%s index=%s rows=%g rejected=%g loops=%g",
					scenario.Name, node.Type, node.Relation, node.Index, rows, rejected, node.Loops)
			}
		}
		for _, child := range node.Plans {
			logScans(child)
		}
	}
	logScans(roots[0].Plan)
	if scenario.Name == "deleted_task" {
		visits := usageBenchPostgresRelationVisits(roots[0].Plan, "usage_projection_contributions")
		require.Less(t, visits, float64(fixture.storage.PublishedContributions)*0.9,
			"the task-filtered PostgreSQL source must visit selected facts' contributions, not the scope-wide point set")
		require.False(t, usageBenchPostgresHasIndex(roots[0].Plan, "usage_projection_contributions", "idx_usage_contribution_points"),
			"the task-filtered source must not start from the scope-wide point index")
		t.Logf("PostgreSQL deleted-task source contribution visits=%g of %d published contributions", visits, fixture.storage.PublishedContributions)
	}
	if strings.HasPrefix(scenario.Name, "five_years/") {
		loops := usageBenchPostgresRelationMaxLoops(roots[0].Plan, "usage_report_utc_calendar")
		require.LessOrEqual(t, loops, float64(1), "the local-month plan must not rescan the UTC calendar for each point contribution")
		t.Logf("PostgreSQL local-month UTC calendar maximum scan loops=%g", loops)
	}
}

func assertPostgresDeletedTaskReportBound(t *testing.T, fixture *usageBenchFixture, statements []usageBenchStatement) {
	t.Helper()
	var found bool
	for _, statement := range statements {
		if !strings.Contains(statement.SQL, "INSERT INTO usage_report_range") {
			continue
		}
		var roots []struct {
			Plan usageBenchPostgresNode `json:"Plan"`
		}
		require.NoError(t, json.Unmarshal(statement.PostgresPlan, &roots))
		require.Len(t, roots, 1)
		visits := usageBenchPostgresRelationVisits(roots[0].Plan, "usage_projection_contributions")
		require.Less(t, visits, float64(fixture.storage.PublishedContributions)*0.9,
			"the actual report materialization must retain the subject-first contribution bound")
		require.False(t, usageBenchPostgresHasIndex(roots[0].Plan, "usage_projection_contributions", "idx_usage_contribution_points"),
			"the actual report must not start from the scope-wide point index")
		t.Logf("PostgreSQL actual deleted-task report contribution visits=%g of %d published contributions", visits, fixture.storage.PublishedContributions)
		found = true
	}
	require.True(t, found, "the actual report range materialization plan must be captured")
}

func usageBenchPostgresRelationVisits(node usageBenchPostgresNode, relation string) float64 {
	var visits float64
	if node.Relation == relation && strings.Contains(node.Type, "Scan") {
		visits += (node.Rows + node.Filtered + node.Rechecked) * node.Loops
	}
	for _, child := range node.Plans {
		visits += usageBenchPostgresRelationVisits(child, relation)
	}
	return visits
}

func usageBenchPostgresRelationMaxLoops(node usageBenchPostgresNode, relation string) float64 {
	var loops float64
	if node.Relation == relation && strings.Contains(node.Type, "Scan") {
		loops = node.Loops
	}
	for _, child := range node.Plans {
		loops = max(loops, usageBenchPostgresRelationMaxLoops(child, relation))
	}
	return loops
}

func usageBenchPostgresHasIndex(node usageBenchPostgresNode, relation, index string) bool {
	if node.Relation == relation && node.Index == index {
		return true
	}
	for _, child := range node.Plans {
		if usageBenchPostgresHasIndex(child, relation, index) {
			return true
		}
	}
	return false
}

func requireUsageReportSourcesEqual(t *testing.T, fixture *usageBenchFixture, filter models.SessionUsageFilter, before, after usageReportQuery) {
	t.Helper()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	metadata, err := fixture.repo.usageProjectionMetadataTx(t.Context(), tx, fixture.subjects[0].scope)
	require.NoError(t, err)
	require.NoError(t, fixture.repo.usageReportGrid(t.Context(), tx, filter, metadata))
	if usageReportUsesUTCMonthSummaries(filter) {
		require.NoError(t, fixture.repo.prepareUsageReportUTCMonthDimensions(t.Context(), tx, fixture.subjects[0].scope, metadata.generation, filter))
	}
	readGrouped := func(source usageReportQuery) []models.TokenUsageRow {
		grouped := usageReportGrouped(source, filter.GroupBy, false)
		var aggregates []usageReportAggregate
		query := `SELECT * FROM (` + grouped.sql + `) report_groups ORDER BY entity,period,model,provider,calendar,currency,calendar_basis,cost_basis`
		err := tx.SelectContext(t.Context(), &aggregates, fixture.database.Rebind(query), grouped.args...)
		require.NoError(t, err)
		rows := make([]models.TokenUsageRow, 0, len(aggregates))
		for _, aggregate := range aggregates {
			row := usageReportRow(aggregate)
			markUsageReportMonth(&row, filter)
			row.SubjectID, row.TaskID, row.SessionID = "", "", ""
			rows = append(rows, row)
		}
		return rows
	}
	beforeRows, afterRows := readGrouped(before), readGrouped(after)
	if !reflect.DeepEqual(beforeRows, afterRows) {
		t.Logf("grouped output counts before=%d after=%d", len(beforeRows), len(afterRows))
		for i := 0; i < len(beforeRows) && i < len(afterRows) && i < 3; i++ {
			if !reflect.DeepEqual(beforeRows[i], afterRows[i]) {
				beforeJSON, _ := json.Marshal(beforeRows[i])
				afterJSON, _ := json.Marshal(afterRows[i])
				t.Logf("first grouped mismatch index=%d before=%s after=%s", i, beforeJSON, afterJSON)
				break
			}
		}
		t.FailNow()
	}
	readTotals := func(source usageReportQuery) models.UsageTotals {
		var row usageReportAggregate
		err := tx.GetContext(t.Context(), &row, fixture.database.Rebind(`WITH base AS (`+source.sql+`) SELECT `+usageReportTotalsSQL(false)+` FROM base HAVING COUNT(*)>0`), source.args...)
		if err != nil && err != sql.ErrNoRows {
			require.NoError(t, err)
		}
		return usageReportTotals(row)
	}
	require.Equal(t, readTotals(before), readTotals(after), "the optimized source must preserve report totals")
}

func TestUsageHistoryAllTimeProviderOptionsUsePublishedDimensions(t *testing.T) {
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{Native: 10_000, External: 1_000})
	fixture.seedRecent(t)
	fixture.validateStorage(t, false)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, observer))
	var scenario *usageBenchCase
	for _, item := range fixture.cases {
		if item.Name == "native" {
			scenario = item
			break
		}
	}
	require.NotNil(t, scenario)
	observer.start()
	started := time.Now()
	_, statements := observeRetainedReport(t, fixture, observer, scenario)
	queryDuration := time.Since(started)
	var providerQuery *usageBenchStatement
	for i := range statements {
		if strings.Contains(statements[i].SQL, "report_providers") {
			providerQuery = &statements[i]
			break
		}
	}
	require.NotNil(t, providerQuery, "the report must return provider options")
	require.Contains(t, providerQuery.SQL, "usage_projection_provider_options")
	require.NotContains(t, providerQuery.SQL, "usage_projection_contributions")
	require.NotContains(t, providerQuery.SQL, "usage_canonical_facts")
	require.NotContains(t, providerQuery.SQL, "usage_daily_summaries")
	require.NotContains(t, providerQuery.SQL, "usage_lifetime_summaries")
	var visits int64
	var indexed bool
	for _, loop := range providerQuery.SQLiteLoops {
		visits += loop.Visits
		t.Logf("all-time provider-options plan: %s; NLOOP=%d NVISIT=%d", loop.Plan, loop.Loops, loop.Visits)
		indexed = indexed || strings.Contains(loop.Plan, "idx_usage_projection_provider_options_report") || strings.Contains(loop.Plan, "idx_usage_projection_provider_options_work")
	}
	t.Logf("all-time report duration=%s; provider option SQL visits=%d, published provider dimensions=%d, raw contributions=%d", queryDuration, visits, fixture.storage.ProviderOptionDimensions, fixture.storage.PublishedContributions)
	require.True(t, indexed, "all-time provider options must use a scoped provider dimension index")
	require.LessOrEqual(t, visits, fixture.storage.ProviderOptionDimensions+8, "all-time provider options must stay bounded by compact provider dimensions, not point contributions")
}

func TestUsageHistoryAllTimeProviderOptionsPostgres(t *testing.T) {
	if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
		t.Skip("actual PostgreSQL control requires the private test DSN")
	}
	fixture := newUsageBenchFixture(t, "postgres", usageBenchSize{Native: 1_000, External: 100})
	fixture.seedRecent(t)
	var scenario *usageBenchCase
	for _, item := range fixture.cases {
		if item.Name == "native" {
			scenario = item
			break
		}
	}
	require.NotNil(t, scenario)
	fixture.readAndValidate(t, scenario)
}

func TestUsageHistoryReportExactUtcSummarySkipsPointFallback(t *testing.T) {
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{Native: 2_000, External: 200})
	fixture.seedRecent(t)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(context.Background(), fixture.database.DB, observer))
	for _, scenario := range fixture.cases {
		if scenario.Name != "five_years/UTC" {
			continue
		}
		_, statements := observeRetainedReport(t, fixture, observer, scenario)
		var pointVisits int64
		for _, statement := range statements {
			for _, loop := range statement.SQLiteLoops {
				if loop.Name == "idx_usage_contribution_points" {
					pointVisits += loop.Visits
				}
			}
		}
		require.Zero(t, pointVisits, "complete UTC days must read their authoritative summaries without probing the point history")
		return
	}
	t.Fatal("the full UTC calendar scenario was not exercised")
}

// @covers AC-COSTS-USAGE-HISTORY-005.2 AC-COSTS-USAGE-HISTORY-005.5
func TestUsageHistoryReportColdCalendarMatchingScalesWithPoints(t *testing.T) {
	fixture := newUsageBenchFixture(t, "sqlite", usageBenchSize{Native: 2_000, External: 200})
	fixture.seedRecent(t)
	fixture.seedOlder(t)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, observer))
	for _, scenario := range fixture.cases {
		if scenario.Filter.Timezone == "UTC" || (!strings.HasPrefix(scenario.Name, "recent/") && !strings.HasPrefix(scenario.Name, "five_years/")) {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			_, statements := observeRetainedReport(t, fixture, observer, scenario)
			var points, calendar int64
			for _, statement := range statements {
				for _, loop := range statement.SQLiteLoops {
					if strings.HasPrefix(loop.Name, "idx_usage_contribution_") && strings.Contains(loop.Plan, "coverage_start<?") {
						points += loop.Visits
					}
					if strings.Contains(loop.Name, "usage_report_calendar") {
						calendar += loop.Visits
						t.Logf("calendar plan: %s; NLOOP=%d NVISIT=%d", loop.Plan, loop.Loops, loop.Visits)
						if strings.HasPrefix(loop.Plan, "SCAN ") {
							require.LessOrEqual(t, loop.Loops, int64(1), "calendar scans may initialize bounds once, but must not repeat for every point")
						}
					}
				}
			}
			require.Positive(t, points, "the cold calendar must actually match timed contributions")
			days := int64(scenario.Filter.End.Sub(*scenario.Filter.Start)/(24*time.Hour)) + 2
			t.Logf("actual report SQL: point NVISIT=%d calendar NVISIT=%d days=%d", points, calendar, days)
			require.LessOrEqual(t, calendar, 3*points+4*days, "calendar work must grow with points plus grid days, not their product, for both recent and five-year reports")
		})
	}
}

func observeRetainedReport(t *testing.T, fixture *usageBenchFixture, observer *usageBenchObserver, scenario *usageBenchCase) (*models.TokenUsageReport, []usageBenchStatement) {
	t.Helper()
	observer.start()
	report := fixture.readAndValidate(t, scenario)
	statements, err := observer.finish()
	require.NoError(t, err)
	require.NotEmpty(t, statements)
	return report, statements
}

func retainedReportVisits(statements []usageBenchStatement) int64 {
	var result int64
	for _, statement := range statements {
		for _, loop := range statement.SQLiteLoops {
			result += loop.Visits
		}
	}
	return result
}
