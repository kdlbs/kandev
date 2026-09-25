package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTerminalRetentionPreventsArchiveCleanupUntilCleared(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	const stepID = "step-terminal-retention"
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, position, auto_archive_after_hours, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, stepID, workspace.OfficeWorkflowID, "Done", 0, 1, now, now); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"held-task", "ordinary-task"} {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: workspace.ID, WorkflowID: workspace.OfficeWorkflowID,
			WorkflowStepID: stepID, Title: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	held := true
	if _, err := svc.UpdateTask(ctx, "held-task", &UpdateTaskRequest{TerminalRetention: &held}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id IN (?, ?)`, now.Add(-48*time.Hour), "held-task", "ordinary-task"); err != nil {
		t.Fatal(err)
	}
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	svc.SetAutoArchiveCoordinator(handoff)
	svc.runAutoArchive(ctx)
	for id, wantArchived := range map[string]bool{"held-task": false, "ordinary-task": true} {
		task, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if (task.ArchivedAt != nil) != wantArchived {
			t.Fatalf("%s archived = %v, want %v", id, task.ArchivedAt != nil, wantArchived)
		}
	}
	if err := svc.ArchiveTask(ctx, "held-task"); !errors.Is(err, ErrTaskArchiveHeld) {
		t.Fatalf("direct archive error = %v, want retention hold", err)
	}
	if _, err := handoff.ArchiveTaskTree(ctx, "held-task", false); !errors.Is(err, ErrTaskArchiveHeld) {
		t.Fatalf("cascade archive error = %v, want retention hold", err)
	}
	held = false
	if _, err := svc.UpdateTask(ctx, "held-task", &UpdateTaskRequest{TerminalRetention: &held}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ArchiveTask(ctx, "held-task"); err != nil {
		t.Fatalf("archive after clearing hold: %v", err)
	}
}
