package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

// TestAgentProfileColumnExists verifies the unified agent_profile_id column
// is present (kanban + office now share it post-ADR-0005).
func TestAgentProfileColumnExists(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()

	// Verify column exists by querying it.
	var dummy any
	err := repo.db.QueryRow(`SELECT agent_profile_id FROM task_sessions LIMIT 0`).Scan(&dummy)
	if err != nil && err.Error() != "sql: no rows in result set" {
		t.Fatalf("agent_profile_id column missing: %v", err)
	}

	// The pre-ADR partial unique index over (task_id, agent_profile_id) was
	// removed — it can't distinguish kanban (many sessions per profile) from
	// office (one per profile) without a discriminator column. Office's
	// per-(task, agent) uniqueness is enforced by EnsureSessionForAgent.
	var indexSQL string
	err = repo.db.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='uniq_office_task_session'`,
	).Scan(&indexSQL)
	if err == nil {
		t.Errorf("expected uniq_office_task_session index to be dropped, found: %q", indexSQL)
	}
	_ = strings.Contains // keep import live if other tests need it

	// Two kanban-style rows for the same (task, profile) must coexist after
	// the index is gone.
	taskID := insertTaskRow(t, repo)
	for i := 0; i < 2; i++ {
		s := &models.TaskSession{
			TaskID:         taskID,
			AgentProfileID: "profile-x",
			State:          models.TaskSessionStateCreated,
			StartedAt:      time.Now().UTC(),
		}
		if err := repo.CreateTaskSession(ctx, s); err != nil {
			t.Fatalf("kanban insert %d: %v", i, err)
		}
	}
}

// TestGetTaskSessionByTaskAndAgent returns the matching row, or nil when none.
func TestGetTaskSessionByTaskAndAgent(t *testing.T) {
	repo := newRepoForHealTests(t)
	ctx := context.Background()
	taskID := insertTaskRow(t, repo)

	got, err := repo.GetTaskSessionByTaskAndAgent(ctx, taskID, "agent-missing")
	if err != nil {
		t.Fatalf("lookup miss: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing pair, got %+v", got)
	}

	s := &models.TaskSession{
		TaskID:         taskID,
		AgentProfileID: "agent-7",
		State:          models.TaskSessionStateRunning,
		StartedAt:      time.Now().UTC(),
	}
	if err := repo.CreateTaskSession(ctx, s); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err = repo.GetTaskSessionByTaskAndAgent(ctx, taskID, "agent-7")
	if err != nil {
		t.Fatalf("lookup hit: %v", err)
	}
	if got == nil {
		t.Fatalf("expected session, got nil")
	}
	if got.ID != s.ID {
		t.Errorf("got session %q, want %q", got.ID, s.ID)
	}
}

// insertTaskRow inserts a minimal task row directly so session FKs validate.
func insertTaskRow(t *testing.T, repo *Repository) string {
	t.Helper()
	taskID := uuid.New().String()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(
		`INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, title, created_at, updated_at)
		 VALUES (?, '', '', '', ?, ?, ?)`,
		taskID, "test-task", now, now,
	); err != nil {
		t.Fatalf("insert task: %v", err)
	}
	return taskID
}
