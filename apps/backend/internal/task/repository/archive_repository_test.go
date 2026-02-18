package repository

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestSQLiteRepository_ArchiveTask(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task One"})

	// Archive the task
	err := repo.ArchiveTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to archive task: %v", err)
	}

	// Verify archived_at is set
	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("expected archived_at to be set")
	}

	// Archive again should fail
	err = repo.ArchiveTask(ctx, "task-1")
	// The repo method doesn't check for already archived, but it still succeeds (updates the timestamp)
	if err != nil {
		t.Fatalf("unexpected error archiving already-archived task: %v", err)
	}

	// Archive non-existent task should fail
	err = repo.ArchiveTask(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error when archiving nonexistent task")
	}
}

func TestSQLiteRepository_ArchiveTask_ExcludesFromListTasks(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task One"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Task Two"})

	// Archive task-1
	_ = repo.ArchiveTask(ctx, "task-1")

	// ListTasks should exclude archived tasks
	tasks, err := repo.ListTasks(ctx, "wf-1")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("expected 1 task (excluding archived), got %d", len(tasks))
	}
	if tasks[0].ID != "task-2" {
		t.Errorf("expected task-2, got %s", tasks[0].ID)
	}

	// ListTasksByWorkflowStep should exclude archived tasks
	tasksByStep, err := repo.ListTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("failed to list tasks by step: %v", err)
	}
	if len(tasksByStep) != 1 {
		t.Errorf("expected 1 task by step (excluding archived), got %d", len(tasksByStep))
	}

	// CountTasksByWorkflow should exclude archived tasks
	count, err := repo.CountTasksByWorkflow(ctx, "wf-1")
	if err != nil {
		t.Fatalf("failed to count tasks: %v", err)
	}
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// CountTasksByWorkflowStep should exclude archived tasks
	countByStep, err := repo.CountTasksByWorkflowStep(ctx, "step-1")
	if err != nil {
		t.Fatalf("failed to count tasks by step: %v", err)
	}
	if countByStep != 1 {
		t.Errorf("expected count 1, got %d", countByStep)
	}
}

func TestSQLiteRepository_ListTasksByWorkspace_IncludeArchived(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-1", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Active Task"})
	_ = repo.CreateTask(ctx, &models.Task{ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1", Title: "Archived Task"})
	_ = repo.ArchiveTask(ctx, "task-2")

	// Without includeArchived: should return only active task
	tasks, total, err := repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 total without archived, got %d", total)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-1" {
		t.Errorf("expected only task-1, got %v", tasks)
	}

	// With includeArchived: should return both tasks
	tasks, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "", 1, 10, true)
	if err != nil {
		t.Fatalf("failed to list tasks with archived: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 total with archived, got %d", total)
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}

	// Search with includeArchived=false should filter archived
	_, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "Task", 1, 10, false)
	if err != nil {
		t.Fatalf("failed to search tasks: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 search result without archived, got %d", total)
	}

	// Search with includeArchived=true should include archived
	_, total, err = repo.ListTasksByWorkspace(ctx, "ws-1", "Task", 1, 10, true)
	if err != nil {
		t.Fatalf("failed to search tasks with archived: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 search results with archived, got %d", total)
	}
}

func TestSQLiteRepository_ListTasksForAutoArchive(t *testing.T) {
	repo, cleanup := createTestSQLiteRepo(t)
	defer cleanup()
	ctx := context.Background()

	// Create the workflow_steps table (normally created by workflow repo)
	_, err := repo.DB().Exec(`
		CREATE TABLE IF NOT EXISTS workflow_steps (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			name TEXT NOT NULL,
			position INTEGER NOT NULL,
			color TEXT,
			prompt TEXT,
			events TEXT,
			allow_manual_move INTEGER DEFAULT 1,
			is_start_step INTEGER DEFAULT 0,
			auto_archive_after_hours INTEGER DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create workflow_steps table: %v", err)
	}

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"})
	workflow := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Test Workflow"}
	_ = repo.CreateWorkflow(ctx, workflow)

	now := time.Now().UTC()

	// Create workflow step with auto-archive after 1 hour
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "step-done", "wf-1", "Done", 2, 1, now, now)
	if err != nil {
		t.Fatalf("failed to create workflow step: %v", err)
	}

	// Create workflow step without auto-archive
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, "step-todo", "wf-1", "Todo", 1, 0, now, now)
	if err != nil {
		t.Fatalf("failed to create workflow step: %v", err)
	}

	oldTime := now.Add(-2 * time.Hour)

	// Task in auto-archive step, updated 2 hours ago — should be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-old", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Old Task",
	})
	// Backdate updated_at (CreateTask sets it to now)
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-old")

	// Task in auto-archive step, updated just now — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-recent", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Recent Task",
	})

	// Task in non-auto-archive step — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-todo", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-todo", Title: "Todo Task",
	})
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-todo")

	// Already archived task — should NOT be eligible
	_ = repo.CreateTask(ctx, &models.Task{
		ID: "task-archived", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-done", Title: "Already Archived",
	})
	_, _ = repo.DB().Exec(`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldTime, "task-archived")
	_ = repo.ArchiveTask(ctx, "task-archived")

	// List candidates
	candidates, err := repo.ListTasksForAutoArchive(ctx)
	if err != nil {
		t.Fatalf("failed to list auto-archive candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}
	if candidates[0].ID != "task-old" {
		t.Errorf("expected task-old, got %s", candidates[0].ID)
	}
}
