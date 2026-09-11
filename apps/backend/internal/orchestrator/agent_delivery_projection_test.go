package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestReplayedTerminalIntentAdvancesOnce covers
// AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2: replaying one terminal turn
// intent must not evaluate the destination step as a second turn.
func TestReplayedTerminalIntentAdvancesOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-replay", "session-delivery-replay", "step1")
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step2"}},
		}},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{
			{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "step3"}},
		}},
	}
	stepGetter.steps["step3"] = &wfmodels.WorkflowStep{
		ID: "step3", WorkflowID: "wf1", Name: "Step 3", Position: 2,
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	effect := workflowEffectForTurn("turn-delivery-replay")
	effectCtx := withWorkflowEffect(ctx, effect)

	session, err := repo.GetTaskSession(ctx, "session-delivery-replay")
	if err != nil {
		t.Fatalf("GetTaskSession before first intent: %v", err)
	}
	if !svc.processOnTurnCompleteViaEngine(effectCtx, "task-delivery-replay", session) {
		t.Fatal("first terminal intent did not advance the workflow")
	}

	session, err = repo.GetTaskSession(ctx, "session-delivery-replay")
	if err != nil {
		t.Fatalf("GetTaskSession before replay: %v", err)
	}
	if svc.processOnTurnCompleteViaEngine(effectCtx, "task-delivery-replay", session) {
		t.Fatal("replayed terminal intent advanced the workflow a second time")
	}

	task, err := repo.GetTask(ctx, "task-delivery-replay")
	if err != nil {
		t.Fatalf("GetTask after replay: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("workflow step after replay = %q, want step2", task.WorkflowStepID)
	}
	stored, err := repo.GetAgentDeliveryEffect(ctx, effect.EffectKey)
	if err != nil {
		t.Fatalf("GetAgentDeliveryEffect: %v", err)
	}
	if stored.State != models.DeliveryEffectCompleted {
		t.Fatalf("effect state = %q, want %q", stored.State, models.DeliveryEffectCompleted)
	}
}
