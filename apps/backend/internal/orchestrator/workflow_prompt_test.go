package orchestrator

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakePromptReferenceExpander is a test double for PromptReferenceExpander.
// It appends a deterministic marker for any "@name" reference found in the
// prompt, mirroring the "resolves then appends a hidden block" contract of
// promptservice.Service.AppendReferenceExpansions without depending on the
// real prompt-resolution machinery.
type fakePromptReferenceExpander struct {
	// calls records every prompt passed to AppendReferenceExpansions, so
	// tests can assert what the expander actually saw (e.g. that template
	// interpolation already ran).
	calls []string
}

func (f *fakePromptReferenceExpander) AppendReferenceExpansions(_ context.Context, prompt string, _ *zap.Logger) string {
	f.calls = append(f.calls, prompt)
	if !strings.Contains(prompt, "@") {
		return prompt
	}
	return prompt + "\n\n<kandev-system>EXPANDED:" + prompt + "</kandev-system>"
}

func TestBuildWorkflowPrompt_ReplacesTaskPromptPlaceholder(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Implement this exactly:\n\n{{task_prompt}}",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

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

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	want := "Commit the changes, push and create a draft PR."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_NoExpanderLeavesReferenceUnexpanded(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Use @my-prompt for context.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	want := "Use @my-prompt for context."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() with no expander set = %q, want unchanged %q", got, want)
	}
}

func TestBuildWorkflowPrompt_ExpandsStepPromptReference(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Use @my-prompt for context.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	if !strings.Contains(got, "Use @my-prompt for context.") {
		t.Fatalf("expected original prompt text preserved, got %q", got)
	}
	if !strings.Contains(got, "<kandev-system>EXPANDED:Use @my-prompt for context.</kandev-system>") {
		t.Fatalf("expected hidden expansion block appended, got %q", got)
	}
}

func TestBuildWorkflowPrompt_PassthroughSkipsReferenceExpansion(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Use @my-prompt for context.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", true)

	want := "Use @my-prompt for context."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() for passthrough session = %q, want unchanged %q", got, want)
	}
	if len(expander.calls) != 0 {
		t.Fatalf("expected expander not to be invoked for a passthrough session, got %d calls", len(expander.calls))
	}
}

func TestBuildWorkflowPrompt_ExpandsBasePromptReference(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Implement this exactly:\n\n{{task_prompt}}",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate @my-prompt datasource.", step, "task-1", "session-1", false)

	want := "Implement this exactly:\n\nMigrate @my-prompt datasource."
	if !strings.Contains(got, want) {
		t.Fatalf("expected base prompt reference preserved in joined prompt, got %q", got)
	}
	if !strings.Contains(got, "<kandev-system>EXPANDED:"+want+"</kandev-system>") {
		t.Fatalf("expected base-prompt reference to be resolved by the expander, got %q", got)
	}
}

func TestBuildWorkflowPrompt_InterpolatesPlaceholdersBeforeExpansion(t *testing.T) {
	svc := createTestService(setupTestRepo(t), newMockStepGetter(), newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Task {task_id}: {{task_prompt}} @my-prompt",
	}

	svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	if len(expander.calls) != 1 {
		t.Fatalf("expected expander to be invoked exactly once, got %d calls", len(expander.calls))
	}
	seen := expander.calls[0]
	if strings.Contains(seen, "{{task_prompt}}") || strings.Contains(seen, "{task_id}") {
		t.Fatalf("expected placeholders to be interpolated before the expander runs, got %q", seen)
	}
	if !strings.Contains(seen, "Task task-1: Migrate Atlantis datasource. @my-prompt") {
		t.Fatalf("expected fully-assembled prompt passed to expander, got %q", seen)
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
		false, // isEphemeral
		false, // isPassthrough
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

func TestApplyWorkflowAndPlanMode_PassthroughSkipsReferenceExpansion(t *testing.T) {
	repo := setupTestRepo(t)
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-1"] = &wfmodels.WorkflowStep{
		ID:     "step-1",
		Prompt: "Use @my-prompt for context.",
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander

	got, _ := svc.applyWorkflowAndPlanMode(
		context.Background(),
		"Migrate Atlantis datasource.",
		"task-1",
		"session-1",
		"step-1",
		false,
		false, // isEphemeral
		true,  // isPassthrough
	)

	want := "Use @my-prompt for context."
	if got != want {
		t.Fatalf("applyWorkflowAndPlanMode() for passthrough session = %q, want unchanged %q", got, want)
	}
	if len(expander.calls) != 0 {
		t.Fatalf("expected expander not to be invoked for a passthrough session, got %d calls", len(expander.calls))
	}
}
