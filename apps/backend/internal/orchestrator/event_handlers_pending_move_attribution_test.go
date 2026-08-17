package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/steptelemetry"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestApplyPendingMove_UsesSenderSessionForLedgerAttribution(t *testing.T) {
	sc := buildPendingMoveScenario(t)
	session, err := sc.repo.GetTaskSession(sc.ctx, sc.reviewSessionID)
	if err != nil {
		t.Fatalf("load review session: %v", err)
	}

	const senderSessionID = "session-caller"
	now := time.Now().UTC()
	if err := sc.repo.CreateTask(context.Background(), &models.Task{
		ID: "task-caller", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: stepInProgressID,
		Title: "Caller", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create caller task: %v", err)
	}
	if err := sc.repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: senderSessionID, TaskID: "task-caller", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create caller session: %v", err)
	}
	sc.svc.applyPendingMove(sc.ctx, "task-1", sc.reviewSessionID, session, &messagequeue.PendingMove{
		TaskID:          "task-1",
		WorkflowID:      "wf1",
		WorkflowStepID:  stepInProgressID,
		SenderSessionID: senderSessionID,
	})

	rows := stepTransitionRowsForTaskOrchestrator(t, sc.repo, "task-1")
	if len(rows) == 0 {
		t.Fatal("expected a ledger row for the deferred move")
	}
	last := rows[len(rows)-1]
	if last.trigger != string(steptelemetry.TriggerMCPDeferredMove) {
		t.Fatalf("trigger = %q, want %q", last.trigger, steptelemetry.TriggerMCPDeferredMove)
	}
	if last.actorKind != string(steptelemetry.ActorAgent) {
		t.Fatalf("actor_kind = %q, want %q", last.actorKind, steptelemetry.ActorAgent)
	}
	if last.actorID == nil || *last.actorID != senderSessionID {
		t.Fatalf("actor_id = %v, want %q", last.actorID, senderSessionID)
	}
	if last.sessionID == nil || *last.sessionID != senderSessionID {
		t.Fatalf("session_id = %v, want %q", last.sessionID, senderSessionID)
	}
}
