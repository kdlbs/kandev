package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

func TestUpdateTaskWithWorkflowStepAdmissionForDeferredMoveRejectsReplacementSession(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred", WorkspaceID: "workspace-deferred", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred", "step-source", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred", "step-target", 1)
	task := &models.Task{
		ID: "task-deferred", WorkspaceID: "workspace-deferred", WorkflowID: "workflow-deferred",
		WorkflowStepID: "step-source", Title: "Deferred candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{
		ID: "session-deferred", TaskID: task.ID, QueueIncarnationID: "incarnation-original",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	if err != nil {
		t.Fatal(err)
	}
	move := messagequeue.PendingMove{
		MoveID: "move-deferred", SessionIncarnationID: session.QueueIncarnationID,
		TaskID: task.ID, WorkflowID: task.WorkflowID, WorkflowStepID: "step-target",
		QueuedAt: time.Now().UTC(),
	}
	if err := queueRepo.SetPendingMove(ctx, session.ID, &move); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?`),
		"incarnation-replacement", session.ID,
	); err != nil {
		t.Fatal(err)
	}

	_, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionForDeferredMove(
		ctx, task, "step-source", "step-target", 0,
		messagequeue.PendingMoveRecord{SessionID: session.ID, Move: move},
	)
	if !errors.Is(err, messagequeue.ErrSessionIdentityMismatch) {
		t.Fatalf("expected session identity mismatch, got %v", err)
	}
	if applied {
		t.Fatal("replacement session must not apply the deferred move")
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WorkflowStepID != "step-source" {
		t.Fatalf("task moved to %q despite replacement session", stored.WorkflowStepID)
	}
	pending, err := queueRepo.GetPendingMove(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.MoveID != move.MoveID {
		t.Fatalf("pending move was not preserved: %+v", pending)
	}
}
