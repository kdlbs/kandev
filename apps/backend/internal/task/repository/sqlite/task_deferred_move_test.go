package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
)

func TestUpdateTaskWithWorkflowStepAdmissionForDeferredMovePersistsQueuedEntryOptionsAtomically(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-options")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-options", WorkspaceID: "workspace-deferred-options", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-options", "step-source-options", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-options", "step-target-options", 1)

	occupant := &models.Task{
		ID: "task-deferred-options-occupant", WorkspaceID: "workspace-deferred-options", WorkflowID: "workflow-deferred-options",
		WorkflowStepID: "step-target-options", Title: "Occupant", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, occupant); err != nil {
		t.Fatal(err)
	}
	candidate := &models.Task{
		ID: "task-deferred-options", WorkspaceID: "workspace-deferred-options", WorkflowID: "workflow-deferred-options",
		WorkflowStepID: "step-source-options", Title: "Deferred candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, candidate); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{
		ID: "session-deferred-options", TaskID: candidate.ID, QueueIncarnationID: "incarnation-options",
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
		MoveID: "move-deferred-options", SessionIncarnationID: session.QueueIncarnationID,
		TaskID: candidate.ID, WorkflowID: candidate.WorkflowID, WorkflowStepID: "step-target-options",
		QueuedAt: time.Now().UTC(), EntryOptions: &workflowmove.EntryOptions{
			ResetContext: true, Instructions: "focus on the auth bug", SkipStepPrompt: true,
		},
	}
	if err := queueRepo.SetPendingMove(ctx, session.ID, &move); err != nil {
		t.Fatal(err)
	}

	admitted, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionForDeferredMove(
		ctx, candidate, "step-source-options", "step-target-options", 1,
		messagequeue.PendingMoveRecord{SessionID: session.ID, Move: move},
	)
	if err != nil {
		t.Fatal(err)
	}
	if admitted || !applied {
		t.Fatalf("admitted=%t applied=%t, want queued applied move", admitted, applied)
	}

	stored, err := repo.GetTask(ctx, candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WorkflowStepID != "step-target-options" || stored.QueuedForStepID != "step-target-options" || stored.WIPAdmitted {
		t.Fatalf("candidate placement = %+v, want queued at target", stored)
	}
	marker, ok := stored.Metadata[models.MetaKeyWorkflowMovePending].(map[string]interface{})
	if !ok {
		t.Fatalf("queued deferred move must retain a workflow move marker, metadata=%+v", stored.Metadata)
	}
	if marker["from_step_id"] != "step-source-options" || marker["move_id"] != move.MoveID {
		t.Fatalf("marker identity = %+v, want source and move ID", marker)
	}
	encoded, ok := marker["options"].(string)
	if !ok || encoded == "" {
		t.Fatalf("marker options = %v, want encoded entry options", marker["options"])
	}
	decoded, err := workflowmove.DecodeEntryOptionsJSON([]byte(encoded))
	if err != nil {
		t.Fatal(err)
	}
	if *decoded != *move.EntryOptions {
		t.Fatalf("decoded marker options = %+v, want %+v", decoded, move.EntryOptions)
	}
	if pending, err := queueRepo.GetPendingMove(ctx, session.ID); err != nil {
		t.Fatal(err)
	} else if pending != nil {
		t.Fatalf("deferred move row must be consumed atomically, got %+v", pending)
	}
}

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
