package sqlite_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type eligibilityQueryCapture struct {
	*sqlx.DB
	query string
	args  []any
}

func (q *eligibilityQueryCapture) QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row {
	q.query = query
	q.args = args
	return q.DB.QueryRowxContext(ctx, query, args...)
}

func productionPayloadEligibilityDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "eligibility.db"))
	require.NoError(t, err)
	d := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	_, err = tasksqlite.NewWithDB(d, d, nil)
	require.NoError(t, err)
	_, err = officesqlite.NewWithDB(d, d, nil)
	require.NoError(t, err)
	_, err = messagequeue.NewSQLiteRepository(d, d)
	require.NoError(t, err)
	_, err = d.Exec(`INSERT INTO tasks(id,workspace_id,title,state,created_at,updated_at) VALUES('task','','retention','COMPLETED','2020-01-01','2020-01-01');
 INSERT INTO task_sessions(id,task_id,state,started_at,completed_at,updated_at) VALUES('session','task','COMPLETED','2020-01-01','2020-01-01','2020-01-01');
 INSERT INTO task_session_turns(id,task_id,task_session_id,started_at,completed_at,created_at,updated_at) VALUES('turn','task','session','2020-01-01','2020-01-01','2020-01-01','2020-01-01');`)
	require.NoError(t, err)
	return d
}

func TestPayloadEligibilityProductionIndexesAndPopulatedTask(t *testing.T) {
	d := productionPayloadEligibilityDB(t)
	const messages = 10000
	_, err := d.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<?)
 INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,content,type,metadata,created_at,updated_at)
 SELECT 'message-'||i,'session','task','turn','agent','','tool_execute',?,'2020-01-01','2020-01-01' FROM n`, messages, `{"result":"`+strings.Repeat("payload", 200)+`"}`)
	require.NoError(t, err)
	_, err = d.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<2000)
 INSERT INTO runs(id,agent_profile_id,reason,status,payload,requested_at)
 SELECT 'run-'||i,'agent','task_assigned','finished',?,'2020-01-01' FROM n`, `{"unused":"`+strings.Repeat("x", 8192)+`"}`)
	require.NoError(t, err)
	q := &eligibilityQueryCapture{DB: d}
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := time.Now()
	eligible, err := tasksqlite.PayloadTaskEligible(context.Background(), q, "task", cutoff)
	require.NoError(t, err)
	require.True(t, eligible)
	t.Logf("production indexes: %d messages + 2000 terminal runs (8KiB payload each), eligibility %s", messages, time.Since(start))
	rows, err := d.Queryx("EXPLAIN QUERY PLAN "+q.query, q.args...)
	require.NoError(t, err)
	var details []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		details = append(details, detail)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	plan := strings.Join(details, "\n")
	t.Log(plan)
	for _, index := range []string{"COVERING INDEX idx_messages_session_updated", "COVERING INDEX idx_messages_session_created", "idx_messages_task_author_created (task_id=? AND author_type=? AND type=?)", "COVERING INDEX idx_run_status_requested"} {
		require.Contains(t, plan, index)
	}
	require.Contains(t, plan, "SEARCH r USING INTEGER PRIMARY KEY")
	require.NotContains(t, plan, "SCAN r\n")
	_, err = d.Exec(`UPDATE task_session_messages SET updated_at='0000-invalid' WHERE id='message-1'`)
	require.NoError(t, err)
	eligible, err = tasksqlite.PayloadTaskEligible(context.Background(), q, "task", cutoff)
	require.NoError(t, err)
	require.False(t, eligible)
}

func TestPayloadEligibilityLargeTaskBudgetIsExplicit(t *testing.T) {
	for _, count := range []int{100000, 250000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			d := productionPayloadEligibilityDB(t)
			_, err := d.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i<?)
 INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,content,type,metadata,created_at,updated_at)
 SELECT 'message-'||i,'session','task','turn','agent','','tool_execute','{}','2020-01-01','2020-01-01' FROM n`, count)
			require.NoError(t, err)
			start := time.Now()
			eligible, err := tasksqlite.PayloadTaskEligible(context.Background(), d, "task", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			if errors.Is(err, tasksqlite.ErrPayloadEligibilityBudget) {
				require.False(t, eligible)
				t.Logf("%d-message task retained with explicit budget error after %s", count, time.Since(start))
				return
			}
			require.NoError(t, err)
			require.True(t, eligible)
			t.Logf("%d-message task validated within budget: %s", count, time.Since(start))
		})
	}
}
