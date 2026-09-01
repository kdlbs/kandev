package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// TestPostgresOfficeTaskSessionUniqueViolation covers the typed PostgreSQL
// error branch and both repository write paths with a temporary scoped index.
// The production migration remains deferred because the shared task_sessions
// table also serves Kanban sessions.
func TestPostgresOfficeTaskSessionUniqueViolation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	const taskID = "task-office-unique-pg"
	seedPostgresTask(t, repo, taskID)

	if _, err := repo.db.Exec(`
		CREATE UNIQUE INDEX uniq_office_task_session
		ON task_sessions(task_id, agent_profile_id)
		WHERE agent_profile_id IS NOT NULL AND state IN (
			'CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT'
		)
	`); err != nil {
		t.Fatalf("create scoped index: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.db.Exec(`DROP INDEX IF EXISTS uniq_office_task_session`)
	})

	first := &models.TaskSession{
		ID:             "sess-office-unique-pg-1",
		TaskID:         taskID,
		AgentProfileID: "agent-office-pg",
		State:          models.TaskSessionStateCreated,
	}
	if err := repo.CreateTaskSession(ctx, first); err != nil {
		t.Fatalf("create first office session: %v", err)
	}

	duplicate := &models.TaskSession{
		ID:             "sess-office-unique-pg-duplicate",
		TaskID:         taskID,
		AgentProfileID: "agent-office-pg",
		State:          models.TaskSessionStateCreated,
	}
	err = repo.CreateTaskSession(ctx, duplicate)
	if err == nil || !errors.Is(err, ErrOfficeSessionRaceConflict) {
		t.Fatalf("duplicate insert error = %v, want ErrOfficeSessionRaceConflict", err)
	}

	updateCandidate := &models.TaskSession{
		ID:             "sess-office-unique-pg-update",
		TaskID:         taskID,
		AgentProfileID: "agent-other-pg",
		State:          models.TaskSessionStateCreated,
	}
	if err := repo.CreateTaskSession(ctx, updateCandidate); err != nil {
		t.Fatalf("create update candidate: %v", err)
	}
	updateCandidate.AgentProfileID = "agent-office-pg"
	changed, err := repo.UpdateTaskSessionIfCurrentState(
		ctx,
		updateCandidate,
		models.TaskSessionStateCreated,
	)
	if changed || err == nil || !errors.Is(err, ErrOfficeSessionRaceConflict) {
		t.Fatalf("conflicting update = changed=%t err=%v, want typed race conflict", changed, err)
	}
}
