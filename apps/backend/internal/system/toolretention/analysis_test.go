package toolretention

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func seedAnalysis(t *testing.T, s *Service) string {
	t.Helper()
	_, err := s.pool.Writer().Exec(`
	CREATE TABLE tasks(id TEXT PRIMARY KEY,state TEXT, queued_for_step_id TEXT,queued_at TIMESTAMP,created_at TIMESTAMP,updated_at TIMESTAMP);
	CREATE TABLE task_sessions(id TEXT PRIMARY KEY,task_id TEXT,state TEXT,started_at TIMESTAMP,completed_at TIMESTAMP,updated_at TIMESTAMP);
	CREATE INDEX sessions_task ON task_sessions(task_id,id);
	CREATE TABLE task_session_turns(id TEXT PRIMARY KEY,task_session_id TEXT,task_id TEXT,completed_at TIMESTAMP,updated_at TIMESTAMP,started_at TIMESTAMP DEFAULT '2020-01-01',created_at TIMESTAMP DEFAULT '2020-01-01',metadata TEXT DEFAULT '{}');
	CREATE INDEX turns_task ON task_session_turns(task_id);
	CREATE TABLE task_session_messages(id TEXT PRIMARY KEY,task_session_id TEXT,task_id TEXT,created_at TIMESTAMP,updated_at TIMESTAMP,type TEXT,metadata TEXT,requests_input INTEGER,author_type TEXT DEFAULT 'agent',turn_id TEXT DEFAULT 'turn');
	CREATE INDEX idx_messages_session_updated ON task_session_messages(task_session_id,updated_at);
	CREATE INDEX idx_messages_session_created ON task_session_messages(task_session_id,created_at);
	CREATE INDEX messages_session ON task_session_messages(task_session_id);
	CREATE TABLE queued_messages(task_id TEXT,session_id TEXT);
	CREATE TABLE pending_moves(task_id TEXT,session_id TEXT);
	CREATE TABLE runs(status TEXT,payload TEXT,session_id TEXT,requested_at TIMESTAMP);
	CREATE INDEX idx_run_status_requested ON runs(status,requested_at);
	CREATE INDEX idx_messages_task_author_created ON task_session_messages(task_id,author_type,type,created_at DESC);
	INSERT INTO tasks VALUES('task','COMPLETED','',NULL,'2020-01-01','2020-01-01');
	INSERT INTO task_sessions VALUES('session','task','COMPLETED','2020-01-01','2020-01-01','2020-01-01');
	INSERT INTO task_session_turns(id,task_session_id,task_id,completed_at,updated_at) VALUES('turn','session','task','2020-01-01','2020-01-01');
	`)
	require.NoError(t, err)
	raw, _ := json.Marshal(map[string]any{"title": "echo result", "tool_call_id": "tool1", "status": "complete", "normalized": map[string]any{"kind": "shell_exec", "shell_exec": map[string]any{"command": "echo result", "output": map[string]any{"stdout": strings.Repeat("payload", 1000), "exit_code": 0}}}})
	_, err = s.pool.Writer().Exec(`INSERT INTO task_session_messages(id,task_session_id,task_id,created_at,updated_at,type,metadata,requests_input) VALUES('message','session','task','2020-01-01','2020-01-01','tool_execute',?,0)`, string(raw))
	require.NoError(t, err)
	return string(raw)
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.4, 001.5, 003.2
func TestAnalysisIsReadOnlyAndCountsExactEligiblePayloads(t *testing.T) {
	s := testService(t)
	before := seedAnalysis(t, s)
	ctx := context.Background()
	id, err := s.Analyze(ctx, Age{3, "months"})
	require.NoError(t, err)
	require.NotEmpty(t, id)
	for i := 0; i < 10; i++ {
		require.NoError(t, s.stepScan(ctx))
		status, err := s.Get(ctx)
		require.NoError(t, err)
		if status.Operation.State != "running" {
			break
		}
	}
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, "succeeded", status.Operation.State)
	require.EqualValues(t, 1, status.Operation.EligibleTasks)
	require.EqualValues(t, 1, status.Operation.EligibleMessages)
	require.Greater(t, status.Operation.PayloadBytes, int64(6000))
	require.False(t, status.Policy.Enabled)
	var after string
	require.NoError(t, s.pool.Reader().Get(&after, `SELECT metadata FROM task_session_messages WHERE id='message'`))
	require.Equal(t, before, after)
}

func TestAnalysisCancellationAndSingleOperation(t *testing.T) {
	s := testService(t)
	seedAnalysis(t, s)
	ctx := context.Background()
	id, err := s.Analyze(ctx, Age{3, "months"})
	require.NoError(t, err)
	_, err = s.Analyze(ctx, Age{1, "weeks"})
	require.ErrorContains(t, err, "busy")
	_, err = s.Cancel(ctx, "wrong")
	require.ErrorContains(t, err, "conflict")
	status, err := s.Cancel(ctx, id)
	require.NoError(t, err)
	require.Equal(t, "cancelled", status.Operation.State)
	require.NoError(t, s.stepScan(ctx))
	status, err = s.Get(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, status.Operation.PayloadBytes)
}
