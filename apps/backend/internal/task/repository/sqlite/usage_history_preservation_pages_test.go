package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

const (
	preservationPageRecords    = 451
	preservationPageSize       = 200
	preservationPageTask       = "preservation-page-task"
	preservationPageSession    = "preservation-page-session"
	preservationOtherTask      = "preservation-other-task"
	preservationOtherSession   = "preservation-other-session"
	preservationSiblingSession = "preservation-sibling-session"
	preservationLegacySource   = "plugin:legacy-pages"
)

type preservationPageAmount struct {
	Input int64
	Total int64
	Cost  int64
}

type preservationPageFixture struct {
	db           *sqlx.DB
	tasks        *Repository
	analytics    *analyticssqlite.Repository
	session      *models.TaskSession
	prune        bool
	nativeKeys   []int64
	externalKeys map[string][]string
	expected     map[string]map[string]preservationPageAmount
}

func TestUsageHistoryTaskDeletePreservationPages(t *testing.T) {
	runPreservationPageCases(t, false, false, []string{"", "native"})
}

func TestUsageHistorySessionPrunePreservationPages(t *testing.T) {
	runPreservationPageCases(t, false, true, []string{"", "measurements", "buckets"})
}

func TestUsageHistoryPostgresTaskDeletePreservationPages(t *testing.T) {
	runPreservationPageCases(t, true, false, []string{"", "native"})
}

func TestUsageHistoryPostgresSessionPrunePreservationPages(t *testing.T) {
	runPreservationPageCases(t, true, true, []string{"", "measurements", "buckets"})
}

func runPreservationPageCases(t *testing.T, postgres, prune bool, failures []string) {
	t.Helper()
	for _, failure := range failures {
		name := "all_pages"
		if failure != "" {
			name = failure + "_rollback_resume"
		}
		t.Run(name, func(t *testing.T) {
			fixture := newPreservationPageFixture(t, postgres, prune)
			if failure != "" {
				fixture.failAndResume(t, failure)
			}
			require.NoError(t, fixture.removeLiveTarget())
			fixture.assertAllRetained(t)
			fixture.assertLiveCascade(t)
			fixture.assertCheckpoints(t, "", true)
		})
	}
}

func newPreservationPageFixture(t *testing.T, postgres, prune bool) *preservationPageFixture {
	t.Helper()
	var database *sqlx.DB
	if postgres {
		dsn := testutil.PostgresDSNFromEnv(t)
		database = testutil.OpenIsolatedPostgres(t, dsn)
		var version string
		require.NoError(t, database.Get(&version, `SHOW server_version`))
		t.Logf("PostgreSQL %s, session timezone Asia/Kathmandu", version)
		_, err := database.Exec(`SET TIME ZONE 'Asia/Kathmandu'`)
		require.NoError(t, err)
	} else {
		raw, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "preservation-pages.db"))
		require.NoError(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = database.Close() })
	}
	fixture := &preservationPageFixture{
		db: database, prune: prune,
		externalKeys: make(map[string][]string),
		expected:     make(map[string]map[string]preservationPageAmount),
	}
	fixture.reloadRepositories(t)
	ctx := context.Background()
	require.NoError(t, fixture.tasks.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Preservation pages"}))
	createUsageEventsTestTask(t, fixture.tasks, preservationPageTask)
	createUsageEventsTestTask(t, fixture.tasks, preservationOtherTask)
	createUsageEventsTestSession(t, fixture.tasks, preservationPageSession, preservationPageTask)
	createUsageEventsTestSession(t, fixture.tasks, preservationOtherSession, preservationOtherTask)
	if prune {
		createUsageEventsTestSession(t, fixture.tasks, preservationSiblingSession, preservationPageTask)
	}
	var err error
	fixture.session, err = fixture.tasks.GetTaskSession(ctx, preservationPageSession)
	require.NoError(t, err)
	fixture.seedLegacyRows(t)
	return fixture
}

func (f *preservationPageFixture) reloadRepositories(t *testing.T) {
	t.Helper()
	var err error
	f.tasks, err = NewWithDB(f.db, f.db, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(f.db, f.db)
	require.NoError(t, err)
	f.analytics, err = analyticssqlite.NewWithDB(f.db, f.db)
	require.NoError(t, err)
	f.tasks.SetUsageHistoryRetainer(f.analytics)
}

func (f *preservationPageFixture) removeLiveTarget() error {
	if f.prune {
		return f.tasks.DeleteTaskSession(context.Background(), f.session)
	}
	return f.tasks.DeleteTask(context.Background(), preservationPageTask)
}

func (f *preservationPageFixture) seedLegacyRows(t *testing.T) {
	t.Helper()
	tx, err := f.db.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	native, err := tx.Preparex(f.db.Rebind(`INSERT INTO task_usage_events
		(usage_event_id, task_id, session_id, model, provider, tokens_in, tokens_total,
		cost_subcents, cost_source, usage_completeness, contract_version, occurred_at, created_at)
		VALUES (?, ?, ?, 'exact-model', 'provider', ?, ?, ?, 'provider_reported', 'complete', 1, ?, ?)`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = native.Close() })
	f.expected["native"] = make(map[string]preservationPageAmount)
	point := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
	for i := 0; i < preservationPageRecords; i++ {
		id := fmt.Sprintf("legacy-native-%03d", i)
		amount := preservationPageAmount{Input: int64(i+1) * 17, Total: int64(i+1) * 17, Cost: int64(i+1) * 2}
		_, err := native.Exec(id, preservationPageTask, preservationPageSession, amount.Input, amount.Total, amount.Cost, point, point)
		require.NoError(t, err)
		f.expected["native"][id] = amount
		_, err = native.Exec("other-"+id, preservationOtherTask, preservationOtherSession, 777, 777, 333, point, point)
		require.NoError(t, err)
	}
	if f.prune {
		_, err := native.Exec("sibling-native", preservationPageTask, preservationSiblingSession, 991, 991, 113, point, point)
		require.NoError(t, err)
	}
	for _, table := range []string{"measurements", "buckets"} {
		f.seedExternalRows(t, tx, table, point)
	}
	require.NoError(t, tx.Commit())
	require.NoError(t, f.db.Select(&f.nativeKeys, f.db.Rebind(`SELECT id FROM task_usage_events
		WHERE task_id = ? AND session_id = ? ORDER BY id`), preservationPageTask, preservationPageSession))
	require.Len(t, f.nativeKeys, preservationPageRecords)
	require.Greater(t, f.nativeKeys[1]-f.nativeKeys[0], int64(1), "native fixture must contain ID gaps")
}

func (f *preservationPageFixture) seedExternalRows(t *testing.T, tx *sqlx.Tx, source string, point time.Time) {
	t.Helper()
	statement, err := tx.Preparex(f.db.Rebind(`INSERT INTO session_usage_` + source + `
		(id, workspace_id, task_id, session_id, source, source_record_id, usage_identity, model, provider,
		revision, payload_digest, observed_at, collected_at, input_tokens, total_tokens, cost_subcents,
		cost_basis, cost_coverage, currency, coverage_key, source_date, source_timezone)
		VALUES (?, 'ws-1', ?, ?, 'plugin:legacy-pages', ?, ?, 'exact-model', 'provider',
		1, 'legacy-digest', ?, ?, ?, ?, ?, 'reported', 'complete', 'USD', ?, ?, 'UTC')`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = statement.Close() })
	f.expected[source] = make(map[string]preservationPageAmount)
	coverage := ""
	if source == "buckets" {
		coverage = "2030-01-02"
	}
	for i := 0; i < preservationPageRecords; i++ {
		key := preservationExternalKey(source, i)
		id := fmt.Sprintf("legacy-%s-%03d", source, i)
		amount := preservationPageAmount{Input: int64(i+1) * 19, Total: int64(i+1) * 19, Cost: int64(i+1) * 3}
		_, err := statement.Exec(key, preservationPageTask, preservationPageSession, id, id, point, point,
			amount.Input, amount.Total, amount.Cost, coverage, coverage)
		require.NoError(t, err)
		f.externalKeys[source] = append(f.externalKeys[source], key)
		f.expected[source][id] = amount
		_, err = statement.Exec(key+"-other", preservationOtherTask, preservationOtherSession, "other-"+id, "other-"+id,
			point, point, 777, 777, 333, coverage, coverage)
		require.NoError(t, err)
	}
	if f.prune {
		_, err := statement.Exec("!sibling-"+source, preservationPageTask, preservationSiblingSession,
			"sibling-"+source, "sibling-"+source, point, point, 991, 991, 113, coverage, coverage)
		require.NoError(t, err)
	}
	sort.Strings(f.externalKeys[source])
}

func preservationExternalKey(source string, index int) string {
	// Keep the legacy range below newly retained native UUIDs while exercising
	// punctuation, case, and non-ASCII byte ordering within that range.
	classes := []string{"é", "a", "Z", "_"}
	return fmt.Sprintf("!%s-%s-%03d", classes[index%len(classes)], source, index)
}

type preservationRetainedRow struct {
	RecordID    string `db:"source_record_id"`
	ScopeID     string `db:"scope_id"`
	SubjectID   string `db:"subject_id"`
	SessionID   string `db:"session_id"`
	ActorID     string `db:"actor_id"`
	ActorStatus string `db:"actor_status"`
	Input       int64  `db:"input_tokens"`
	Total       int64  `db:"total_tokens"`
	Cost        int64  `db:"cost_subcents"`
}

func (f *preservationPageFixture) assertAllRetained(t *testing.T) {
	t.Helper()
	for _, source := range []string{"native", "measurements", "buckets"} {
		table, provenance := "session_usage_"+source, preservationLegacySource
		if source == "native" {
			table, provenance = "session_usage_buckets", "native"
		}
		var rows []preservationRetainedRow
		err := f.db.Select(&rows, f.db.Rebind(`SELECT source_record_id, scope_id, subject_id, session_id,
			actor_id, actor_status, input_tokens, total_tokens, cost_subcents FROM `+table+`
			WHERE task_id = ? AND session_id = ? AND source = ?`), preservationPageTask, preservationPageSession, provenance)
		require.NoError(t, err)
		if len(rows) != preservationPageRecords {
			t.Errorf("%s retained rows = %d, want every one of %d legacy records", source, len(rows), preservationPageRecords)
			continue
		}
		seen := make(map[string]bool, len(rows))
		for _, row := range rows {
			amount, exists := f.expected[source][row.RecordID]
			require.True(t, exists, "unexpected %s identity %q", source, row.RecordID)
			require.False(t, seen[row.RecordID], "duplicate %s identity %q", source, row.RecordID)
			seen[row.RecordID] = true
			require.Equal(t, amount, preservationPageAmount{row.Input, row.Total, row.Cost}, "%s/%s amounts", source, row.RecordID)
			require.NotEmpty(t, row.ScopeID, "%s/%s scope", source, row.RecordID)
			require.NotEmpty(t, row.SubjectID, "%s/%s subject", source, row.RecordID)
			require.Empty(t, row.ActorID, "%s/%s must keep unknown legacy actor", source, row.RecordID)
			require.Equal(t, "unknown", row.ActorStatus, "%s/%s actor status", source, row.RecordID)
		}
	}
}

func (f *preservationPageFixture) assertLiveCascade(t *testing.T) {
	t.Helper()
	var taskRows, sessionRows, nativeRows, detachedRows int
	require.NoError(t, f.db.Get(&taskRows, f.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE id = ?`), preservationPageTask))
	require.NoError(t, f.db.Get(&sessionRows, f.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE id = ?`), preservationPageSession))
	require.NoError(t, f.db.Get(&nativeRows, f.db.Rebind(`SELECT COUNT(*) FROM task_usage_events WHERE task_id = ?
		AND usage_event_id <> 'sibling-native'`), preservationPageTask))
	require.Zero(t, sessionRows)
	if f.prune {
		require.Equal(t, 1, taskRows)
		require.Equal(t, preservationPageRecords, nativeRows)
		require.NoError(t, f.db.Get(&detachedRows, f.db.Rebind(`SELECT COUNT(*) FROM task_usage_events
			WHERE task_id = ? AND session_id IS NULL`), preservationPageTask))
		require.Equal(t, preservationPageRecords, detachedRows)
		f.assertSiblingUntouched(t)
	} else {
		require.Zero(t, taskRows)
		require.Zero(t, nativeRows)
	}
	var liveLinks, otherRows int
	require.NoError(t, f.db.Get(&liveLinks, f.db.Rebind(`SELECT COUNT(*) FROM usage_live_links
		WHERE live_session_id = ?`), preservationPageSession))
	require.Zero(t, liveLinks)
	require.NoError(t, f.db.Get(&otherRows, f.db.Rebind(`SELECT COUNT(*) FROM task_usage_events
		WHERE task_id = ? AND tokens_in = 777 AND cost_subcents = 333`), preservationOtherTask))
	require.Equal(t, preservationPageRecords, otherRows)
	for _, source := range []string{"measurements", "buckets"} {
		require.NoError(t, f.db.Get(&otherRows, f.db.Rebind(`SELECT COUNT(*) FROM session_usage_`+source+`
			WHERE task_id = ? AND scope_id = '' AND subject_id = '' AND input_tokens = 777 AND cost_subcents = 333`), preservationOtherTask))
		require.Equal(t, preservationPageRecords, otherRows)
	}
}

func (f *preservationPageFixture) assertSiblingUntouched(t *testing.T) {
	t.Helper()
	var count int
	require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE id = ?`), preservationSiblingSession))
	require.Equal(t, 1, count)
	require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM task_usage_events
		WHERE session_id = ? AND tokens_in = 991 AND cost_subcents = 113`), preservationSiblingSession))
	require.Equal(t, 1, count)
	for _, source := range []string{"measurements", "buckets"} {
		require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM session_usage_`+source+`
			WHERE session_id = ? AND scope_id = '' AND subject_id = '' AND input_tokens = 991 AND cost_subcents = 113`), preservationSiblingSession))
		require.Equal(t, 1, count)
	}
}

func (f *preservationPageFixture) checkpointKey(source string, index int) string {
	if source == "native" {
		return strconv.FormatInt(f.nativeKeys[index], 10)
	}
	return f.externalKeys[source][index]
}
