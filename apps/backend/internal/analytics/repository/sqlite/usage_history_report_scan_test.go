//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"os"
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
