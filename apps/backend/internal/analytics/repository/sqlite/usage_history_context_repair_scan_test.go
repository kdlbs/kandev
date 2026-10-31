//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

const contextRepairScanPage = 7

type contextRepairScanFixture struct {
	repo                         *Repository
	scope, subject, work, cursor string
	version                      int64
	current                      pluginsdk.UsageCollectionTargetV1
}

type contextRepairScanSample struct {
	ids        []string
	statements []usageBenchStatement
	visits     int64
}

// The existing publication test counts rewrites. This test also bounds reads
// when current proven rows accumulate in the very same retained subject.
func TestUsageHistoryContextRepairBoundedSQLVisits(t *testing.T) {
	fixture := newContextRepairScanFixture(t)
	observer := &usageBenchObserver{}
	require.NoError(t, attachUsageBenchSQLite(t.Context(), fixture.repo.db.DB, observer))
	kinds := []string{usageMeasurementsTable, usageBucketsTable, "runtime_subject", "segment_cursor"}
	before := make(map[string]contextRepairScanSample, len(kinds))
	for _, kind := range kinds {
		before[kind] = measureContextRepairScan(t, fixture, observer, kind)
	}
	seedContextRepairScanRows(t, fixture.repo, fixture.current.SubjectBinding, "current", 401, false)
	var current int
	require.NoError(t, fixture.repo.db.Get(&current, `SELECT COUNT(*) FROM session_usage_measurements WHERE scope_id=? AND subject_id=? AND work_status='proven' AND selection_version=?`, fixture.scope, fixture.subject, fixture.version))
	require.GreaterOrEqual(t, current, 200, "growth must be current proven source rows, not fixture-only counters")
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			after := measureContextRepairScan(t, fixture, observer, kind)
			require.Equal(t, before[kind].ids, after.ids, "unrelated current work cannot change this repair page")
			t.Logf("%s: baseline NVISIT=%d; after 401 current proven rows in the same subject NVISIT=%d", kind, before[kind].visits, after.visits)
			for _, statement := range after.statements {
				for _, loop := range statement.SQLiteLoops {
					t.Logf("SQL=%s; plan=%s; NLOOP=%d NVISIT=%d", statement.SQL, loop.Plan, loop.Loops, loop.Visits)
				}
			}
			require.LessOrEqual(t, after.visits, before[kind].visits+4, "a fixed repair page must not scan unrelated current history; NVISIT already includes all NLOOPs")
		})
	}
}

func newContextRepairScanFixture(t *testing.T) contextRepairScanFixture {
	t.Helper()
	repo, _ := historyRetainedContractFixture(t)
	tasks, err := tasksqlite.NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(repo)
	at := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: "bounds-task", WorkspaceID: "ws-1", Title: "Bounds", CreatedAt: at, UpdatedAt: at}))
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{ID: "bounds-session", TaskID: "bounds-task", StartedAt: at}))
	target := contextRepairScanTarget(t, repo)
	seedContextRepairScanRows(t, repo, target.SubjectBinding, "original", 48, true)
	finishFixtureRetention(t, repo)
	completeFixtureProjection(t, repo)
	fixture := contextRepairScanFixture{repo: repo}
	require.NoError(t, repo.db.QueryRowx(`SELECT scope_id,subject_id,selection_partition FROM session_usage_measurements WHERE source_record_id='original-0000'`).Scan(&fixture.scope, &fixture.subject, &fixture.work))
	require.NoError(t, repo.db.Get(&fixture.version, `SELECT work_version FROM usage_scopes WHERE scope_id=?`, fixture.scope))
	require.Positive(t, fixture.version)
	_, err = repo.db.Exec(`UPDATE task_sessions SET metadata='{"acp":{"session_id":"bounds-proven-context"}}' WHERE id='bounds-session'`)
	require.NoError(t, err)
	fixture.current = contextRepairScanTarget(t, repo)
	var pending int
	require.NoError(t, repo.db.Get(&pending, `SELECT work_repair_pending FROM usage_subjects WHERE scope_id=? AND subject_id=?`, fixture.scope, fixture.subject))
	require.Equal(t, 1, pending, "real discovery must enqueue the subject's unproven context repair")
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		// Old-version dirtiness models a previous accepted mapping. Publication
		// state and source ownership remain those built by the real repository.
		result, err := repo.db.Exec(`UPDATE `+table+` SET selection_version=? WHERE scope_id=? AND subject_id=? AND work_status='unproven'`, fixture.version-1, fixture.scope, fixture.subject)
		require.NoError(t, err)
		count, err := result.RowsAffected()
		require.NoError(t, err)
		require.EqualValues(t, 24, count)
	}
	tx, err := repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	rows, err := repo.usageSegmentScanTx(t.Context(), tx, fixture.scope, fixture.work, "", contextRepairScanPage)
	require.NoError(t, err)
	require.Len(t, rows, contextRepairScanPage)
	fixture.cursor = rows[len(rows)-1].ScanKey
	require.NotEmpty(t, fixture.cursor)
	return fixture
}

func contextRepairScanTarget(t *testing.T, repo *Repository) pluginsdk.UsageCollectionTargetV1 {
	t.Helper()
	page, err := repo.ListUsageCollectionTargets(t.Context(), "bounds-collector", pluginsdk.UsageCollectionTargetQueryV1{
		WorkspaceID: "ws-1", CurrentTarget: &pluginsdk.UsageCollectionCurrentTargetQueryV1{TaskID: "bounds-task", SessionID: "bounds-session"},
	})
	require.NoError(t, err)
	require.True(t, page.CurrentTarget)
	require.Len(t, page.Targets, 1)
	require.Equal(t, "eligible", page.Targets[0].SourceScopeStatus)
	return page.Targets[0]
}

func seedContextRepairScanRows(t *testing.T, repo *Repository, binding, prefix string, count int, unproven bool) {
	t.Helper()
	for start := 0; start < count; start += maxSessionUsageBatch {
		batch := pluginsdk.UsageObservationBatchV1{SchemaVersion: pluginsdk.UsageObservationSchemaVersionV2}
		for i := start; i < min(start+maxSessionUsageBatch, count); i++ {
			item := contractObservation(t, binding, fmt.Sprintf("%s-%04d", prefix, i), 1, "bounds-model", 1)
			item.ModelProvider = stringPtr("bounds-provider")
			item.Segment = &pluginsdk.UsageSegmentIdentityV1{DecompositionID: "bounds-records", SegmentID: item.ItemID}
			if unproven {
				item.WorkIdentity = &pluginsdk.UsageWorkIdentityV1{Scheme: "unsupported-bounds-fixture", ID: "original-work"}
			}
			if i%2 != 0 {
				at := "2026-01-04T12:00:00Z"
				item.Coverage = &pluginsdk.UsageCoverageV1{OccurrenceAt: &at}
			}
			sealContractObservation(t, &item)
			batch.Observations = append(batch.Observations, item)
		}
		result, err := repo.ReportUsageObservations(t.Context(), "bounds-collector", batch)
		require.NoError(t, err)
		require.Equal(t, "accepted", result.Status)
		require.Len(t, result.Results, len(batch.Observations))
		for _, receipt := range result.Results {
			status := "proven"
			if unproven {
				status = "unproven"
			}
			require.Equal(t, status, receipt.WorkIdentityStatus)
		}
	}
}

func measureContextRepairScan(t *testing.T, fixture contextRepairScanFixture, observer *usageBenchObserver, kind string) contextRepairScanSample {
	t.Helper()
	tx, err := fixture.repo.db.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, _, err = fixture.repo.lockUsageProjectionScopeTx(t.Context(), tx, fixture.scope)
	require.NoError(t, err)
	observer.start()
	var ids []string
	switch kind {
	case usageMeasurementsTable, usageBucketsTable:
		query, args := usageInitialRepairQuery(kind, fixture.scope, `selection_version<?`, fixture.version, contextRepairScanPage)
		var refs []selectionReference
		require.NoError(t, tx.SelectContext(t.Context(), &refs, query, args...))
		for _, ref := range refs {
			ids = append(ids, ref.ID)
		}
	case "runtime_subject":
		worked, err := fixture.repo.repairUsageContextPageTx(t.Context(), tx, fixture.scope, contextRepairScanPage)
		require.NoError(t, err)
		require.True(t, worked)
	case "segment_cursor":
		rows, err := fixture.repo.usageSegmentScanTx(t.Context(), tx, fixture.scope, fixture.work, fixture.cursor, contextRepairScanPage)
		require.NoError(t, err)
		for _, row := range rows {
			require.Greater(t, row.ScanKey, fixture.cursor)
			ids = append(ids, row.Measurement.ID)
		}
	default:
		t.Fatalf("unknown repair scan %q", kind)
	}
	statements, err := observer.finish()
	require.NoError(t, err)
	if kind == "runtime_subject" {
		require.NoError(t, tx.SelectContext(t.Context(), &ids, `SELECT id FROM session_usage_buckets WHERE scope_id=? AND subject_id=? AND source_record_id LIKE 'original-%' AND selection_version=? ORDER BY id`, fixture.scope, fixture.subject, fixture.version))
	}
	require.Len(t, ids, contextRepairScanPage, "the measured production operation must process a full nonempty page")
	assertContextRepairScanPlan(t, tx, fixture, kind, statements)
	visits := retainedReportVisits(statements)
	require.Positive(t, visits, "the proof requires actual SQLite visits")
	return contextRepairScanSample{ids: ids, statements: statements, visits: visits}
}

func assertContextRepairScanPlan(t *testing.T, tx *sqlx.Tx, fixture contextRepairScanFixture, kind string, statements []usageBenchStatement) {
	t.Helper()
	fragment := "selection_version<?"
	args := []any{fixture.scope, fixture.version, contextRepairScanPage}
	constraints := []string{"scope_id=?", "selection_version<?"}
	if kind == "runtime_subject" {
		fragment = "work_status='unproven' AND source<>'native' AND id>?"
		args = []any{fixture.scope, fixture.subject, "", contextRepairScanPage}
		constraints = []string{"scope_id=?", "subject_id=?", "id>?"}
	}
	if kind == "segment_cursor" {
		fragment = "UNION ALL"
		args = []any{fixture.scope, fixture.work, fixture.cursor, fixture.scope, fixture.work, fixture.cursor, contextRepairScanPage}
		constraints = []string{"scope_id=?", "selection_partition=?", "retired=?", "selection_scan_key>?"}
	}
	matched := false
	for _, statement := range statements {
		if !strings.Contains(statement.SQL, fragment) {
			continue
		}
		matched = true
		rows, err := tx.QueryxContext(t.Context(), "EXPLAIN QUERY PLAN "+statement.SQL, args...)
		require.NoError(t, err)
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
			plan = append(plan, detail)
		}
		require.NoError(t, rows.Err())
		require.NoError(t, rows.Close())
		t.Logf("%s actual EXPLAIN QUERY PLAN: %s", kind, strings.Join(plan, " | "))
		require.NotContains(t, strings.Join(plan, " "), "USE TEMP B-TREE", "the bounded cursor order must come from the index")
		require.NotEmpty(t, statement.SQLiteLoops)
		for _, loop := range statement.SQLiteLoops {
			require.Contains(t, loop.Plan, "SEARCH ", "the production page must seek its bounded index range")
			for _, constraint := range constraints {
				require.Contains(t, loop.Plan, constraint)
			}
		}
	}
	require.True(t, matched, "instrumentation must observe the intended production query")
}
