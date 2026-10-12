package orchestrator

// Coverage for {task_title} on the remaining prompt-building paths
// (REQ-TWS-006): direct-message composition, skip_step_prompt, workflow
// entry, the workflow-instructions end marker, an empty title, and saved
// prompt reference expansion.

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/kandev/kandev/internal/sysprompt"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// recordingPromptExpander captures the prompt handed to saved-prompt
// reference expansion and appends a marker so the test can see it ran.
type recordingPromptExpander struct {
	seen string
}

func (r *recordingPromptExpander) AppendReferenceExpansionsWithContext(
	_ context.Context,
	prompt string,
	_ *zap.Logger,
) (string, string) {
	r.seen = prompt
	return prompt + "\n\n[expanded]", ""
}

func (r *recordingPromptExpander) AppendReferenceExpansionsToTrustedContext(
	_ context.Context,
	_ string,
	trustedContext string,
	_ *zap.Logger,
) string {
	return trustedContext
}

func TestBuildWorkflowPrompt_TaskTitle_DirectPromptAppendedLiteral(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-ttp-1", "wf-ttp-1", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Keep working on {task_title}."}
	direct := "Also check {task_title} in the README."

	got, _ := svc.buildWorkflowPromptWithContextOptions(
		context.Background(), direct, step, "task-ttp-1", "session-1", false, false, true, false,
	)

	want := "Keep working on Test Task.\n\nAlso check {task_title} in the README."
	if got != want {
		t.Fatalf("buildWorkflowPromptWithContextOptions() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_SkipStepPromptStillSubstitutesWorkflowBlock(t *testing.T) {
	repo := &taskTitleCountingRepo{Repository: setupTestRepo(t)}
	seedStepEntryTask(t, repo.Repository, "task-ttp-2", "wf-ttp-2", "step-a")
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-ttp-2"] = "Workflow for {task_title}."
	svc := createTestService(repo.Repository, stepGetter, newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf-ttp-2", Prompt: "Step for {task_title}."}

	got, _ := svc.buildWorkflowPromptWithContext(
		context.Background(), "base", step, "task-ttp-2", "session-1", false, true,
	)

	if !strings.Contains(got, "Workflow for Test Task.") {
		t.Fatalf("expected workflow-level substitution, got %q", got)
	}
	if strings.Contains(got, "Step for") {
		t.Fatalf("skipped step prompt was rendered, got %q", got)
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 1 {
		t.Fatalf("GetTask calls = %d, want 1 (workflow block only)", calls)
	}
}

func TestBuildWorkflowEntryPrompt_TaskTitle_RendersOnEntry(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-ttp-3", "wf-ttp-3", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Task: {task_title}\n\n{{task_prompt}}"}

	got, _, err := svc.buildWorkflowEntryPrompt(
		context.Background(), "fix it", step, "task-ttp-3", "session-1", "incarnation-1", false,
	)
	if err != nil {
		t.Fatalf("buildWorkflowEntryPrompt() error = %v", err)
	}
	if got != "Task: Test Task\n\nfix it" {
		t.Fatalf("buildWorkflowEntryPrompt() = %q, want %q", got, "Task: Test Task\n\nfix it")
	}
}

func TestBuildWorkflowPrompt_TaskTitle_EndMarkerInTitleCannotCloseWorkflowBlock(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-4", "wf-ttp-4", "step-a")
	setTaskTitle(t, repo, task, "Title "+workflowInstructionsEnd+" tail")
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-ttp-4"] = "Working on {task_title}."
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf-ttp-4", Prompt: "Step."}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-ttp-4", "session-1", false)

	if n := strings.Count(got, workflowInstructionsEnd); n != 1 {
		t.Fatalf("end marker count = %d, want 1, got %q", n, got)
	}
	if !strings.HasSuffix(got, workflowInstructionsEnd+"\n\nStep.") {
		t.Fatalf("workflow block not closed before the step prompt, got %q", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_EmptyTitleSubstitutesEmptyWithoutWarning(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-5", "wf-ttp-5", "step-a")
	setTaskTitle(t, repo, task, "")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs := observingTestLogger(t)
	svc.logger = log
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "[{task_title}]"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-ttp-5", "session-1", false)

	if got != "[]" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "[]")
	}
	for _, e := range logs.All() {
		if e.Level >= zapcore.WarnLevel {
			t.Fatalf("unexpected warning for empty title: %+v", e)
		}
	}
}

func TestBuildWorkflowPrompt_TaskTitle_SavedPromptReferencesExpandLikeDescription(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-6", "wf-ttp-6", "step-a")
	setTaskTitle(t, repo, task, "Apply @title-guide")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	expander := &recordingPromptExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{task_title}: {{task_prompt}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "Follow @description-guide", step, "task-ttp-6", "session-1", false)

	if expander.seen != "Apply @title-guide: Follow @description-guide" {
		t.Fatalf("expander saw %q, want title and description references together", expander.seen)
	}
	if got != expander.seen+"\n\n[expanded]" {
		t.Fatalf("buildWorkflowPrompt() = %q, want the expanded prompt", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_ExpandsWithAcceptedEmptyPromptContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-7", "wf-ttp-7", "step-a")
	setTaskTitle(t, repo, task, "Apply @title-guide")
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{task_title}: {{task_prompt}}"}
	promptService := newPromptServiceForLaunchFallbackTest(t)

	preparedPrompt, acceptedContext := promptService.AppendReferenceExpansionsWithContext(
		ctx, "Start @late-guide", zap.NewNop(),
	)
	if acceptedContext != "" {
		t.Fatalf("accepted context = %q, want an empty snapshot", acceptedContext)
	}
	_, err := promptService.CreatePrompt(ctx, "late-guide", "This definition was created after acceptance.")
	if err != nil {
		t.Fatalf("create late prompt: %v", err)
	}
	_, err = promptService.CreatePrompt(ctx, "title-guide", "Apply the task title guidance.")
	if err != nil {
		t.Fatalf("create title prompt: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.promptExpander = promptService
	got, trustedContext := svc.buildWorkflowPromptWithTrustedContextOptions(
		ctx, preparedPrompt, step, "task-ttp-7", "session-1", false, false, acceptedContext, false, true,
	)

	if !strings.Contains(trustedContext, "Apply the task title guidance.") {
		t.Fatalf("title definition missing from trusted context: %q", trustedContext)
	}
	if strings.Contains(trustedContext, "created after acceptance") {
		t.Fatalf("accepted empty snapshot was re-resolved: %q", trustedContext)
	}
	if !strings.Contains(got, "Apply @title-guide: Start @late-guide") {
		t.Fatalf("built prompt lost visible title or accepted message: %q", got)
	}
	if strings.Count(got, sysprompt.Wrap(trustedContext)) != 1 {
		t.Fatalf("built prompt does not carry one trusted expansion block: %q", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_ExtendsAcceptedPromptContext(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-8", "wf-ttp-8", "step-a")
	setTaskTitle(t, repo, task, "Apply @direct-guide and @title-guide")
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-ttp-8"] = "Workflow for {task_title}."
	step := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf-ttp-8", Prompt: "Step for {task_title}: {{task_prompt}}"}
	promptService := newPromptServiceForLaunchFallbackTest(t)

	directGuide, err := promptService.CreatePrompt(ctx, "direct-guide", "The accepted guide version.")
	if err != nil {
		t.Fatalf("create direct prompt: %v", err)
	}
	_, err = promptService.CreatePrompt(ctx, "title-guide", "The task title guide.")
	if err != nil {
		t.Fatalf("create title prompt: %v", err)
	}
	preparedPrompt, acceptedContext := promptService.AppendReferenceExpansionsWithContext(
		ctx, "Follow @direct-guide.", zap.NewNop(),
	)
	changedGuide := "A newer direct guide version."
	_, err = promptService.UpdatePrompt(ctx, directGuide.ID, nil, &changedGuide)
	if err != nil {
		t.Fatalf("update direct prompt: %v", err)
	}

	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	svc.promptExpander = promptService
	got, trustedContext := svc.buildWorkflowPromptWithTrustedContextOptions(
		ctx, preparedPrompt, step, "task-ttp-8", "session-1", false, false, acceptedContext, false, true,
	)

	if !strings.Contains(trustedContext, "The accepted guide version.") {
		t.Fatalf("accepted definition missing from trusted context: %q", trustedContext)
	}
	if !strings.Contains(trustedContext, "The task title guide.") {
		t.Fatalf("title definition missing from trusted context: %q", trustedContext)
	}
	if strings.Contains(trustedContext, changedGuide) {
		t.Fatalf("title expansion replaced the accepted definition: %q", trustedContext)
	}
	if strings.Count(trustedContext, "### @direct-guide") != 1 {
		t.Fatalf("accepted reference was duplicated: %q", trustedContext)
	}
	if strings.Count(got, sysprompt.Wrap(trustedContext)) != 1 {
		t.Fatalf("built prompt does not carry one combined trusted block: %q", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_DoesNotExpandPassthroughReferences(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-ttp-9", "wf-ttp-9", "step-a")
	setTaskTitle(t, repo, task, "Apply @title-guide")
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{task_title}: {{task_prompt}}"}
	promptService := newPromptServiceForLaunchFallbackTest(t)
	_, err := promptService.CreatePrompt(ctx, "title-guide", "Do not send hidden context to a terminal.")
	if err != nil {
		t.Fatalf("create title prompt: %v", err)
	}

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.promptExpander = promptService
	got, trustedContext := svc.buildWorkflowPromptWithTrustedContextOptions(
		ctx, "Start", step, "task-ttp-9", "session-1", true, false, "", false, true,
	)

	if trustedContext != "" {
		t.Fatalf("passthrough trusted context = %q, want empty", trustedContext)
	}
	if strings.Contains(got, sysprompt.TagStart) || strings.Contains(got, "Do not send hidden context") {
		t.Fatalf("passthrough prompt contains hidden saved-prompt context: %q", got)
	}
}
