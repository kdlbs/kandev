//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const usageOverlapLookupHistory = 20000

type usageOverlapLookupPlan struct {
	Plan          string                   `json:"Node Type"`
	Index         string                   `json:"Index Name"`
	Rows          float64                  `json:"Actual Rows"`
	Loops         float64                  `json:"Actual Loops"`
	Removed       float64                  `json:"Rows Removed by Filter"`
	SharedHits    int64                    `json:"Shared Hit Blocks"`
	SharedReads   int64                    `json:"Shared Read Blocks"`
	IndexSearches float64                  `json:"Index Searches"`
	Relation      string                   `json:"Relation Name"`
	Filter        string                   `json:"Filter"`
	Plans         []usageOverlapLookupPlan `json:"Plans"`
}

type usageLookupStats struct {
	Visits  int64
	Buffers int64
	Plans   []string
}

type usageOverlapLookupCandidates struct {
	External indexedObservation
	Native   indexedObservation
	Coarse   indexedObservation
}

func TestUsageHistoryOverlapAndPendingLookupsBoundActualVisits(t *testing.T) {
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			if engine == "postgres" {
				require.NotEmpty(t, os.Getenv("KANDEV_TEST_POSTGRES_DSN"), "lookup plan proof requires the actual PostgreSQL test database")
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_POSTGRES", "1")
			} else {
				t.Setenv("KANDEV_USAGE_BENCH_TRACE_SQLITE", "1")
			}
			fixture := newUsageBenchFixture(t, engine, usageBenchSize{})
			candidates := seedUsageOverlapLookupHistory(t, fixture)
			for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
				_, err := fixture.database.ExecContext(t.Context(), `ANALYZE `+table)
				require.NoError(t, err)
			}
			if engine == "postgres" {
				var pendingIndexes []string
				err := fixture.database.SelectContext(t.Context(), &pendingIndexes, `SELECT indexdef FROM pg_indexes WHERE schemaname=current_schema() AND indexname LIKE '%native_accounting_pending_v1' ORDER BY indexname`)
				require.NoError(t, err)
				t.Logf("PostgreSQL pending-accounting index definitions: %s", strings.Join(pendingIndexes, " | "))
			}

			candidate := candidates.External
			baselineQuery, baselineArgs := usageOverlapLookupBaseline(candidate)
			baselineRefs, baseline := measureUsageOverlapReferenceQuery(t, fixture, baselineQuery, baselineArgs)
			actualRefs, actual := measureUsageOverlapReferences(t, fixture, candidate)
			require.ElementsMatch(t, baselineRefs, actualRefs, "the access-path split must preserve exact reference membership")
			require.Len(t, actualRefs, 3, "distinct native events at the same point and the covering coarse interval remain candidates")
			t.Logf("%s overlap lookup: baseline_rows=%d optimized_rows=%d baseline_buffers=%d optimized_buffers=%d; baseline_plan=%s; optimized_plan=%s", engine, baseline.Visits, actual.Visits, baseline.Buffers, actual.Buffers, strings.Join(baseline.Plans, " | "), strings.Join(actual.Plans, " | "))
			if engine == "sqlite" {
				require.Greater(t, baseline.Visits, int64(usageOverlapLookupHistory/2), "fixture must expose the broad historical scan")
				require.Less(t, actual.Visits, baseline.Visits/20, "point lookup must seek the bounded time range and coarse interval index")
			} else {
				require.LessOrEqual(t, actual.Visits, baseline.Visits, "PostgreSQL 18's existing skip-scan access path must remain bounded")
				require.Less(t, actual.Buffers, baseline.Buffers, "disjoint point/coarse branches must reduce PostgreSQL buffer work")
			}

			for _, candidate := range []struct {
				name string
				row  indexedObservation
				want int
			}{{"native_event", candidates.Native, 2}, {"coarse_interval", candidates.Coarse, 51}} {
				t.Run(candidate.name, func(t *testing.T) {
					if candidate.name == "native_event" {
						require.NotEmpty(t, candidate.row.EventID, "native overlap uses its nonblank event ID")
					}
					legacyQuery, legacyArgs := usageOverlapLookupBaseline(candidate.row)
					legacyRefs, legacyStats := measureUsageOverlapReferenceQuery(t, fixture, legacyQuery, legacyArgs)
					optimizedRefs, optimizedStats := measureUsageOverlapReferences(t, fixture, candidate.row)
					require.ElementsMatch(t, legacyRefs, optimizedRefs, "native event and coarse interval predicates must retain exact membership")
					require.Len(t, optimizedRefs, candidate.want)
					t.Logf("%s %s overlap: baseline_rows=%d optimized_rows=%d baseline_buffers=%d optimized_buffers=%d", engine, candidate.name, legacyStats.Visits, optimizedStats.Visits, legacyStats.Buffers, optimizedStats.Buffers)
				})
			}

			pendingSQL := usageNativeAccountingPendingBaseline()
			baselinePendingFound, baselinePendingStats := measureUsagePendingQuery(t, fixture, pendingSQL, fixture.subjects[6].scope)
			require.False(t, baselinePendingFound, "seeded history has no pending native accounting rows")
			actualPendingFound, actualPendingStats := measureUsagePendingGuard(t, fixture, fixture.subjects[6].scope)
			require.Equal(t, baselinePendingFound, actualPendingFound)
			t.Logf("%s empty pending guard: baseline_rows=%d optimized_rows=%d baseline_buffers=%d optimized_buffers=%d; baseline_plan=%s; optimized_plan=%s", engine, baselinePendingStats.Visits, actualPendingStats.Visits, baselinePendingStats.Buffers, actualPendingStats.Buffers, strings.Join(baselinePendingStats.Plans, " | "), strings.Join(actualPendingStats.Plans, " | "))
			if engine == "sqlite" {
				require.GreaterOrEqual(t, baselinePendingStats.Visits, int64(usageOverlapLookupHistory/2), "baseline guard must inspect existing native history")
				require.LessOrEqual(t, actualPendingStats.Visits, int64(2), "an empty partial-index lookup must not visit native history")
			} else {
				require.LessOrEqual(t, actualPendingStats.Visits, int64(2), "ordered scalar probes must use the exact PostgreSQL pending index")
			}

			pendingTx, err := fixture.database.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			result, err := pendingTx.ExecContext(t.Context(), fixture.database.Rebind(`UPDATE session_usage_buckets SET turns=7 WHERE source_record_id=? AND scope_id=?`), "native-lookup-0000", fixture.subjects[6].scope)
			require.NoError(t, err)
			changed, err := result.RowsAffected()
			require.NoError(t, err)
			require.EqualValues(t, 1, changed, "create one real legacy turns-pending native entry")
			require.NoError(t, pendingTx.Commit())
			for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
				_, err := fixture.database.ExecContext(t.Context(), `ANALYZE `+table)
				require.NoError(t, err)
			}
			baselinePendingFound, baselinePendingStats = measureUsagePendingQuery(t, fixture, pendingSQL, fixture.subjects[6].scope)
			require.True(t, baselinePendingFound)
			actualPendingFound, actualPendingStats = measureUsagePendingGuard(t, fixture, fixture.subjects[6].scope)
			require.True(t, actualPendingFound, "the indexed guard must still find an actual turns-pending row")
			t.Logf("%s pending-entry guard: baseline_rows=%d optimized_rows=%d baseline_buffers=%d optimized_buffers=%d; baseline_plan=%s; optimized_plan=%s", engine, baselinePendingStats.Visits, actualPendingStats.Visits, baselinePendingStats.Buffers, actualPendingStats.Buffers, strings.Join(baselinePendingStats.Plans, " | "), strings.Join(actualPendingStats.Plans, " | "))
			if engine == "sqlite" {
				require.LessOrEqual(t, actualPendingStats.Visits, int64(2), "the exact partial-index guard must stop after finding the real pending entry")
			} else {
				require.Positive(t, actualPendingStats.Visits, "PostgreSQL plan must report actual row work for the legacy pending path")
				require.LessOrEqual(t, actualPendingStats.Visits, int64(2), "ordered PostgreSQL probes must stop at the exact pending entry")
			}

			versionTx, err := fixture.database.BeginTxx(t.Context(), nil)
			require.NoError(t, err)
			_, err = versionTx.ExecContext(t.Context(), fixture.database.Rebind(`UPDATE session_usage_buckets SET turns=NULL,native_accounting_version=1 WHERE source_record_id=? AND scope_id=?`), "native-lookup-0000", fixture.subjects[6].scope)
			require.NoError(t, err)
			_, err = versionTx.ExecContext(t.Context(), fixture.database.Rebind(`UPDATE session_usage_buckets SET turns=NULL,native_accounting_version=0 WHERE source_record_id=? AND scope_id=?`), "native-lookup-0001", fixture.subjects[6].scope)
			require.NoError(t, err)
			require.NoError(t, versionTx.Commit())
			for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
				_, err := fixture.database.ExecContext(t.Context(), `ANALYZE `+table)
				require.NoError(t, err)
			}
			baselinePendingFound, baselinePendingStats = measureUsagePendingQuery(t, fixture, pendingSQL, fixture.subjects[6].scope)
			require.True(t, baselinePendingFound, "version zero alone remains pending after turns have been cleared")
			actualPendingFound, actualPendingStats = measureUsagePendingGuard(t, fixture, fixture.subjects[6].scope)
			require.True(t, actualPendingFound, "the indexed guard must retain version-only repair work")
			t.Logf("%s version-only pending guard: baseline_rows=%d optimized_rows=%d baseline_buffers=%d optimized_buffers=%d; optimized_plan=%s", engine, baselinePendingStats.Visits, actualPendingStats.Visits, baselinePendingStats.Buffers, actualPendingStats.Buffers, strings.Join(actualPendingStats.Plans, " | "))
			require.LessOrEqual(t, actualPendingStats.Visits, int64(2), "the version-only row must be found by the exact pending index")
		})
	}
}

func seedUsageOverlapLookupHistory(t *testing.T, fixture *usageBenchFixture) usageOverlapLookupCandidates {
	t.Helper()
	subject := fixture.subjects[6]
	start := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	const rowsPerSource = usageOverlapLookupHistory / 2
	records := make([]usageBenchSeedRecord, 0, usageOverlapLookupHistory+2)
	var candidates usageOverlapLookupCandidates
	last := start.Add(time.Duration(rowsPerSource-1) * time.Hour)
	for index := range rowsPerSource {
		at := start.Add(time.Duration(index) * time.Hour)
		for _, source := range []string{usageSourceNative, "plugin:bench"} {
			sourceAt := at
			id := fmt.Sprintf("native-lookup-%04d", index)
			if source != usageSourceNative {
				id = fmt.Sprintf("external-lookup-%04d", index)
			}
			if source == usageSourceNative && index == 0 {
				sourceAt = last
			}
			measurement := usageBenchMeasurement(subject, id, sourceAt, source)
			measurement.Model = fmt.Sprintf("model-%d", index%3)
			measurement.Provider = fmt.Sprintf("provider-%d", index%2)
			record := fixture.seedRecord(t, subject, measurement)
			if index == rowsPerSource-1 {
				if source == usageSourceNative {
					candidates.Native = record.observation
				} else {
					candidates.External = record.observation
				}
			}
			records = append(records, record)
		}
	}
	for _, interval := range []struct {
		id    string
		begin time.Time
		end   time.Time
	}{{"coarse-lookup-overlap", last.Add(-24 * time.Hour), last.Add(24 * time.Hour)}, {"coarse-lookup-before", last.Add(-48 * time.Hour), last.Add(-24 * time.Hour)}} {
		measurement := usageBenchMeasurement(subject, interval.id, interval.begin, "plugin:bench")
		measurement.CoverageStart, measurement.CoverageEnd = &interval.begin, &interval.end
		measurement.SourceDate, measurement.SourceTimezone = "", ""
		measurement.Model, measurement.Provider = "coarse-model", "coarse-provider"
		record := fixture.seedRecord(t, subject, measurement)
		if interval.id == "coarse-lookup-overlap" {
			candidates.Coarse = record.observation
		}
		records = append(records, record)
	}
	require.NotEmpty(t, candidates.External.Measurement.ID)
	require.NotEmpty(t, candidates.Native.Measurement.ID)
	fixture.insertBatch(t, records)
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var loadErr error
	candidates.External, loadErr = fixture.repo.loadIndexedObservationTx(t.Context(), tx, candidates.External.Table, candidates.External.Measurement.ID)
	require.NoError(t, loadErr)
	candidates.Native, loadErr = fixture.repo.loadIndexedObservationTx(t.Context(), tx, candidates.Native.Table, candidates.Native.Measurement.ID)
	require.NoError(t, loadErr)
	candidates.Coarse, loadErr = fixture.repo.loadIndexedObservationTx(t.Context(), tx, candidates.Coarse.Table, candidates.Coarse.Measurement.ID)
	require.NoError(t, loadErr)
	require.Equal(t, "", candidates.External.EventID, "external observations retain blank work_event_id semantics")
	require.NotEmpty(t, candidates.Native.EventID)
	return candidates
}

func usageOverlapLookupBaseline(candidate indexedObservation) (string, []any) {
	predicate, bounds := usageSelectionOverlap(candidate)
	var queries []string
	args := make([]any, 0, 16)
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		queries = append(queries, `SELECT id, '`+table+`' AS source_table, selection_coverage FROM `+table+` WHERE scope_id=? AND selection_partition=? AND retired=0`+predicate+` AND selection_coverage<>?`)
		args = append(args, candidate.Measurement.ScopeID, candidate.WorkKey)
		args = append(args, bounds...)
		args = append(args, candidate.CoverageKey)
		if candidate.EventID != "" {
			queries[len(queries)-1] += ` AND (work_event_id='' OR work_event_id=?)`
			args = append(args, candidate.EventID)
		}
	}
	return strings.Join(queries, ` UNION ALL `) + ` ORDER BY selection_coverage,id LIMIT 201`, args
}

func measureUsageOverlapReferences(t *testing.T, fixture *usageBenchFixture, candidate indexedObservation) ([]selectionReference, usageLookupStats) {
	t.Helper()
	fixture.observer.start()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	refs, err := fixture.repo.selectionReferencesTx(t.Context(), tx, candidate.Measurement.ScopeID, candidate.WorkKey, "", &candidate)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	statements, err := fixture.observer.finish()
	require.NoError(t, err)
	return refs, usageOverlapLookupVisits(t, fixture, statements)
}

func measureUsageOverlapReferenceQuery(t *testing.T, fixture *usageBenchFixture, inputQuery string, inputArgs []any) ([]selectionReference, usageLookupStats) {
	t.Helper()
	fixture.observer.start()
	var refs []selectionReference
	require.NoError(t, fixture.database.SelectContext(t.Context(), &refs, fixture.database.Rebind(inputQuery), inputArgs...))
	statements, err := fixture.observer.finish()
	require.NoError(t, err)
	return refs, usageOverlapLookupVisits(t, fixture, statements)
}

func usageOverlapLookupVisits(t *testing.T, fixture *usageBenchFixture, statements []usageBenchStatement) usageLookupStats {
	t.Helper()
	stats := usageLookupStats{}
	for _, statement := range statements {
		if !strings.Contains(statement.SQL, "selection_partition") || !strings.Contains(statement.SQL, "coverage_start") {
			continue
		}
		if fixture != nil && fixture.engine == "postgres" {
			visits, buffers, plan := usageOverlapPostgresPlanStats(t, fixture.database, statement)
			stats.Visits += visits
			stats.Buffers += buffers
			stats.Plans = append(stats.Plans, plan)
			continue
		}
		for _, loop := range statement.SQLiteLoops {
			stats.Visits += loop.Visits
			stats.Plans = append(stats.Plans, loop.Plan)
		}
	}
	require.NotEmpty(t, stats.Plans, "lookup proof must include the production SQL plan")
	return stats
}

func usageOverlapPostgresPlanStats(t *testing.T, database *sqlx.DB, statement usageBenchStatement) (int64, int64, string) {
	t.Helper()
	args := usageOverlapStatementArgs(statement.Args)
	var encoded []byte
	require.NoError(t, database.QueryRowxContext(t.Context(), `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) `+statement.SQL, args...).Scan(&encoded))
	var envelope []struct {
		Plan usageOverlapLookupPlan `json:"Plan"`
	}
	require.NoError(t, json.Unmarshal(encoded, &envelope))
	require.Len(t, envelope, 1)
	plan := envelope[0].Plan
	return usageOverlapPostgresRows(plan), plan.SharedHits + plan.SharedReads, usageOverlapPostgresSummary(plan)
}

func usageOverlapPostgresRows(plan usageOverlapLookupPlan) int64 {
	var total int64
	if plan.Plan == "Index Scan" || plan.Plan == "Index Only Scan" || plan.Plan == "Bitmap Heap Scan" || plan.Plan == "Seq Scan" {
		loops := plan.Loops
		if loops < 1 {
			loops = 1
		}
		total += int64((plan.Rows + plan.Removed) * loops)
	}
	for _, child := range plan.Plans {
		total += usageOverlapPostgresRows(child)
	}
	return total
}

func usageOverlapPostgresSummary(plan usageOverlapLookupPlan) string {
	var nodes []string
	if plan.Plan == "Index Scan" || plan.Plan == "Index Only Scan" || plan.Plan == "Bitmap Heap Scan" || plan.Plan == "Seq Scan" {
		nodes = append(nodes, fmt.Sprintf("%s[%s] relation=%s rows=%.0f removed=%.0f loops=%.0f buffers=%d searches=%.0f filter=%s", plan.Plan, plan.Index, plan.Relation, plan.Rows, plan.Removed, plan.Loops, plan.SharedHits+plan.SharedReads, plan.IndexSearches, plan.Filter))
	}
	for _, child := range plan.Plans {
		nodes = append(nodes, usageOverlapPostgresSummary(child))
	}
	return strings.Join(nodes, ",")
}

func usageOverlapStatementArgs(args []driver.NamedValue) []any {
	values := make([]any, len(args))
	for i := range args {
		values[i] = args[i].Value
	}
	return values
}

func usageNativeAccountingPendingBaseline() string {
	return `SELECT EXISTS(SELECT 1 FROM session_usage_measurements WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + `) OR EXISTS(SELECT 1 FROM session_usage_buckets WHERE scope_id=? AND ` + nativeUsageAccountingPendingSQL + `)`
}

func measureUsagePendingGuard(t *testing.T, fixture *usageBenchFixture, scope string) (bool, usageLookupStats) {
	t.Helper()
	fixture.observer.start()
	tx, err := fixture.database.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	pending, err := fixture.repo.nativeUsageAccountingRepairPendingTx(t.Context(), tx, scope)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	statements, err := fixture.observer.finish()
	require.NoError(t, err)
	return pending, usagePendingLookupVisits(t, fixture, statements)
}

func measureUsagePendingQuery(t *testing.T, fixture *usageBenchFixture, query, scope string) (bool, usageLookupStats) {
	t.Helper()
	fixture.observer.start()
	var pending bool
	require.NoError(t, fixture.database.GetContext(t.Context(), &pending, fixture.database.Rebind(query), scope, scope))
	statements, err := fixture.observer.finish()
	require.NoError(t, err)
	return pending, usagePendingLookupVisits(t, fixture, statements)
}

func usagePendingLookupVisits(t *testing.T, fixture *usageBenchFixture, statements []usageBenchStatement) usageLookupStats {
	t.Helper()
	stats := usageLookupStats{}
	for _, statement := range statements {
		if !strings.Contains(statement.SQL, "native_accounting_version") && !strings.Contains(statement.SQL, "native_accounting_source_version") {
			continue
		}
		if fixture != nil && fixture.engine == "postgres" {
			visits, buffers, plan := usageOverlapPostgresPlanStats(t, fixture.database, statement)
			stats.Visits += visits
			stats.Buffers += buffers
			stats.Plans = append(stats.Plans, plan)
			continue
		}
		for _, loop := range statement.SQLiteLoops {
			stats.Visits += loop.Visits
			stats.Plans = append(stats.Plans, loop.Plan)
		}
	}
	require.NotEmpty(t, stats.Plans, "pending proof must include the guard SQL plan")
	return stats
}
