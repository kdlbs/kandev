package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func createUsageTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "usage.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func usageFixtureRepositories(t *testing.T, database *sqlx.DB) *tasksqlite.Repository {
	t.Helper()
	tasks, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(database, database)
	require.NoError(t, err)
	retainer, err := NewWithDB(database, database)
	require.NoError(t, err)
	tasks.SetUsageHistoryRetainer(retainer)
	return tasks
}

func seedUsageFixtures(t *testing.T, database *sqlx.DB) {
	t.Helper()
	seedUsageWorkspace(t, database, usageFixtureRepositories(t, database), "ws-1", "task-1", "session-1", "transcript-1")
}

func seedSecondUsageWorkspace(t *testing.T, database *sqlx.DB) {
	t.Helper()
	seedUsageWorkspace(t, database, usageFixtureRepositories(t, database), "ws-2", "task-2", "session-2", "transcript-2")
}

func seedUsageWorkspace(t *testing.T, database *sqlx.DB, tasks *tasksqlite.Repository, workspace, task, session, transcript string) {
	t.Helper()
	require.NoError(t, tasks.CreateWorkspace(t.Context(), &taskmodels.Workspace{ID: workspace, Name: "Usage"}))
	require.NoError(t, tasks.CreateTask(t.Context(), &taskmodels.Task{ID: task, WorkspaceID: workspace, Title: "Usage task"}))
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := database.Exec(database.Rebind(`UPDATE tasks SET created_at=?,updated_at=? WHERE id=?`), created, created, task)
	require.NoError(t, err)
	seedUsageSession(t, tasks, task, session, transcript)
}

func seedUsageSession(t *testing.T, tasks *tasksqlite.Repository, task, session, transcript string) {
	t.Helper()
	require.NoError(t, tasks.CreateTaskSession(t.Context(), &taskmodels.TaskSession{
		ID: session, TaskID: task, QueueIncarnationID: session, AgentProfileID: "agent-1", State: taskmodels.TaskSessionStateRunning,
		StartedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Metadata:  map[string]any{"acp": map[string]any{"session_id": transcript}},
	}))
}

func seedExtraUsageSession(t *testing.T, database *sqlx.DB) string {
	t.Helper()
	seedUsageSession(t, usageFixtureRepositories(t, database), "task-1", "session-extra", "transcript-2")
	return "session-extra"
}

func insertNativeUsageEvent(t *testing.T, database *sqlx.DB, id int, model, provider string, input, cost, total, output int64, costSource string, occurred time.Time) {
	t.Helper()
	_, err := database.Exec(database.Rebind(`INSERT INTO usage_fixture_legacy_native_inputs (
		id,task_id,session_id,model,provider,tokens_in,tokens_cached_read,tokens_cached_write,tokens_out,tokens_thought,
		tokens_total,cost_subcents,cost_source,occurred_at,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`),
		id, "task-1", "session-1", model, provider, input, 0, 0, output, 0, total, cost, costSource, occurred, occurred)
	require.NoError(t, err)
	// This writer models a ledger captured before usage-history retention was
	// installed. Backfill must ingest its stable event ID; no Retainer is attached.
	writer, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	event := &taskmodels.TaskUsageEvent{UsageEventID: fmt.Sprintf("legacy-event-%d", id), UsageCompleteness: "complete", ContractVersion: 1}
	var recordedCost sql.NullInt64
	require.NoError(t, database.QueryRowx(database.Rebind(`SELECT task_id,COALESCE(session_id,''),model,provider,tokens_in,
		tokens_cached_read,tokens_cached_write,tokens_out,tokens_thought,tokens_total,cost_subcents,cost_source,occurred_at,created_at
		FROM usage_fixture_legacy_native_inputs WHERE id=?`), id).Scan(&event.TaskID, &event.SessionID, &event.Model, &event.Provider,
		&event.TokensIn, &event.TokensCachedRead, &event.TokensCachedWrite, &event.TokensOut, &event.TokensThought,
		&event.TokensTotal, &recordedCost, &event.CostSource, &event.OccurredAt, &event.CreatedAt))
	event.CostSubcents = recordedCost.Int64
	if !recordedCost.Valid {
		event.CostSource = "unpriced"
	}
	event.Estimated = event.CostSource == usageCostSourceEstimated
	require.NoError(t, writer.CreateTaskUsageEvent(t.Context(), event))
	var retained int
	require.NoError(t, database.Get(&retained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
	require.Zero(t, retained, "legacy ledger inputs must reach retention through actual backfill")
}

func assertUsageFixtureNativeMigration(t *testing.T, repo *Repository) {
	t.Helper()
	var ledger, retained, exact int
	require.NoError(t, repo.db.Get(&ledger, `SELECT COUNT(*) FROM task_usage_events`))
	require.NoError(t, repo.db.Get(&retained, `SELECT COUNT(*) FROM session_usage_buckets WHERE source='native'`))
	require.NoError(t, repo.db.Get(&exact, `SELECT COUNT(*) FROM task_usage_events e WHERE
		(SELECT COUNT(*) FROM session_usage_buckets b WHERE b.source='native' AND b.source_record_id=e.usage_event_id)=1`))
	require.Equal(t, ledger, retained, "actual native migration must neither lose nor duplicate ledger rows")
	require.Equal(t, ledger, exact, "actual migration must preserve every stable event ID exactly once")
	t.Logf("actual native migration: ledger=%d retained=%d exact_event_id_matches=%d", ledger, retained, exact)
}

func finishUsageFixtureSelection(t *testing.T, repo *Repository) {
	t.Helper()
	for range 100 {
		done, err := repo.BackfillUsageSelection(t.Context(), 200)
		require.NoError(t, err)
		if done {
			return
		}
	}
	t.Fatal("usage selection fixture backfill did not finish")
}

func logUsageFixtureWorkProof(t *testing.T, repo *Repository) {
	t.Helper()
	var proof struct {
		NativeLedgerRows     int
		OriginalThreadColumn bool
		Retained             []struct {
			Source     string `db:"source"`
			Scope      string `db:"scope_id"`
			Subject    string `db:"subject_id"`
			Binding    string `db:"usage_binding_id"`
			Transcript string `db:"transcript_id"`
			Work       string `db:"selection_partition"`
			Status     string `db:"work_status"`
			Event      string `db:"work_event_id"`
			Record     string `db:"source_record_id"`
		}
		Aliases []struct {
			Scope    string `db:"scope_id"`
			Subject  string `db:"subject_id"`
			Scheme   string `db:"scheme"`
			External string `db:"external_id"`
			Event    string `db:"event_id"`
		}
	}
	var err error
	proof.OriginalThreadColumn, err = db.ColumnExists(repo.db, "task_usage_events", "provider_thread_id")
	require.NoError(t, err)
	require.NoError(t, repo.db.Get(&proof.NativeLedgerRows, `SELECT COUNT(*) FROM task_usage_events`))
	columns := `source,scope_id,subject_id,usage_binding_id,transcript_id,selection_partition,work_status,work_event_id,source_record_id`
	require.NoError(t, repo.db.Select(&proof.Retained, `SELECT `+columns+` FROM session_usage_measurements UNION ALL SELECT `+columns+` FROM session_usage_buckets ORDER BY source`))
	require.NoError(t, repo.db.Select(&proof.Aliases, `SELECT scope_id,subject_id,scheme,external_id,event_id FROM usage_work_identities ORDER BY scheme,external_id`))
	encoded, err := json.Marshal(proof)
	require.NoError(t, err)
	t.Logf("usage work proof: %s", encoded)
}
