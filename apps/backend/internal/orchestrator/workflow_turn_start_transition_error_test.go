package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-TASKS-RESUME-PROMPT-QUEUE-001.10
func TestLegacyTurnStartTransitionFailureSurfacesForStartingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateStarting, "resume attempt active"); err != nil {
		t.Fatalf("mark session starting: %v", err)
	}

	transitionErr := errors.New("injected destination-step lookup failure")
	currentStep := &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Implement", Position: 0,
		Events: wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{
			Type: wfmodels.OnTurnStartMoveToNext,
		}}},
	}
	steps := newMockStepGetter()
	steps.steps[currentStep.ID] = currentStep
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Review", Position: 1,
	}
	steps.getStepFunc = func(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
		if stepID == currentStep.ID {
			return currentStep, nil
		}
		return nil, transitionErr
	}

	svc := createTestService(repo, steps, newMockTaskRepo())
	svc.workflowEngine = nil
	if _, err := svc.processOnTurnStartAdmission(ctx, "t1", "s1", true); !errors.Is(err, transitionErr) {
		t.Fatalf("strict on_turn_start error = %v, want injected transition error", err)
	}

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if session.State != models.TaskSessionStateStarting {
		t.Fatalf("session state = %s, want STARTING", session.State)
	}
	if session.ErrorMessage != "resume attempt active" {
		t.Fatalf("session error = %q, want existing startup error metadata", session.ErrorMessage)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != currentStep.ID {
		t.Fatalf("workflow step = %q, want unchanged source step %q", task.WorkflowStepID, currentStep.ID)
	}
}
