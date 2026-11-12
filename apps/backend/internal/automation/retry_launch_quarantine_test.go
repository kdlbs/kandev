package automation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetryLegacyQuarantineUsesOpenRecoveryFenceIndex(t *testing.T) {
	ctx := context.Background()
	store := setupTestStore(t)
	_, err := store.db.Exec(`
		CREATE TABLE task_creation_authorizations(kind TEXT, creation_id TEXT, task_id TEXT, PRIMARY KEY(kind, creation_id));
		CREATE TABLE tasks(id TEXT PRIMARY KEY, metadata TEXT DEFAULT '{}', archived_at TIMESTAMP);
		CREATE TABLE task_sessions(id TEXT PRIMARY KEY, task_id TEXT, is_primary INTEGER DEFAULT 0,
			state TEXT DEFAULT 'RUNNING', queue_incarnation_id TEXT DEFAULT '', metadata TEXT DEFAULT '{}');
		CREATE TABLE task_session_turns(id TEXT PRIMARY KEY, task_id TEXT, task_session_id TEXT, metadata TEXT DEFAULT '{}');
		CREATE TABLE harness_session_generations(session_id TEXT, incarnation_id TEXT, generation BIGINT);
		CREATE TABLE session_recovery_blocks(id TEXT PRIMARY KEY, session_id TEXT, incarnation_id TEXT,
			expected_generation BIGINT, reason TEXT, state TEXT, consumer_reference TEXT,
			created_at TIMESTAMP, updated_at TIMESTAMP);
		CREATE UNIQUE INDEX session_recovery_blocks_open_reason ON session_recovery_blocks(
			session_id, incarnation_id, expected_generation, reason) WHERE state = 'open';
	`)
	require.NoError(t, err)
	_, err = store.db.Exec(`INSERT INTO tasks(id) VALUES ('legacy-task');
		INSERT INTO task_sessions(id, task_id, queue_incarnation_id) VALUES ('legacy-session', 'legacy-task', 'incarnation');`)
	require.NoError(t, err)
	automation := &Automation{ID: "legacy-quarantine", WorkspaceID: "workspace", Name: "legacy", Enabled: true}
	require.NoError(t, store.CreateAutomation(ctx, automation))
	run := &AutomationRun{
		ID: "legacy-quarantine-run", AutomationID: automation.ID, TaskID: "legacy-task",
		Status: RunStatusTriggered, RetryGroupID: "legacy-group", RetryGroupGeneration: 1,
	}
	require.NoError(t, store.CreateRun(ctx, run))

	require.NoError(t, store.installRetryIdentityGuards(ctx))
	var block struct {
		ID         string `db:"id"`
		SessionID  string `db:"session_id"`
		Generation int64  `db:"expected_generation"`
		State      string `db:"state"`
	}
	require.NoError(t, store.db.GetContext(ctx, &block, `SELECT id, session_id, expected_generation, state
		FROM session_recovery_blocks WHERE session_id = 'legacy-session'`))
	require.Equal(t, "legacy-session", block.SessionID)
	require.EqualValues(t, 0, block.Generation)
	require.Equal(t, "open", block.State)

	require.NoError(t, store.installRetryIdentityGuards(ctx))
	var count int
	require.NoError(t, store.db.GetContext(ctx, &count, `SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = 'legacy-session'`))
	require.Equal(t, 1, count)
}
