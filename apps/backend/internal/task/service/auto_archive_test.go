package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestAutoArchiveUsesCascadeCoordinator(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	stepID := "step-auto-archive-coordinator"
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO workflow_steps (
			id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
	`, stepID, workspace.OfficeWorkflowID, "Auto archive", 0, 1, now, now); err != nil {
		t.Fatalf("Create auto-archive step: %v", err)
	}
	taskID := "task-auto-archive-coordinator"
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspace.ID, WorkflowID: workspace.OfficeWorkflowID,
		WorkflowStepID: stepID, Title: "Auto archive coordinator",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	oldUpdatedAt := now.Add(-48 * time.Hour)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = ?`, oldUpdatedAt, taskID); err != nil {
		t.Fatalf("age task: %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(taskSvc)
	taskSvc.SetAutoArchiveCoordinator(handoff)
	taskSvc.runAutoArchive(ctx)

	archived, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("auto-archive did not archive the eligible task")
	}
	if archived.ArchivedByCascadeID == "" {
		t.Fatal("auto-archive bypassed the cascade lifecycle coordinator")
	}
}
