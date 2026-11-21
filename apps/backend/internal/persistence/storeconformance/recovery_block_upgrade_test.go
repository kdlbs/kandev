package storeconformance

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

// @covers AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.2 AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-001.6
// @covers AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.1 AC-PLATFORM-POSTGRES-DOMAIN-STORE-PARITY-006.2
func TestRecoveryBlockUpgradeWithAutomationQuarantine(t *testing.T) {
	database := legacyRecoveryBlockUpgradeDB(t)
	repo := tasksqlite.NewWithInitializedDB(database, database, nil)
	seedRecoveryBlockUpgradeRows(t, repo, database)
	beforeOpen, err := repo.GetSessionRecoveryBlock(t.Context(), "open")
	require.NoError(t, err)
	beforeResolved, err := repo.GetSessionRecoveryBlock(t.Context(), "resolved")
	require.NoError(t, err)
	store, err := automation.NewStore(database, database)
	require.NoError(t, err)
	triggers := upgradeQuarantineDefinitions(t, database)
	require.Len(t, triggers, 4)
	repo, err = tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	require.Equal(t, triggers, upgradeQuarantineDefinitions(t, database))
	open, err := repo.GetSessionRecoveryBlock(t.Context(), "open")
	require.NoError(t, err)
	resolved, err := repo.GetSessionRecoveryBlock(t.Context(), "resolved")
	require.NoError(t, err)
	require.Equal(t, beforeOpen, open)
	require.Equal(t, beforeResolved, resolved)
	require.NoError(t, store.CreateAutomation(t.Context(), &automation.Automation{
		ID: "upgrade-automation", WorkspaceID: "upgrade-workspace", Name: "Upgrade", Enabled: true,
	}))
	for _, mode := range []string{"run_insert", "run_update", "operation_insert", "operation_update"} {
		t.Run(mode, func(t *testing.T) { assertUpgradedQuarantine(t, repo, database, store, mode) })
	}
	_, err = tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = automation.NewStore(database, database)
	require.NoError(t, err)
	require.Equal(t, triggers, upgradeQuarantineDefinitions(t, database))
}

func seedRecoveryBlockUpgradeRows(t *testing.T, repo *tasksqlite.Repository, database *sqlx.DB) {
	t.Helper()
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "upgrade-workspace", Name: "Upgrade"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
		ID: "upgrade-task", WorkspaceID: "upgrade-workspace", Title: "Upgrade",
	}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: "upgrade-session", TaskID: "upgrade-task"}))
	_, err := database.Exec(`INSERT INTO session_recovery_blocks
		(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
		 delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
		 authorized_action, created_at, updated_at, resolved_at)
		VALUES ('open', 'upgrade-session', 'incarnation', 3, 'current', 'open', 'consumer',
		 'submission', 'stream', 19, 'turn', 'unknown', '',
		 '2026-10-01 01:02:03.123456+00:00', '2026-10-02 01:02:03.654321+00:00', NULL),
		 ('resolved', 'upgrade-session', 'incarnation', 3, 'history', 'resolved', 'historical-consumer',
		 'old-submission', 'old-stream', 7, 'old-turn', 'completed', 'resume',
		 '2026-09-01 01:02:03.123456+00:00', '2026-09-02 01:02:03.654321+00:00', '2026-09-02 01:02:03.654321+00:00')`)
	require.NoError(t, err)
}

func upgradeQuarantineDefinitions(t *testing.T, database *sqlx.DB) []string {
	t.Helper()
	var triggers []string
	require.NoError(t, database.Select(&triggers, `SELECT sql FROM sqlite_master
		WHERE type = 'trigger' AND name LIKE 'automation_retry_%quarantine_%' ORDER BY name`))
	return triggers
}

func assertUpgradedQuarantine(t *testing.T, repo *tasksqlite.Repository, database *sqlx.DB, store *automation.Store, mode string) {
	t.Helper()
	taskID, sessionID, runID := mode+"-task", mode+"-session", mode+"-run"
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
		ID: taskID, WorkspaceID: "upgrade-workspace", Title: "Quarantine",
	}))
	require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: sessionID, TaskID: taskID}))
	run := &automation.AutomationRun{
		ID: runID, AutomationID: "upgrade-automation", Status: automation.RunStatusTriggered,
		RetryGroupID: mode + "-group", RetryGroupGeneration: 1,
	}
	var statement string
	var args []any
	if mode == "run_insert" {
		run.TaskID = taskID
	} else {
		require.NoError(t, store.CreateRun(t.Context(), run))
		if mode == "run_update" {
			statement, args = `UPDATE automation_runs SET task_id = ? WHERE id = ?`, []any{taskID, runID}
		} else {
			statement, args = stageUpgradeQuarantineOperation(t, database, runID, taskID, mode)
		}
	}
	var count int
	query := `SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = ? AND reason = 'automation_retry_acceptance_unresolved'`
	require.NoError(t, database.Get(&count, query, sessionID))
	require.Zero(t, count)
	if mode == "run_insert" {
		require.NoError(t, store.CreateRun(t.Context(), run))
	} else {
		_, err := database.Exec(statement, args...)
		require.NoError(t, err)
	}
	require.NoError(t, database.Get(&count, query, sessionID))
	require.Equal(t, 1, count)
	var consumer string
	require.NoError(t, database.Get(&consumer, `SELECT consumer_reference FROM session_recovery_blocks
		WHERE session_id = ? AND state = 'open' AND expected_generation = 0`, sessionID))
	require.Equal(t, "automation_retry:"+runID, consumer)
}

func stageUpgradeQuarantineOperation(t *testing.T, database *sqlx.DB, runID, taskID, mode string) (string, []any) {
	t.Helper()
	_, err := database.Exec(`INSERT INTO automation_run_task_intents
		(intent_id, run_id, state, group_generation, created_at, updated_at)
		VALUES (?, ?, 'preparing', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, runID+"-intent", runID)
	require.NoError(t, err)
	insert := `INSERT INTO automation_run_operations
		(operation_id, intent_id, run_id, group_generation, operation_kind, state, external_task_id, created_at, updated_at)
		VALUES (?, ?, ?, 1, 'task', 'preparing', ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`
	args := []any{runID + "-operation", runID + "-intent", runID, taskID}
	if mode == "operation_insert" {
		return insert, args
	}
	args[3] = ""
	_, err = database.Exec(insert, args...)
	require.NoError(t, err)
	return `UPDATE automation_run_operations SET external_task_id = ? WHERE operation_id = ?`, []any{taskID, runID + "-operation"}
}

func legacyRecoveryBlockUpgradeDB(t *testing.T) *sqlx.DB {
	t.Helper()
	engine := testconformance.OpenEngine(t, testconformance.EngineSQLite, "")
	database := engine.DB
	_, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	_, err = database.Exec(`DROP TABLE session_recovery_blocks;
		CREATE TABLE session_recovery_blocks (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			expected_generation BIGINT NOT NULL,
			reason TEXT NOT NULL,
			state TEXT NOT NULL,
			consumer_reference TEXT NOT NULL DEFAULT '',
			delivery_submission_id TEXT NOT NULL DEFAULT '',
			delivery_stream_id TEXT NOT NULL DEFAULT '',
			delivery_sequence BIGINT NOT NULL DEFAULT 0,
			delivery_turn_id TEXT NOT NULL DEFAULT '',
			delivery_outcome TEXT NOT NULL DEFAULT '',
			authorized_action TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			UNIQUE (session_id, incarnation_id, expected_generation, reason),
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		)`)
	require.NoError(t, err)
	return database
}
