package sqlite

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	analyticssqlite "github.com/kandev/kandev/internal/analytics/repository/sqlite"
	"github.com/kandev/kandev/internal/common/usagehistory"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func TestUsageHistoryUncopiedNativeCleanup(t *testing.T) {
	runUncopiedNativeCleanup(t, false)
}

func TestUsageHistoryPostgresUncopiedNativeCleanup(t *testing.T) {
	runUncopiedNativeCleanup(t, true)
}

func TestUsageHistoryDurableExternalGapAllowsCleanup(t *testing.T) {
	runDurableExternalGapCleanup(t, false)
}

func TestUsageHistoryPostgresDurableExternalGapAllowsCleanup(t *testing.T) {
	runDurableExternalGapCleanup(t, true)
}

func runDurableExternalGapCleanup(t *testing.T, postgres bool) {
	t.Helper()
	f := newUncopiedNativeFixture(t, postgres, 2)
	for _, event := range f.readLedger(t) {
		f.retainOriginalEvent(t, event)
	}
	seedLegacyExternalUsage(t, f.tasks, f.proof.TaskID, f.session.ID, "durable-external-gap")
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		_, err := f.db.Exec(f.db.Rebind(`UPDATE `+table+` SET scope_id = ?, subject_id = ? WHERE source_record_id = ?`),
			"unavailable-original-scope", "unavailable-original-subject", "durable-external-gap")
		require.NoError(t, err)
	}
	require.NoError(t, f.cleanup(false), "external rows already occupy the retained tables, without live cascade foreign keys")
	for _, table := range []string{"session_usage_measurements", "session_usage_buckets"} {
		var count int
		require.NoError(t, f.db.Get(&count, f.db.Rebind(`SELECT COUNT(*) FROM `+table+`
			WHERE source_record_id = ? AND task_id = ? AND session_id = ? AND input_tokens = 23 AND total_tokens = 23
			AND scope_id = ? AND subject_id = ? AND payload_digest = 'legacy-digest'`),
			"durable-external-gap", f.proof.TaskID, f.session.ID, "unavailable-original-scope", "unavailable-original-subject"))
		require.Equal(t, 1, count, "unresolved external source values and original identity must survive unchanged")
	}
}

// @covers AC-COSTS-USAGE-HISTORY-007.2
func runUncopiedNativeCleanup(t *testing.T, postgres bool) {
	t.Helper()
	for _, prune := range []bool{false, true} {
		name := "delete"
		if prune {
			name = "prune"
		}
		for _, count := range []int{2, 451} {
			t.Run(name+"-"+strconv.Itoa(count), func(t *testing.T) {
				f := newUncopiedNativeFixture(t, postgres, count)
				before := f.readLedger(t)
				// Earlier events can be copied, but the last has no discoverable
				// ownership proof. Each page's copies and cursor share one Tx.
				require.ErrorContains(t, f.cleanup(prune), "no accepted retained copy")
				f.assertBlocked(t, before)
				f.restart(t)
				require.ErrorContains(t, f.cleanup(prune), "no accepted retained copy")
				f.assertBlocked(t, before)

				// Restore the original execution's authoritative evidence, captured
				// before either event. Current workspace ownership is not that proof.
				f.retainOriginalEvent(t, before[len(before)-1])
				_, err := f.db.Exec(f.db.Rebind(`UPDATE workspaces SET org_id = ? WHERE id = ?`), "org-b", f.proof.WorkspaceID)
				require.NoError(t, err)
				require.NoError(t, f.cleanup(prune))
				f.assertRetained(t, before)
				var liveSessions, liveTasks int
				require.NoError(t, f.db.Get(&liveSessions, f.db.Rebind(`SELECT COUNT(*) FROM task_sessions WHERE id = ?`), f.session.ID))
				require.Zero(t, liveSessions)
				require.NoError(t, f.db.Get(&liveTasks, f.db.Rebind(`SELECT COUNT(*) FROM tasks WHERE id = ?`), f.proof.TaskID))
				if prune {
					require.Equal(t, 1, liveTasks)
					after := f.readLedger(t)
					require.Len(t, after, len(before))
					for _, event := range after {
						require.Empty(t, event.SessionID)
					}
				} else {
					require.Zero(t, liveTasks)
					require.Empty(t, f.readLedger(t))
				}
			})
		}
	}
}

type uncopiedNativeFixture struct {
	db      *sqlx.DB
	tasks   *Repository
	history *analyticssqlite.Repository
	session *models.TaskSession
	proof   usagehistory.Binding
}

func newUncopiedNativeFixture(t *testing.T, postgres bool, count int) *uncopiedNativeFixture {
	t.Helper()
	f := &uncopiedNativeFixture{}
	if postgres {
		f.db = testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
		var version string
		require.NoError(t, f.db.Get(&version, `SHOW server_version`))
		t.Logf("PostgreSQL %s", version)
	} else {
		raw, err := internaldb.OpenSQLite(filepath.Join(t.TempDir(), "uncopied-native.db"))
		require.NoError(t, err)
		f.db = sqlx.NewDb(raw, "sqlite3")
		t.Cleanup(func() { _ = f.db.Close() })
	}
	var err error
	f.tasks, err = NewWithDB(f.db, f.db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Legacy", OrgID: "org-a"}))
	task := createUsageEventsTestTask(t, f.tasks, "uncopied-task")
	task, err = f.tasks.GetTask(ctx, task.ID)
	require.NoError(t, err)
	createUsageEventsTestSession(t, f.tasks, "uncopied-session", task.ID)
	f.session, err = f.tasks.GetTaskSession(ctx, "uncopied-session")
	require.NoError(t, err)
	f.proof = usagehistory.Binding{
		ID: "original-execution-proof", Kind: usagehistory.BindingKindExecution,
		WorkspaceID: task.WorkspaceID, OrganizationID: "org-a", TaskID: task.ID,
		TaskIncarnationID: task.CreatedAt.UTC().Format(time.RFC3339Nano),
		SessionID:         f.session.ID, SessionIncarnationID: f.session.QueueIncarnationID,
		ExecutionID: "original-execution", ActorStatus: usagehistory.ActorStatusUnknown,
		ActorKind: usagehistory.ActorKindUnknown, ActorSource: "legacy", CapturedAt: time.Now().UTC(),
	}
	for i := range count {
		event := newTestUsageEvent("uncopied-event-"+strconv.Itoa(i), task.ID, f.session.ID)
		event.CostSource = usageCostSourceProviderReported
		event.TokensIn += int64(i)
		event.TokensTotal += int64(i)
		event.CostSubcents += int64(i)
		if i < count-1 {
			event.UsageBindingID = f.proof.ID
		}
		require.NoError(t, f.tasks.CreateTaskUsageEvent(ctx, event))
	}
	f.restart(t)
	tx, err := f.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	require.NoError(t, f.history.AdmitBindingTx(ctx, tx, f.proof))
	require.NoError(t, tx.Commit())
	return f
}

func (f *uncopiedNativeFixture) restart(t *testing.T) {
	t.Helper()
	var err error
	f.tasks, err = NewWithDB(f.db, f.db, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(f.db, f.db)
	require.NoError(t, err)
	f.history, err = analyticssqlite.NewWithDB(f.db, f.db)
	require.NoError(t, err)
	f.tasks.SetUsageHistoryRetainer(f.history)
}

func (f *uncopiedNativeFixture) cleanup(prune bool) error {
	if prune {
		return f.tasks.DeleteTaskSession(context.Background(), f.session)
	}
	return f.tasks.DeleteTask(context.Background(), f.proof.TaskID)
}

func (f *uncopiedNativeFixture) readLedger(t *testing.T) []*models.TaskUsageEvent {
	t.Helper()
	events, err := f.tasks.ListTaskUsageEvents(context.Background(), f.proof.TaskID, 0)
	require.NoError(t, err)
	return events
}

func (f *uncopiedNativeFixture) assertBlocked(t *testing.T, before []*models.TaskUsageEvent) {
	t.Helper()
	require.Equal(t, before, f.readLedger(t), "failed cleanup must preserve every raw value and source identity")
	_, err := f.tasks.GetTask(context.Background(), f.proof.TaskID)
	require.NoError(t, err)
	_, err = f.tasks.GetTaskSession(context.Background(), f.session.ID)
	require.NoError(t, err)
	var copied int
	require.NoError(t, f.db.Get(&copied, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native'`))
	committed := (len(before) - 1) / preservationPageSize * preservationPageSize
	require.Equal(t, committed, copied, "an unproven later event must roll back every copy in its page")
	var cursor struct {
		HighWater string `db:"high_water"`
		LastKey   string `db:"last_key"`
		Complete  int    `db:"complete"`
		State     string `db:"state"`
	}
	require.NoError(t, f.db.Get(&cursor, `SELECT c.high_water, c.last_key, c.complete, f.state
		FROM usage_preservation_cursors c JOIN usage_preservation_fences f ON f.fence_id = c.fence_id
		WHERE c.source_key = 'native'`))
	require.Equal(t, strconv.FormatInt(before[len(before)-1].ID, 10), cursor.HighWater)
	lastKey := ""
	if committed != 0 {
		lastKey = strconv.FormatInt(before[committed-1].ID, 10)
	}
	require.Equal(t, lastKey, cursor.LastKey)
	require.Zero(t, cursor.Complete)
	require.Equal(t, "preserving", cursor.State)
}

func (f *uncopiedNativeFixture) retainOriginalEvent(t *testing.T, event *models.TaskUsageEvent) {
	t.Helper()
	ctx := context.Background()
	tx, err := f.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	native := nativeUsageFromTaskEvent(event)
	native.BindingID = f.proof.ID
	require.NoError(t, f.history.RetainNativeUsageTx(ctx, tx, native))
	require.NoError(t, tx.Commit())
}

func (f *uncopiedNativeFixture) assertRetained(t *testing.T, before []*models.TaskUsageEvent) {
	t.Helper()
	var count int
	require.NoError(t, f.db.Get(&count, `SELECT COUNT(*) FROM session_usage_buckets WHERE source = 'native'`))
	require.Equal(t, len(before), count)
	for _, event := range before {
		var retained struct {
			OrganizationID string `db:"organization_id"`
			TaskID         string `db:"task_id"`
			SessionID      string `db:"session_id"`
			IncarnationID  string `db:"task_incarnation_id"`
			SessionBirth   string `db:"session_incarnation_id"`
			BindingID      string `db:"usage_binding_id"`
			ActorID        string `db:"actor_id"`
			ActorStatus    string `db:"actor_status"`
			Input          int64  `db:"input_tokens"`
			Output         int64  `db:"output_tokens"`
			CacheRead      int64  `db:"cache_read_tokens"`
			CacheWrite     int64  `db:"cache_write_tokens"`
			Thought        *int64 `db:"reasoning_tokens"`
			Total          int64  `db:"total_tokens"`
			Cost           int64  `db:"cost_subcents"`
		}
		require.NoError(t, f.db.Get(&retained, f.db.Rebind(`SELECT s.organization_id, b.task_id, b.session_id,
			u.task_incarnation_id, u.session_incarnation_id, b.usage_binding_id, b.actor_id, b.actor_status,
			b.input_tokens, b.output_tokens, b.cache_read_tokens, b.cache_write_tokens, b.reasoning_tokens, b.total_tokens, b.cost_subcents
			FROM session_usage_buckets b JOIN usage_scopes s ON s.scope_id = b.scope_id
			JOIN usage_subjects u ON u.subject_id = b.subject_id
			WHERE b.source = 'native' AND b.source_record_id = ?`), event.UsageEventID))
		require.Equal(t, "org-a", retained.OrganizationID)
		require.Equal(t, event.TaskID, retained.TaskID)
		require.Equal(t, f.session.ID, retained.SessionID)
		require.Equal(t, f.proof.TaskIncarnationID, retained.IncarnationID)
		require.Equal(t, f.proof.SessionIncarnationID, retained.SessionBirth)
		require.Equal(t, f.proof.ID, retained.BindingID)
		require.Empty(t, retained.ActorID)
		require.Equal(t, usagehistory.ActorStatusUnknown, retained.ActorStatus)
		require.Equal(t, event.TokensIn, retained.Input)
		require.Equal(t, *event.TokensOut, retained.Output)
		require.Equal(t, *event.TokensCachedRead, retained.CacheRead)
		require.Equal(t, *event.TokensCachedWrite, retained.CacheWrite)
		require.Equal(t, event.TokensThought, retained.Thought)
		require.Equal(t, event.TokensTotal, retained.Total)
		require.Equal(t, event.CostSubcents, retained.Cost)
	}
}
