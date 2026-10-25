package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// AC-TASKS-WORKFLOW-STAGE-MOVES-001.3: a later move that leaves and returns
// to the source step between the replay check and the atomic write must make
// the deferred admission a source change, not an applied move.
func TestUpdateTaskWithWorkflowStepAdmissionForDeferredMoveRejectsLeaveAndReturn(t *testing.T) {
	verifyDeferredMoveRejectsLeaveAndReturn(t, newRepoForEntityTests(t))
}

func TestPostgresDeferredMoveRejectsLeaveAndReturn(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	verifyDeferredMoveRejectsLeaveAndReturn(t, repo)
}

func verifyDeferredMoveRejectsLeaveAndReturn(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-deferred-return")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-deferred-return", WorkspaceID: "workspace-deferred-return", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedCASWorkflowStep(t, repo, "workflow-deferred-return", "step-source-return", 0)
	seedCASWorkflowStep(t, repo, "workflow-deferred-return", "step-elsewhere-return", 1)
	seedCASWorkflowStep(t, repo, "workflow-deferred-return", "step-target-return", 2)
	task := &models.Task{
		ID: "task-deferred-return", WorkspaceID: "workspace-deferred-return", WorkflowID: "workflow-deferred-return",
		WorkflowStepID: "step-source-return", Title: "Deferred candidate", WIPAdmitted: true,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	session := &models.TaskSession{
		ID: "session-deferred-return", TaskID: task.ID, QueueIncarnationID: "incarnation-return",
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.ro)
	if err != nil {
		t.Fatal(err)
	}
	sourceTransitionID, err := repo.GetLatestTaskStepTransitionID(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Built from the persisted JSON shape that queue snapshots carry.
	encoded, err := json.Marshal(map[string]interface{}{
		"move_id": "move-deferred-return", "session_incarnation_id": session.QueueIncarnationID,
		"task_id": task.ID, "workflow_id": task.WorkflowID, "workflow_step_id": "step-target-return",
		"from_step_id": "step-source-return", "from_transition_id": sourceTransitionID,
		"queued_at": time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var move messagequeue.PendingMove
	if err := json.Unmarshal(encoded, &move); err != nil {
		t.Fatal(err)
	}
	if err := queueRepo.SetPendingMove(ctx, session.ID, &move); err != nil {
		t.Fatal(err)
	}

	for _, stepID := range []string{"step-elsewhere-return", "step-source-return"} {
		current, err := repo.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatal(err)
		}
		current.WorkflowStepID = stepID
		if err := repo.UpdateTask(ctx, current); err != nil {
			t.Fatal(err)
		}
	}

	candidate, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionForDeferredMove(
		ctx, candidate, "step-source-return", "step-target-return", 0,
		messagequeue.PendingMoveRecord{SessionID: session.ID, Move: move},
	)
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Fatal("a deferred move superseded by a leave-and-return must not apply")
	}
	stored, err := repo.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.WorkflowStepID != "step-source-return" {
		t.Fatalf("task moved to %q despite the later move", stored.WorkflowStepID)
	}
	pending, err := queueRepo.GetPendingMove(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.MoveID != move.MoveID {
		t.Fatalf("rejected admission must leave the pending row for the caller to drop, got %+v", pending)
	}
}
