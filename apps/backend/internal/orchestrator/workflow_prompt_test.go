package orchestrator

import (
	"context"
	"strings"
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestBuildWorkflowPrompt_ReplacesTaskPromptPlaceholder(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Implement this exactly:\n\n{{task_prompt}}",
	}

	got := svc.buildWorkflowPrompt("Migrate Atlantis datasource.", step, "task-1", "session-1")

	want := "Implement this exactly:\n\nMigrate Atlantis datasource."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_UsesStepPromptOnlyWithoutTaskPromptPlaceholder(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Commit the changes, push and create a draft PR.",
	}

	got := svc.buildWorkflowPrompt("Migrate Atlantis datasource.", step, "task-1", "session-1")

	want := "Commit the changes, push and create a draft PR."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestApplyWorkflowAndPlanMode_KeepsWorkflowPromptVisibleWhenStepEnablesPlanMode(t *testing.T) {
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Commit the changes, push and create a draft PR.",
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterEnablePlanMode}},
		},
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	got, planModeActive := svc.applyWorkflowAndPlanMode(
		context.Background(),
		"Migrate Atlantis datasource.",
		"task-1",
		"session-1",
		"step-1",
		false,
	)

	if !planModeActive {
		t.Fatal("expected plan mode to be active")
	}
	if !strings.Contains(got, "Commit the changes, push and create a draft PR.") {
		t.Fatalf("expected visible workflow prompt in effective prompt, got %q", got)
	}
	if strings.Contains(got, "Migrate Atlantis datasource.") {
		t.Fatalf("expected base prompt to be omitted when step prompt lacks {{task_prompt}}, got %q", got)
	}
	if strings.Contains(got, "<kandev-system>") {
		t.Fatalf("expected workflow prompt to remain visible without hidden system wrapping, got %q", got)
	}
}
