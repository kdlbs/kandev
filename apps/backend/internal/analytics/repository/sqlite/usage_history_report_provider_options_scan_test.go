//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/stretchr/testify/require"
)

func TestUsageHistoryProviderOptionsCoverReportSources(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "postgres" {
				if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
					t.Skip("actual PostgreSQL provider-option regression requires the private test DSN")
				}
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
			}

			fixture := newUsageBenchFixture(t, engine, usageBenchSize{Native: 10_000, External: 1_000})
			seedProviderOptionCoverage(t, fixture)
			fixture.seedRecent(t)
			fixture.validateStorage(t, false)
			if engine == "sqlite" {
				require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, fixture.observer))
			}

			start := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			base := models.SessionUsageFilter{
				WorkspaceID: usageBenchWorkspace, Timezone: "UTC", GroupBy: "model", Limit: 200,
			}
			bounded := base
			bounded.Start, bounded.End = &start, &end
			bounded.Model, bounded.Source = "external-model", "plugin:bench"
			boundedProvider := bounded
			boundedProvider.Provider = "external-provider"
			taskFiltered := base
			taskFiltered.Model, taskFiltered.Source = "external-model", "plugin:bench"
			taskFiltered.TaskIDs = []string{fixture.subjects[4].task}
			sessionFiltered := base
			sessionFiltered.Model, sessionFiltered.Source = "external-model", "plugin:bench"
			sessionFiltered.SessionIDs = []string{fixture.subjects[4].session}
			intervalFiltered := base
			intervalFiltered.Model, intervalFiltered.Source, intervalFiltered.Provider = "provider-options-model", "plugin:bench", "interval-only-provider"
			nativeFiltered := base
			nativeFiltered.Model, nativeFiltered.Source, nativeFiltered.Provider = "native-model", usageSourceNative, "native-provider"
			unknownControl := base

			cases := []struct {
				name          string
				filter        models.SessionUsageFilter
				wantProviders []string
				wantInterval  bool
				wantUnknown   bool
			}{
				{name: "cached_no_provider", filter: bounded, wantProviders: []string{"external-provider"}},
				{name: "bounded_provider_filter", filter: boundedProvider, wantProviders: []string{"external-provider"}},
				{name: "task_filtered_fallback", filter: taskFiltered, wantProviders: []string{"external-provider"}},
				{name: "session_filtered_fallback", filter: sessionFiltered, wantProviders: []string{"external-provider"}},
				{name: "all_time_native_summary", filter: nativeFiltered, wantProviders: []string{"native-provider"}},
				{name: "all_time_undated_interval_fallback", filter: intervalFiltered, wantProviders: []string{"interval-only-provider"}, wantInterval: true},
				{name: "unknown_provider_summary", filter: unknownControl, wantUnknown: true},
			}
			for _, testCase := range cases {
				t.Run(testCase.name, func(t *testing.T) {
					fixture.observer.start()
					report, err := fixture.repo.ListSessionUsage(t.Context(), testCase.filter)
					require.NoError(t, err)
					statements, err := fixture.observer.finish()
					require.NoError(t, err)
					providerQuery := usageProviderOptionsStatement(t, statements)
					if testCase.wantProviders != nil {
						require.Equal(t, testCase.wantProviders, report.Providers)
					}
					if testCase.wantUnknown {
						require.Contains(t, report.Providers, "__unknown_provider__")
					}
					if testCase.wantInterval {
						require.NotEmpty(t, report.Rows)
						require.Equal(t, "provider-options-model", report.Rows[0].Model)
						require.Equal(t, "interval-only-provider", report.Rows[0].Provider)
						require.NotNil(t, report.Totals.TotalTokens)
						require.EqualValues(t, 10, *report.Totals.TotalTokens)
					}
					require.Contains(t, providerQuery.SQL, "report_providers", "every report source must project provider options to one column")

					if engine == "postgres" {
						require.NoError(t, explainUsageBenchPostgres(t.Context(), fixture.database, statements))
						t.Logf("PostgreSQL %s provider-options plan: %s", testCase.name, string(providerQuery.PostgresPlan))
					} else {
						for _, loop := range providerQuery.SQLiteLoops {
							t.Logf("SQLite %s provider-options plan: %s; NLOOP=%d NVISIT=%d", testCase.name, loop.Plan, loop.Loops, loop.Visits)
						}
					}
				})
			}
		})
	}
}

func TestUsageHistoryAllTimeProviderOptionsIncludeUndatedIntervals(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "postgres" {
				if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
					t.Skip("actual PostgreSQL interval-provider regression requires the private test DSN")
				}
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
			}

			fixture := newUsageBenchFixture(t, engine, usageBenchSize{})
			seedProviderOptionCoverage(t, fixture)
			seedProviderOptionNoise(t, fixture)
			fixture.publish(t)
			fixture.validateStorage(t, false)
			if engine == "postgres" {
				_, err := fixture.database.Exec(`ANALYZE usage_projection_provider_options`)
				require.NoError(t, err)
			}
			if engine == "sqlite" {
				require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, fixture.observer))
			}

			filter := models.SessionUsageFilter{
				WorkspaceID: usageBenchWorkspace, Timezone: "UTC", GroupBy: "model",
				Model: "provider-options-model", Source: "plugin:bench", Provider: "interval-only-provider", Limit: 100,
			}
			fixture.observer.start()
			report, err := fixture.repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.NotEmpty(t, report.Rows)
			require.Equal(t, "provider-options-model", report.Rows[0].Model)
			require.Equal(t, "interval-only-provider", report.Rows[0].Provider)
			require.NotNil(t, report.Totals.TotalTokens)
			require.EqualValues(t, 10, *report.Totals.TotalTokens)
			require.Equal(t, []string{"interval-only-provider"}, report.Providers)
			statements, err := fixture.observer.finish()
			require.NoError(t, err)
			providerQuery := usageProviderOptionsStatement(t, statements)
			require.Contains(t, providerQuery.SQL, "report_providers")
			require.Contains(t, providerQuery.SQL, "usage_projection_provider_options", "undated intervals must remain available through the published provider dimension")
			require.NotContains(t, providerQuery.SQL, "usage_projection_contributions", "all-time provider options must not rebuild from point contributions")

			if engine == "postgres" {
				require.NoError(t, explainUsageBenchPostgres(t.Context(), fixture.database, statements))
				t.Logf("PostgreSQL interval-provider plan: %s", string(providerQuery.PostgresPlan))
				require.GreaterOrEqual(t, fixture.storage.ProviderOptionDimensions, int64(100))
				if !usageProviderPlanBoundedToDimension(t, providerQuery, fixture.storage.ProviderOptionDimensions) {
					t.Logf("PostgreSQL interval-provider plan: %s", string(providerQuery.PostgresPlan))
					t.FailNow()
				}
			} else {
				var visits int64
				var boundedPlan bool
				for _, loop := range providerQuery.SQLiteLoops {
					if strings.Contains(loop.Plan, "idx_usage_projection_provider_options_report") {
						visits += loop.Visits
						boundedPlan = strings.Contains(loop.Plan, "model=?") && strings.Contains(loop.Plan, "source=?")
						t.Logf("SQLite interval-provider option plan: %s; NLOOP=%d NVISIT=%d", loop.Plan, loop.Loops, loop.Visits)
					}
				}
				require.True(t, boundedPlan, "SQLite must use the scoped model/source provider dimension index")
				require.Positive(t, visits)
				require.LessOrEqual(t, visits, int64(4), "the index should visit only matching provider memberships in this three-observation fixture")
			}
		})
	}
}

func TestUsageHistoryAllTimeProviderOptionsExcludeSupersededDatedProvider(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "postgres" {
				if os.Getenv("KANDEV_TEST_POSTGRES_DSN") == "" {
					t.Skip("actual PostgreSQL alternative-provider regression requires the private test DSN")
				}
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
			}

			fixture := newUsageBenchFixture(t, engine, usageBenchSize{})
			seedProviderOptionAlternative(t, fixture)
			seedProviderOptionNoise(t, fixture)
			fixture.publish(t)
			fixture.validateStorage(t, false)
			_, err := fixture.database.Exec(fixture.database.Rebind(`DELETE FROM usage_projection_provider_options WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, fixture.subjects[6].generation, fixture.subjects[6].work)
			require.NoError(t, err)
			_, err = fixture.database.Exec(fixture.database.Rebind(`UPDATE usage_projection_history SET provider_options_ready=0,provider_options_cursor='',provider_options_phase='clear'
				WHERE scope_id=? AND generation=? AND work_key=?`), fixture.subjects[6].scope, fixture.subjects[6].generation, fixture.subjects[6].work)
			require.NoError(t, err)
			for range 100 {
				done, err := fixture.repo.RunUsageHistoryProjectionBatch(t.Context(), 200)
				require.NoError(t, err)
				if done {
					break
				}
			}
			if engine == "postgres" {
				_, err = fixture.database.Exec(`ANALYZE usage_projection_provider_options`)
				require.NoError(t, err)
			}
			if engine == "sqlite" {
				require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.database.DB, fixture.observer))
			}

			filter := models.SessionUsageFilter{
				WorkspaceID: usageBenchWorkspace, Timezone: "UTC", GroupBy: "model",
				Model: "provider-alternative-model", Source: "plugin:bench", Provider: "lifetime-only-provider", Limit: 100,
			}
			fixture.observer.start()
			report, err := fixture.repo.ListSessionUsage(t.Context(), filter)
			require.NoError(t, err)
			require.NotEmpty(t, report.Rows)
			require.Equal(t, "lifetime-only-provider", report.Rows[0].Provider)
			require.Equal(t, []string{"independent-dated-provider", "lifetime-only-provider"}, report.Providers)
			statements, err := fixture.observer.finish()
			require.NoError(t, err)
			providerQuery := usageProviderOptionsStatement(t, statements)
			require.Contains(t, providerQuery.SQL, "report_providers")
			if engine == "postgres" {
				require.NoError(t, explainUsageBenchPostgres(t.Context(), fixture.database, statements))
				t.Logf("PostgreSQL alternative-provider plan: %s", string(providerQuery.PostgresPlan))
				require.GreaterOrEqual(t, fixture.storage.ProviderOptionDimensions, int64(100))
				bounded := usageProviderPlanBoundedToDimension(t, providerQuery, fixture.storage.ProviderOptionDimensions)
				if !bounded {
					t.Logf("PostgreSQL alternative-provider plan: %s", string(providerQuery.PostgresPlan))
				}
				require.True(t, bounded, "PostgreSQL provider-options reads must scan only the compact provider-dimension table")
			} else {
				var indexed bool
				for _, loop := range providerQuery.SQLiteLoops {
					t.Logf("SQLite alternative-provider option plan: %s; NLOOP=%d NVISIT=%d", loop.Plan, loop.Loops, loop.Visits)
					indexed = indexed || strings.Contains(loop.Plan, "idx_usage_projection_provider_options_report")
				}
				require.True(t, indexed, "SQLite must use the published provider dimension index")
			}
		})
	}
}

func seedProviderOptionCoverage(t *testing.T, fixture *usageBenchFixture) {
	t.Helper()
	at := time.Date(2021, 6, 15, 12, 0, 0, 0, time.UTC)
	interval := usageBenchMeasurement(fixture.subjects[6], "provider-options-undated-interval", at, "plugin:bench")
	interval.Model, interval.Provider = "provider-options-model", "interval-only-provider"
	interval.CoverageEnd = timePointer(at.Add(time.Hour))
	interval.SourceDate, interval.SourceTimezone, interval.CoverageKey = "", "", ""
	fixture.reportSpecial(t, fixture.subjects[6], interval, nil)
	var accepted int
	require.NoError(t, fixture.database.Get(&accepted, fixture.database.Rebind(`SELECT COUNT(*) FROM usage_observation_items WHERE source=? AND item_id=?`), interval.Source, interval.SourceRecordID))
	require.Equal(t, 1, accepted, "the undated interval must enter through accepted observation admission")
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: interval})

	otherModel := usageBenchMeasurement(fixture.subjects[6], "provider-options-other-model", at.Add(2*time.Hour), "plugin:bench")
	otherModel.Model, otherModel.Provider = "different-provider-options-model", "other-model-provider"
	otherModel.CoverageEnd = timePointer(at.Add(3 * time.Hour))
	otherModel.SourceDate, otherModel.SourceTimezone, otherModel.CoverageKey = "", "", ""
	fixture.reportSpecial(t, fixture.subjects[6], otherModel, nil)
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: otherModel})

	otherSource := usageBenchMeasurement(fixture.subjects[0], "provider-options-other-source", at.Add(4*time.Hour), usageSourceNative)
	otherSource.Model, otherSource.Provider = "provider-options-model", "different-source-provider"
	fixture.reportSpecial(t, fixture.subjects[0], otherSource, nil)
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: otherSource, Native: true})
}

func seedProviderOptionAlternative(t *testing.T, fixture *usageBenchFixture) {
	t.Helper()
	at := time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)
	end := at.AddDate(0, 0, 1)
	dated := usageBenchMeasurement(fixture.subjects[6], "provider-options-dated-alternative", at, "plugin:bench")
	dated.Model, dated.Provider = "provider-alternative-model", "dated-only-provider"
	dated.CoverageEnd = timePointer(end)
	date := at.Format(time.DateOnly)
	dated.SourceDate, dated.SourceTimezone, dated.CoverageKey = date, "UTC", date
	fixture.reportSpecial(t, fixture.subjects[6], dated, nil)
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: dated, AllTimeExcluded: true})

	lifetime := dated
	lifetime.ID += "-lifetime"
	lifetime.SourceRecordID, lifetime.UsageIdentity = lifetime.ID, lifetime.ID
	lifetime.Provider = "lifetime-only-provider"
	lifetime.CoverageStart, lifetime.CoverageEnd = nil, nil
	lifetime.SourceDate, lifetime.SourceTimezone, lifetime.CoverageKey = "", "", ""
	fixture.reportSpecial(t, fixture.subjects[6], lifetime, nil)
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: lifetime})

	independent := usageBenchMeasurement(fixture.subjects[5], "provider-options-independent-dated", at, "plugin:bench")
	independent.Model, independent.Provider = "provider-alternative-model", "independent-dated-provider"
	independent.CoverageEnd = timePointer(end)
	independent.SourceDate, independent.SourceTimezone, independent.CoverageKey = date, "UTC", date
	fixture.reportSpecial(t, fixture.subjects[5], independent, nil)
	fixture.recordOracle(usageBenchExpectedRecord{Measurement: independent})
}

func seedProviderOptionNoise(t *testing.T, fixture *usageBenchFixture) {
	t.Helper()
	start := time.Date(2021, 6, 16, 0, 0, 0, 0, time.UTC)
	for i := range 128 {
		at := start.Add(time.Duration(i) * time.Minute)
		id := fmt.Sprintf("provider-options-noise-%04d", i)
		measurement := usageBenchMeasurement(fixture.subjects[4], id, at, "plugin:noise")
		measurement.Model = fmt.Sprintf("provider-options-noise-model-%04d", i)
		measurement.Provider = fmt.Sprintf("provider-options-noise-provider-%04d", i)
		measurement.CoverageEnd = timePointer(at.Add(time.Minute))
		measurement.SourceDate, measurement.SourceTimezone, measurement.CoverageKey = "", "", ""
		fixture.reportSpecial(t, fixture.subjects[4], measurement, nil)
		fixture.recordOracle(usageBenchExpectedRecord{Measurement: measurement})
	}
}

func usageProviderOptionsStatement(t *testing.T, statements []usageBenchStatement) *usageBenchStatement {
	t.Helper()
	for i := range statements {
		if strings.Contains(statements[i].SQL, "report_providers") {
			return &statements[i]
		}
	}
	t.Fatal("report did not execute a provider-options query")
	return nil
}

func usageProviderPlanUsesIndex(t *testing.T, statement *usageBenchStatement, index string) bool {
	t.Helper()
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(statement.PostgresPlan, &roots))
	var contains func(usageBenchPostgresNode) bool
	contains = func(node usageBenchPostgresNode) bool {
		if node.Index == index {
			return true
		}
		for _, child := range node.Plans {
			if contains(child) {
				return true
			}
		}
		return false
	}
	for _, root := range roots {
		if contains(root.Plan) {
			return true
		}
	}
	return false
}

func usageProviderPlanBoundedToDimension(t *testing.T, statement *usageBenchStatement, dimensions int64) bool {
	t.Helper()
	var roots []struct {
		Plan usageBenchPostgresNode `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(statement.PostgresPlan, &roots))
	var bounded func(usageBenchPostgresNode) bool
	bounded = func(node usageBenchPostgresNode) bool {
		if node.Relation == "usage_projection_provider_options" {
			loops := node.Loops
			if loops < 1 {
				loops = 1
			}
			visited := (node.Rows + node.Filtered + node.Rechecked) * loops
			return visited <= float64(dimensions)
		}
		for _, child := range node.Plans {
			if bounded(child) {
				return true
			}
		}
		return false
	}
	for _, root := range roots {
		if bounded(root.Plan) {
			return true
		}
	}
	return false
}

func timePointer(value time.Time) *time.Time { return &value }
