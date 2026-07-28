package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/review"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type fakeReviewRunner struct {
	requests []review.RunRequest
	err      error
}

func (f *fakeReviewRunner) Launch(_ context.Context, req review.RunRequest) (*taskmodels.TaskReviewRun, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return nil, f.err
	}
	return &taskmodels.TaskReviewRun{ID: "run-1", TaskID: req.TaskID, Status: taskmodels.ReviewRunPending}, nil
}

func reviewCallbackInput(profileID string) engine.ActionInput {
	in := engine.ActionInput{
		Trigger: engine.TriggerOnEnter,
		State:   engine.MachineState{TaskID: "t1", SessionID: "s1"},
		Step:    engine.StepSpec{ID: "step-review", Name: "Review"},
		Action:  engine.Action{Kind: engine.ActionRunCodeReview},
	}
	if profileID != "" {
		in.Action.RunCodeReview = &engine.RunCodeReviewAction{AgentProfileID: profileID}
	}
	return in
}

func TestRunCodeReviewCallback_LaunchesWithStepContext(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step-review")
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	runner := &fakeReviewRunner{}
	svc.reviewRunner = runner

	if _, err := (&runCodeReviewCallback{svc: svc}).Execute(context.Background(), reviewCallbackInput("profile-9")); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("expected one launch, got %d", len(runner.requests))
	}
	got := runner.requests[0]
	if got.TaskID != "t1" || got.SessionID != "s1" {
		t.Fatalf("expected the triggering task/session, got %+v", got)
	}
	if got.Trigger != taskmodels.ReviewTriggerWorkflowStep {
		t.Fatalf("expected the workflow_step trigger, got %q", got.Trigger)
	}
	if got.WorkflowStepID != "step-review" {
		t.Fatalf("expected the step id recorded, got %q", got.WorkflowStepID)
	}
	if got.AgentProfileID != "profile-9" {
		t.Fatalf("expected the step's reviewing profile, got %q", got.AgentProfileID)
	}
}

func TestRunCodeReviewCallback_NoProfileUsesUtilityAgentDefault(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step-review")
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	runner := &fakeReviewRunner{}
	svc.reviewRunner = runner

	if _, err := (&runCodeReviewCallback{svc: svc}).Execute(context.Background(), reviewCallbackInput("")); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if runner.requests[0].AgentProfileID != "" {
		t.Fatalf("expected an empty profile so the code-review utility agent is used, got %q",
			runner.requests[0].AgentProfileID)
	}
}

func TestRunCodeReviewCallback_FailureDoesNotBlockStepEntry(t *testing.T) {
	// A task must never get stuck in a step because no reviewer was configured
	// or the provider was down; the run row carries the reason instead.
	for name, cause := range map[string]error{
		"no capable agent":  review.ErrAgentUnavailable,
		"nothing to review": review.ErrNoChanges,
		"workspace down":    review.ErrWorkspaceUnavailable,
		"unexpected":        errors.New("boom"),
	} {
		t.Run(name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step-review")
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
			svc.reviewRunner = &fakeReviewRunner{err: cause}

			result, err := (&runCodeReviewCallback{svc: svc}).Execute(context.Background(), reviewCallbackInput(""))
			if err != nil {
				t.Fatalf("a review failure must not fail step entry, got %v", err)
			}
			if result.DataPatch != nil {
				t.Fatalf("expected no workflow data patch, got %+v", result.DataPatch)
			}
		})
	}
}

func TestBuildWorkflowCallbacks_RegistersRunCodeReviewOnlyWhenWired(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})

	if _, found := buildWorkflowCallbacks(svc).Get(engine.ActionRunCodeReview); found {
		t.Fatal("run_code_review must not be registered without a review runner")
	}

	svc.reviewRunner = &fakeReviewRunner{}
	if _, found := buildWorkflowCallbacks(svc).Get(engine.ActionRunCodeReview); !found {
		t.Fatal("run_code_review must be registered once the review runner is wired")
	}
}
