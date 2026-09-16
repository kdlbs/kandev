package sqlite

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func payloadScanFixture(t *testing.T) *sqlx.DB {
	t.Helper()
	d, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	d.SetMaxOpenConns(1)
	_, err = d.Exec(`
	CREATE TABLE tasks(id TEXT PRIMARY KEY,state TEXT, queued_for_step_id TEXT,queued_at TIMESTAMP,created_at TIMESTAMP,updated_at TIMESTAMP);
	CREATE TABLE task_sessions(id TEXT PRIMARY KEY,task_id TEXT,state TEXT,started_at TIMESTAMP,completed_at TIMESTAMP,updated_at TIMESTAMP);
	CREATE INDEX sessions_task ON task_sessions(task_id);
	CREATE TABLE task_session_turns(id TEXT PRIMARY KEY,task_session_id TEXT,task_id TEXT,completed_at TIMESTAMP,updated_at TIMESTAMP,started_at TIMESTAMP,created_at TIMESTAMP,metadata TEXT DEFAULT '{}');
	CREATE INDEX turns_task ON task_session_turns(task_id);
	CREATE TABLE task_session_messages(id TEXT PRIMARY KEY,task_session_id TEXT,created_at TIMESTAMP,updated_at TIMESTAMP,type TEXT,metadata TEXT,requests_input INTEGER,task_id TEXT DEFAULT 'task',turn_id TEXT DEFAULT 'turn',author_type TEXT DEFAULT 'agent');
	CREATE INDEX idx_messages_session_updated ON task_session_messages(task_session_id,updated_at);
	CREATE INDEX idx_messages_session_created ON task_session_messages(task_session_id,created_at);
	CREATE INDEX idx_messages_task_author_created ON task_session_messages(task_id,author_type,type,created_at DESC);
	CREATE TABLE queued_messages(task_id TEXT,session_id TEXT);
	CREATE TABLE pending_moves(task_id TEXT,session_id TEXT);
	CREATE TABLE runs(status TEXT,payload TEXT,session_id TEXT,requested_at TIMESTAMP);
	CREATE INDEX idx_run_status_requested ON runs(status,requested_at);
	INSERT INTO tasks VALUES('task','COMPLETED','',NULL,'2020-01-01','2020-01-01');
	INSERT INTO task_sessions VALUES('session','task','COMPLETED','2020-01-01','2020-01-01','2020-01-01');
	INSERT INTO task_session_turns VALUES('turn','session','task','2020-01-01','2020-01-01','2020-01-01','2020-01-01','{}');
	INSERT INTO task_session_messages(id,task_session_id,created_at,updated_at,type,metadata,requests_input) VALUES('message','session','2020-01-01','2020-01-01','tool_execute','{}',0);
	`)
	require.NoError(t, err)
	return d
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.3, 002.5
func TestPayloadTaskEligibilityProtectsActivityAndAdmission(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name, mutation string
		want           bool
	}{
		{"old terminal", "", true},
		{"running", `UPDATE task_sessions SET state='RUNNING'`, false},
		{"idle", `UPDATE task_sessions SET state='IDLE'`, false},
		{"waiting", `UPDATE task_sessions SET state='WAITING_FOR_INPUT'`, false},
		{"unknown", `UPDATE task_sessions SET state='NEW_STATE'`, false},
		{"cutoff equality", `UPDATE task_session_messages SET updated_at='2026-01-01'`, false},
		{"recent creation", `UPDATE task_session_messages SET created_at='2026-06-01'`, false},
		{"renamed", `UPDATE tasks SET updated_at='2026-02-01'`, false},
		{"queue", `INSERT INTO queued_messages VALUES('task','session')`, false},
		{"move", `INSERT INTO pending_moves VALUES('task','session')`, false},
		{"run", `INSERT INTO runs VALUES('queued','{"task_id":"task"}','','2020-01-01')`, false},
		{"unknown run", `INSERT INTO runs VALUES('other','{"task_id":"task"}','','2020-01-01')`, false},
		{"open turn", `UPDATE task_session_turns SET completed_at=NULL`, false},
		{"invalid timestamp", `UPDATE task_session_messages SET updated_at='invalid'`, false},
		{"missing timestamp", `UPDATE task_session_messages SET updated_at=NULL`, false},
		{"pending permission", `UPDATE task_session_messages SET type='permission_request',metadata='{"status":"pending"}',requests_input=1`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := payloadScanFixture(t)
			if tt.mutation != "" {
				_, err := d.Exec(tt.mutation)
				require.NoError(t, err)
			}
			got, err := PayloadTaskEligible(context.Background(), d, "task", cutoff)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPayloadEligibilityAuditRegressions(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		eligible  bool
	}{
		{"empty terminal sibling", `INSERT INTO task_sessions VALUES('empty','task','CANCELLED','2020-01-01','2020-01-01','2020-01-01')`, true},
		{"no messages", `DELETE FROM task_session_messages`, true},
		{"hidden malformed update", `INSERT INTO task_session_messages(id,task_session_id,created_at,updated_at,type) VALUES('bad','session','2020-01-01','0000-invalid','message')`, false},
		{"chronological offsets", `UPDATE task_session_messages SET updated_at='2025-12-31T23:30:00-01:00'; INSERT INTO task_session_messages(id,task_session_id,created_at,updated_at,type) VALUES('other','session','2020-01-01','2026-01-01T00:00:00+01:00','message')`, false},
		{"invalid calendar date", `UPDATE task_session_messages SET updated_at='2020-02-31'`, false},
		{"Julian numeric", `UPDATE task_session_messages SET updated_at='1'`, false},
		{"new turn creation", `UPDATE task_session_turns SET created_at='2026-01-02'`, false},
		{"invalid turn start", `UPDATE task_session_turns SET started_at='bad'`, false},
		{"foreign turn ownership", `UPDATE task_session_turns SET task_id='foreign',completed_at=NULL`, false},
		{"foreign session ownership", `UPDATE task_session_turns SET task_session_id='foreign'`, false},
		{"queued session mismatch", `INSERT INTO queued_messages VALUES('foreign','session')`, false},
		{"foreign message task", `UPDATE task_session_messages SET task_id='foreign'`, false},
		{"foreign message session", `UPDATE task_session_messages SET task_session_id='foreign'`, false},
		{"missing message turn", `UPDATE task_session_messages SET turn_id='missing'`, false},
		{"clarification delivery", `UPDATE task_session_messages SET type='clarification_request',metadata='{"status":"answered","response_delivery_pending":true}'`, false},
		{"clarification pending", `UPDATE task_session_messages SET type='clarification_request',metadata='{"status":"pending"}'`, false},
		{"clarification answered", `UPDATE task_session_messages SET type='clarification_request',metadata='{"status":"answered"}'`, true},
		{"unknown permission status", `UPDATE task_session_messages SET type='permission_request',metadata='{"status":"cancelled"}'`, false},
		{"permission expired", `UPDATE task_session_messages SET type='permission_request',metadata='{"status":"expired"}'`, true},
		{"permission wrong author", `UPDATE task_session_messages SET type='permission_request',author_type='user',metadata='{"status":"approved"}'`, false},
		{"cancelled run", `INSERT INTO runs VALUES('cancelled','{"task_id":"task"}','session','2020-01-01')`, true},
		{"unknown run owner", `INSERT INTO runs VALUES('claimed','{}','','2020-01-01')`, false},
		{"pending turn dispatch", `UPDATE task_session_turns SET metadata='{"prompt_dispatch_pending":true}'`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := payloadScanFixture(t)
			_, err := d.Exec(tc.sql)
			require.NoError(t, err)
			eligible, err := PayloadTaskEligible(context.Background(), d, "task", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.Equal(t, tc.eligible, eligible)
		})
	}
}

func TestPayloadEligibilityBudgetAndCancellationFailClosed(t *testing.T) {
	d := payloadScanFixture(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	eligible, err := PayloadTaskEligible(ctx, d, "task", time.Now())
	require.False(t, eligible)
	require.ErrorIs(t, err, ErrPayloadEligibilityBudget)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	eligible, err = PayloadTaskEligible(ctx, d, "task", time.Now())
	require.False(t, eligible)
	require.ErrorIs(t, err, context.Canceled)
}

func TestPayloadEligibilityChecksEveryTimestamp(t *testing.T) {
	for _, table := range []struct {
		name    string
		columns []string
	}{
		{"tasks", []string{"created_at", "updated_at"}},
		{"task_sessions", []string{"started_at", "completed_at", "updated_at"}},
		{"task_session_turns", []string{"started_at", "created_at", "completed_at", "updated_at"}},
		{"task_session_messages", []string{"created_at", "updated_at"}},
	} {
		for _, column := range table.columns {
			for _, value := range []string{"NULL", "''", "'0000-invalid'", "'2020-02-31'", "'2020-01-01T24:00:00Z'", "'0001-01-01T00:00:00Z'", "'2026-01-01T00:00:00Z'", "1"} {
				t.Run(table.name+"/"+column+"/"+value, func(t *testing.T) {
					d := payloadScanFixture(t)
					_, err := d.Exec("UPDATE " + table.name + " SET " + column + "=" + value)
					require.NoError(t, err)
					eligible, err := PayloadTaskEligible(context.Background(), d, "task", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
					require.NoError(t, err)
					require.False(t, eligible)
				})
			}
		}
	}
}

func TestPayloadEligibilityControlMetadataFailsClosed(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"malformed run", `INSERT INTO runs VALUES('claimed','{bad','','2020-01-01')`},
		{"null run", `INSERT INTO runs VALUES('claimed','null','','2020-01-01')`},
		{"array run", `INSERT INTO runs VALUES('claimed','[]','','2020-01-01')`},
		{"unknown run status", `INSERT INTO runs VALUES(NULL,'{"task_id":"task"}','','2020-01-01')`},
		{"permission invalid JSON", `UPDATE task_session_messages SET type='permission_request',metadata='{bad'`},
		{"permission scalar JSON", `UPDATE task_session_messages SET type='permission_request',metadata='true'`},
		{"clarification invalid JSON", `UPDATE task_session_messages SET type='clarification_request',metadata='{bad'`},
		{"clarification unknown status", `UPDATE task_session_messages SET type='clarification_request',metadata='{"status":"future"}'`},
		{"pending permission dispatch", `UPDATE task_session_messages SET type='permission_request',metadata='{"status":"approved","permission_resolution":{"result":"dispatching"}}'`},
		{"unknown permission dispatch", `UPDATE task_session_messages SET type='permission_request',metadata='{"status":"approved","permission_resolution":{"result":"future"}}'`},
		{"null pending delivery flag", `UPDATE task_session_messages SET type='clarification_request',metadata='{"status":"answered","response_delivery_pending":null}'`},
		{"invalid turn metadata", `UPDATE task_session_turns SET metadata='{bad'`},
		{"turn start outbox pending", `UPDATE task_session_turns SET metadata='{"prompt_dispatch_start_event_pending":true}'`},
		{"null turn dispatch flag", `UPDATE task_session_turns SET metadata='{"prompt_dispatch_pending":null}'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := payloadScanFixture(t)
			_, err := d.Exec(tc.sql)
			require.NoError(t, err)
			eligible, err := PayloadTaskEligible(context.Background(), d, "task", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
			require.NoError(t, err)
			require.False(t, eligible)
		})
	}
}
