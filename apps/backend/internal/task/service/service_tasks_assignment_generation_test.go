package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// setupOfficeTestWithBus mirrors setupOfficeTest but keeps the MockEventBus
// so a test can inspect the payload of a published task.created event.
func setupOfficeTestWithBus(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	svc, bus, repo := createTestService(t)
	ctx := context.Background()

	_, err := repo.DB().Exec(`
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT DEFAULT '',
			prompt TEXT DEFAULT '',
			events TEXT DEFAULT '{}',
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			show_in_command_panel INTEGER DEFAULT 1,
			auto_archive_after_hours INTEGER DEFAULT 0,
			agent_profile_id TEXT DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE
		)`)
	if err != nil {
		t.Fatalf("create workflow_steps: %v", err)
	}

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := repo.EnsureOfficeWorkflow(ctx, "ws-1"); err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	svc.SetStartStepResolver(&dbStepResolver{repo: repo})
	svc.SetWorkspacePolicyAttacher(noOpWorkspacePolicyAttacher{})
	return svc, bus, repo
}

// AC-OFFICE-RUN-DEDUP-001.9: a task created already assigned publishes
// assignment_generation 1 on its task.created event, matching the value
// insertTaskTx committed - a creation and the first following assignment
// must not both mint generation 1.
func TestCreateTask_Assigned_PublishesAssignmentGenerationOne(t *testing.T) {
	svc, bus, repo := setupOfficeTestWithBus(t)
	ctx := context.Background()

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:            "ws-1",
		Title:                  "Agent Task",
		Origin:                 models.TaskOriginAgentCreated,
		AssigneeAgentProfileID: "agent-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data := singlePublishedEventData(t, bus)
	gen, ok := data["assignment_generation"].(int64)
	if !ok || gen != 1 {
		t.Fatalf("assignment_generation = %#v, want int64(1)", data["assignment_generation"])
	}

	stored, err := repo.GetTask(ctx, result.Task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	var persistedGen int64
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT assignment_generation FROM tasks WHERE id = ?`, stored.ID).Scan(&persistedGen); err != nil {
		t.Fatalf("query assignment_generation: %v", err)
	}
	if persistedGen != 1 {
		t.Fatalf("persisted assignment_generation = %d, want 1", persistedGen)
	}
}

// AC-OFFICE-RUN-DEDUP-001.9 boundary: a task created UNASSIGNED publishes
// assignment_generation 0, never 1 - the boundary that keeps a creation and a
// first assignment from both minting generation 1.
func TestCreateTask_Unassigned_PublishesAssignmentGenerationZero(t *testing.T) {
	svc, bus, repo := setupOfficeTestWithBus(t)
	ctx := context.Background()

	result, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Unassigned Task",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	data := singlePublishedEventData(t, bus)
	gen, ok := data["assignment_generation"].(int64)
	if !ok || gen != 0 {
		t.Fatalf("assignment_generation = %#v, want int64(0)", data["assignment_generation"])
	}

	var persistedGen int64
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT assignment_generation FROM tasks WHERE id = ?`, result.Task.ID).Scan(&persistedGen); err != nil {
		t.Fatalf("query assignment_generation: %v", err)
	}
	if persistedGen != 0 {
		t.Fatalf("persisted assignment_generation = %d, want 0", persistedGen)
	}
}
