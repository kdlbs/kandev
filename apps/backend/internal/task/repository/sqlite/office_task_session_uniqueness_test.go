package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// officeSessionSeed describes a task_sessions row to insert directly,
// bypassing CreateTaskSession so tests can construct rows with a specific
// (task_id, agent_profile_id, state) shape.
type officeSessionSeed struct {
	id             string
	taskID         string
	agentProfileID string
	state          models.TaskSessionState
}

func insertOfficeSession(t *testing.T, db *sqlx.DB, s officeSessionSeed) {
	t.Helper()
	now := time.Now().UTC()
	_, err := db.Exec(`
		INSERT INTO task_sessions (
			id, task_id, agent_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id
		) VALUES (?, ?, ?, '', '', '', '', '', '',
		          '{}', '{}', '{}', '{}',
		          ?, '', '{}', ?, NULL, ?,
		          0, '', 0, '')
	`, s.id, s.taskID, s.agentProfileID, string(s.state), now, now)
	if err != nil {
		t.Fatalf("insert office session %s: %v", s.id, err)
	}
}

// TestIsOfficeTaskSessionUniqueViolation_ClassifiesSQLiteMessage exercises
// isOfficeTaskSessionUniqueViolation against a real SQLite UNIQUE-constraint
// error, then drives both production call sites that wrap it in
// ErrOfficeSessionRaceConflict (createTaskSession's INSERT and
// updateTaskSessionWithStateGuard's UPDATE) to prove the wrapping itself,
// not just the classifier. uniq_office_task_session is not wired into
// base_schema.go — a table-wide partial index on this pair broke live
// kanban-relaunch and workflow-replacement flows (see the follow-up card
// linked from ErrOfficeSessionRaceConflict's doc comment) — so this test
// builds and drops its own scoped index rather than relying on production
// schema.
func TestIsOfficeTaskSessionUniqueViolation_ClassifiesSQLiteMessage(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-classify-1")
	agent := "agent-classify"

	if _, err := repo.db.Exec(`
		CREATE UNIQUE INDEX uniq_office_task_session_probe
		ON task_sessions(task_id, agent_profile_id)
		WHERE agent_profile_id IS NOT NULL AND state IN (
			'CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT'
		)
	`); err != nil {
		t.Fatalf("create scoped index: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.db.Exec(`DROP INDEX IF EXISTS uniq_office_task_session_probe`)
	})

	insertOfficeSession(t, repo.db, officeSessionSeed{
		id: "sess-classify-1", taskID: "task-classify-1", agentProfileID: agent,
		state: models.TaskSessionStateCreated,
	})

	_, err := repo.db.Exec(`
		INSERT INTO task_sessions (
			id, task_id, agent_profile_id, executor_id, executor_profile_id, environment_id,
			repository_id, base_branch, base_commit_sha,
			agent_profile_snapshot, executor_snapshot, environment_snapshot, repository_snapshot,
			state, error_message, metadata, started_at, completed_at, updated_at,
			is_primary, review_status, is_passthrough, task_environment_id
		) VALUES (?, 'task-classify-1', ?, '', '', '', '', '', '',
		          '{}', '{}', '{}', '{}',
		          'CREATED', '', '{}', datetime('now'), NULL, datetime('now'),
		          0, '', 0, '')
	`, "sess-classify-2", agent)
	if err == nil {
		t.Fatal("expected inserting a second live row for the same pair to violate the scoped index")
	}
	if !isOfficeTaskSessionUniqueViolation(err) {
		t.Fatalf("expected err to classify as an office task session unique violation, got: %v", err)
	}

	// The raw INSERT above only proves SQLite's message format. Also drive
	// the two real production call sites that route through the
	// classifier, so a change that breaks either wrapping fails here rather
	// than only after the follow-up index lands.
	ctx := context.Background()

	// CreateTaskSession -> createTaskSession's INSERT path.
	createErr := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "sess-classify-3", TaskID: "task-classify-1", AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	if createErr == nil {
		t.Fatal("expected CreateTaskSession into an occupied live (task, agent) slot to fail")
	}
	if !errors.Is(createErr, ErrOfficeSessionRaceConflict) {
		t.Fatalf("expected errors.Is(err, ErrOfficeSessionRaceConflict) from CreateTaskSession, got: %v", createErr)
	}

	// UpdateTaskSession -> updateTaskSessionWithStateGuard's UPDATE path
	// (e.g. a terminal-session resume racing another live row into the
	// same slot).
	insertOfficeSession(t, repo.db, officeSessionSeed{
		id: "sess-classify-4", taskID: "task-classify-1", agentProfileID: "agent-classify-other",
		state: models.TaskSessionStateCreated,
	})
	updateErr := repo.UpdateTaskSession(ctx, &models.TaskSession{
		ID: "sess-classify-4", TaskID: "task-classify-1", AgentProfileID: agent,
		State: models.TaskSessionStateCreated,
	})
	if updateErr == nil {
		t.Fatal("expected UpdateTaskSession moving a session into an occupied live (task, agent) slot to fail")
	}
	if !errors.Is(updateErr, ErrOfficeSessionRaceConflict) {
		t.Fatalf("expected errors.Is(err, ErrOfficeSessionRaceConflict) from UpdateTaskSession, got: %v", updateErr)
	}
}

// TestIsOfficeTaskSessionUniqueViolation_IgnoresUnrelatedViolation asserts
// the classifier does not fire on an unrelated UNIQUE-constraint error (the
// tasks primary key here), which a bare "UNIQUE constraint failed" substring
// match would have misclassified.
func TestIsOfficeTaskSessionUniqueViolation_IgnoresUnrelatedViolation(t *testing.T) {
	repo := newRepoForHealTests(t)
	insertTask(t, repo.db, "task-classify-2")

	_, err := repo.db.Exec(`
		INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, description, state, created_at, updated_at)
		VALUES ('task-classify-2', '', '', '', 'dup', '', 'todo', datetime('now'), datetime('now'))
	`)
	if err == nil {
		t.Fatal("expected duplicate task id to violate the tasks primary key")
	}
	if isOfficeTaskSessionUniqueViolation(err) {
		t.Fatalf("expected primary-key violation not to be misclassified as an office task session unique violation: %v", err)
	}
}
