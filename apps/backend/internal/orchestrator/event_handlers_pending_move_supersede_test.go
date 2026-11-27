package orchestrator

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// pendingMoveFromStoredShape builds a pending move from the JSON shape that
// queue snapshots persist and restore.
func pendingMoveFromStoredShape(t *testing.T, fields map[string]interface{}) *messagequeue.PendingMove {
	t.Helper()
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("encode pending move: %v", err)
	}
	var move messagequeue.PendingMove
	if err := json.Unmarshal(encoded, &move); err != nil {
		t.Fatalf("decode pending move: %v", err)
	}
	return &move
}

// armSupersedablePendingMove replaces the scenario's pending move with one
// that records the step and ledger entry it was requested from, plus its
// correlated hand-off prompt, and returns the stored row as turn-end replay
// reads it.
func armSupersedablePendingMove(t *testing.T, sc *pendingMoveScenario, moveID string) *messagequeue.PendingMove {
	t.Helper()
	reviewSession, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}
	sourceTransitionID, err := sc.repo.GetLatestTaskStepTransitionID(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("read source transition: %v", err)
	}
	if _, ok := sc.svc.messageQueue.TakeQueued(sc.ctx, sc.reviewSessionID); !ok {
		t.Fatalf("remove scenario hand-off prompt")
	}
	if _, err := sc.svc.messageQueue.QueueMessageWithMetadata(
		sc.ctx, sc.reviewSessionID, "task-1", "hand-off for In Progress", "",
		messagequeue.QueuedByMoveTask, false, nil,
		map[string]interface{}{messagequeue.MetadataDeferredMoveID: moveID},
	); err != nil {
		t.Fatalf("queue hand-off prompt: %v", err)
	}
	move := pendingMoveFromStoredShape(t, map[string]interface{}{
		"move_id":                moveID,
		"session_incarnation_id": reviewSession.QueueIncarnationID,
		"task_id":                "task-1",
		"workflow_id":            "wf1",
		"workflow_step_id":       stepInProgressID,
		"from_step_id":           stepInReviewID,
		"from_transition_id":     sourceTransitionID,
	})
	if err := sc.svc.messageQueue.SetPendingMove(sc.ctx, sc.reviewSessionID, move); err != nil {
		t.Fatalf("set pending move: %v", err)
	}
	stored, exists, err := sc.svc.messageQueue.GetPendingMoveWithError(sc.ctx, sc.reviewSessionID)
	if err != nil || !exists {
		t.Fatalf("read pending move: exists=%t err=%v", exists, err)
	}
	return stored
}

func moveScenarioTaskTo(t *testing.T, sc *pendingMoveScenario, stepID string) {
	t.Helper()
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WorkflowStepID = stepID
	if err := sc.repo.UpdateTask(sc.ctx, task); err != nil {
		t.Fatalf("move task to %s: %v", stepID, err)
	}
}

func assertSupersededPendingMoveDropped(t *testing.T, sc *pendingMoveScenario, wantStepID string) {
	t.Helper()
	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != wantStepID {
		t.Fatalf("workflow_step_id = %q, want %q kept by the later move", task.WorkflowStepID, wantStepID)
	}
	pending, exists, err := sc.svc.messageQueue.GetPendingMoveWithError(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("read pending move: %v", err)
	}
	if exists {
		t.Fatalf("superseded pending move still armed: %+v", pending)
	}
	if status := sc.svc.messageQueue.GetStatus(sc.ctx, sc.reviewSessionID); len(status.Entries) != 0 {
		t.Fatalf("queue entries = %#v, want the superseded hand-off prompt removed", status.Entries)
	}
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-001.1, 001.4
func TestPendingMove_DropsMoveSupersededByLaterMove(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	move := armSupersedablePendingMove(t, sc, "move-superseded")
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	moveScenarioTaskTo(t, sc, stepReviewedID)
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)

	assertSupersededPendingMoveDropped(t, sc, stepReviewedID)
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-001.2, 001.4
func TestPendingMove_DropsMoveSupersededByLeaveAndReturn(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	move := armSupersedablePendingMove(t, sc, "move-left-and-returned")
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	moveScenarioTaskTo(t, sc, stepReviewedID)
	moveScenarioTaskTo(t, sc, stepInReviewID)
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)

	assertSupersededPendingMoveDropped(t, sc, stepInReviewID)
}

// AC-TASKS-WORKFLOW-STAGE-MOVES-001.5
func TestPendingMove_AppliesRecordedMoveWithoutLaterMove(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	move := armSupersedablePendingMove(t, sc, "move-not-superseded")
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, move)

	task, err := sc.repo.GetTask(sc.ctx, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.WorkflowStepID != stepInProgressID {
		t.Fatalf("workflow_step_id = %q, want the deferred target %q", task.WorkflowStepID, stepInProgressID)
	}
}
