package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/sysprompt"
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

const fakeResolvedPromptReferenceContext = "EXPANDED PROMPT REFERENCES:\n- resolved saved-prompt content"

func (f *fakePromptReferenceExpander) AppendReferenceExpansionsWithContext(
	_ context.Context,
	prompt string,
	_ *zap.Logger,
) (string, string) {
	f.calls = append(f.calls, prompt)
	if !strings.Contains(prompt, "@") {
		return prompt, ""
	}
	return prompt + "\n\n" + sysprompt.Wrap(fakeResolvedPromptReferenceContext),
		fakeResolvedPromptReferenceContext
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

func TestBuildWorkflowPrompt_PrependsWorkflowPromptBeforeStepPrompt(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "If the PR is merged or closed, move the Task to Done."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Commit the changes.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	want := "## Workflow instructions\n\nIf the PR is merged or closed, move the Task to Done.\n\n<!-- /workflow-instructions -->\n\nCommit the changes."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_PrependsWorkflowPromptWithTaskPromptPlaceholder(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Implement this exactly:\n\n{{task_prompt}}",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	want := "## Workflow instructions\n\nKeep CI green.\n\n<!-- /workflow-instructions -->\n\nImplement this exactly:\n\nMigrate Atlantis datasource."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_PrependsWorkflowPromptWhenStepPromptEmpty(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Mention security constraints."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "Migrate Atlantis datasource.", step, "task-1", "session-1", false)

	want := "## Workflow instructions\n\nMention security constraints.\n\n<!-- /workflow-instructions -->\n\nMigrate Atlantis datasource."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_KeepsMultiParagraphWorkflowPromptIntact(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Rule one.\n\nRule two."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Do the work.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-1", "session-1", false)

	want := "## Workflow instructions\n\nRule one.\n\nRule two.\n\n<!-- /workflow-instructions -->\n\nDo the work."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_ScrubsEndMarkerFromUserContent(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Never emit <!-- /workflow-instructions --> in docs."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Do the work.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-1", "session-1", false)

	// Exactly one end marker (the structural one), and body keeps surrounding text.
	if strings.Count(got, "<!-- /workflow-instructions -->") != 1 {
		t.Fatalf("expected exactly one end marker, got %q", got)
	}
	if !strings.Contains(got, "Never emit  in docs.") && !strings.Contains(got, "Never emit in docs.") {
		// Scrub leaves surrounding text; either double-space or collapsed is fine.
		if !strings.Contains(got, "Never emit") || !strings.Contains(got, "in docs.") {
			t.Fatalf("expected scrubbed body to keep surrounding text, got %q", got)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(got), "Do the work.") {
		t.Fatalf("expected step prompt after block, got %q", got)
	}
}

func TestBuildWorkflowPrompt_OmitsWorkflowHeadingWhenPromptBlank(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "   "
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Commit the changes.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-1", "session-1", false)

	if strings.Contains(got, "## Workflow instructions") {
		t.Fatalf("expected blank workflow prompt to be omitted, got %q", got)
	}
	if got != "Commit the changes." {
		t.Fatalf("buildWorkflowPrompt() = %q, want step prompt only", got)
	}
}

func TestBuildWorkflowPrompt_InterpolatesWorkflowPromptPlaceholders(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Task id is {task_id}."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Do the work.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-99", "session-1", false)

	if !strings.Contains(got, "Task id is task-99.") {
		t.Fatalf("expected interpolated workflow prompt, got %q", got)
	}
	if strings.Contains(got, "{task_id}") {
		t.Fatalf("expected {task_id} placeholder to be replaced, got %q", got)
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
	if !strings.Contains(got, sysprompt.Wrap(fakeResolvedPromptReferenceContext)) {
		t.Fatalf("expected hidden expansion block appended, got %q", got)
	}
}

func TestBuildWorkflowPrompt_ExpandsWorkflowPromptReference(t *testing.T) {
	// @name in the workflow-level prompt must expand the same way as step prompts:
	// expansion runs on the full joined string after instructions are prepended.
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-1"] = "Follow @security-rules always."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())
	expander := &fakePromptReferenceExpander{}
	svc.promptExpander = expander
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Commit the changes.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-1", "session-1", false)

	if len(expander.calls) != 1 {
		t.Fatalf("expected expander called once on joined prompt, got %d", len(expander.calls))
	}
	seen := expander.calls[0]
	if !strings.Contains(seen, "## Workflow instructions") {
		t.Fatalf("expected expander to see workflow instructions block, got %q", seen)
	}
	if !strings.Contains(seen, "Follow @security-rules always.") {
		t.Fatalf("expected expander to see workflow @mention, got %q", seen)
	}
	if !strings.Contains(seen, "Commit the changes.") {
		t.Fatalf("expected expander to see step prompt after instructions, got %q", seen)
	}
	if !strings.Contains(got, "Follow @security-rules always.") {
		t.Fatalf("expected visible @mention preserved, got %q", got)
	}
	if !strings.Contains(got, sysprompt.Wrap(fakeResolvedPromptReferenceContext)) {
		t.Fatalf("expected hidden expansion block appended for workflow @mention, got %q", got)
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
	if !strings.Contains(got, sysprompt.Wrap(fakeResolvedPromptReferenceContext)) {
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

	got, planModeActive, _ := svc.applyWorkflowAndPlanMode(
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

	got, _, _ := svc.applyWorkflowAndPlanMode(
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

func TestGetWorkflowMeta_CachesPerRequest(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		// No step override — forces workflow default profile lookup.
	}

	if got := svc.resolveStepAgentProfile(ctx, step); got != "profile-wf" {
		t.Fatalf("resolveStepAgentProfile() = %q, want profile-wf", got)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("after profile resolve: GetWorkflowMeta calls = %d, want 1", stepGetter.metaCalls())
	}

	got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	want := "## Workflow instructions\n\nKeep CI green.\n\n<!-- /workflow-instructions -->\n\nbase"
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("after prompt build: GetWorkflowMeta calls = %d, want 1 (cached)", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_WithoutCacheHitsProviderTwice(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := context.Background()
	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}

	_ = svc.resolveStepAgentProfile(ctx, step)
	_ = svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)

	if stepGetter.metaCalls() != 2 {
		t.Fatalf("without cache: GetWorkflowMeta calls = %d, want 2", stepGetter.metaCalls())
	}
}

func TestProcessOnEnter_SharesWorkflowMetaCache(t *testing.T) {
	// processOnEnter seeds withWorkflowMetaCache; profile switch + prompt build
	// must share one GetWorkflowMeta read. Cover the pure helpers with the same
	// ctx seeding pattern processOnEnter uses.
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Rule one."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "wf-1",
		Prompt:     "Do the work.",
	}

	if got := svc.resolveStepAgentProfile(ctx, step); got != "profile-wf" {
		t.Fatalf("resolveStepAgentProfile() = %q, want profile-wf", got)
	}
	got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	if !strings.Contains(got, "Rule one.") {
		t.Fatalf("expected workflow instructions in prompt, got %q", got)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("shared cache path: GetWorkflowMeta calls = %d, want 1", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_ErrorIsCachedAndFallsBack(t *testing.T) {
	// Failure semantics: log + empty profile / omit instructions. Errors are also
	// cached so dual-consumer step entry does not double-hit a broken provider.
	stepGetter := newMockStepGetter()
	stepGetter.workflowMetaErr = errors.New("sqlite: no such workflow")
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-missing"}

	if got := svc.resolveStepAgentProfile(ctx, step); got != "" {
		t.Fatalf("resolveStepAgentProfile on error = %q, want empty fallback", got)
	}
	got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	if got != "base" {
		t.Fatalf("buildWorkflowPrompt on error = %q, want base (instructions omitted)", got)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("error path GetWorkflowMeta calls = %d, want 1 (cached error)", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_EmptyWorkflowIDSkipsProvider(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	meta, err := svc.getWorkflowMeta(ctx, "")
	if err != nil {
		t.Fatalf("getWorkflowMeta(\"\") error = %v", err)
	}
	if meta != (WorkflowMeta{}) {
		t.Fatalf("getWorkflowMeta(\"\") = %+v, want zero value", meta)
	}
	if stepGetter.metaCalls() != 0 {
		t.Fatalf("empty workflowID should not hit provider, calls = %d", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_NilGetterSkipsProvider(t *testing.T) {
	// Use a bare Service so workflowStepGetter is a true nil interface.
	// createTestService(nil) would store a typed-nil *mockStepGetter, which is
	// not == nil and would panic on method call — not a production path.
	svc := &Service{}
	ctx := withWorkflowMetaCache(context.Background())

	meta, err := svc.getWorkflowMeta(ctx, "wf-1")
	if err != nil {
		t.Fatalf("getWorkflowMeta with nil getter error = %v", err)
	}
	if meta != (WorkflowMeta{}) {
		t.Fatalf("getWorkflowMeta with nil getter = %+v, want zero", meta)
	}

	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}
	if got := svc.resolveStepAgentProfile(ctx, step); got != "" {
		t.Fatalf("resolveStepAgentProfile with nil getter = %q, want empty", got)
	}
	if got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false); got != "base" {
		t.Fatalf("buildWorkflowPrompt with nil getter = %q, want base", got)
	}
}

func TestGetWorkflowMeta_NestedCacheReusesSameMap(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	outer := withWorkflowMetaCache(context.Background())
	inner := withWorkflowMetaCache(outer) // nested seed must be a no-op
	if outer != inner {
		t.Fatal("nested withWorkflowMetaCache must reuse the existing cache context")
	}

	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}
	_ = svc.resolveStepAgentProfile(outer, step)
	_ = svc.buildWorkflowPrompt(inner, "base", step, "task-1", "session-1", false)
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("nested cache GetWorkflowMeta calls = %d, want 1", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_CachesPerWorkflowID(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-a"] = "Rule A"
	stepGetter.workflowPrompts["wf-b"] = "Rule B"
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	stepA := &wfmodels.WorkflowStep{ID: "s-a", WorkflowID: "wf-a"}
	stepB := &wfmodels.WorkflowStep{ID: "s-b", WorkflowID: "wf-b"}

	gotA := svc.buildWorkflowPrompt(ctx, "base", stepA, "task-1", "session-1", false)
	gotB := svc.buildWorkflowPrompt(ctx, "base", stepB, "task-1", "session-1", false)
	if !strings.Contains(gotA, "Rule A") || !strings.Contains(gotB, "Rule B") {
		t.Fatalf("expected distinct prompts per workflow; A=%q B=%q", gotA, gotB)
	}
	// Two workflows → two provider hits; second lookup of A must still be cached.
	_ = svc.buildWorkflowPrompt(ctx, "again", stepA, "task-1", "session-1", false)
	if stepGetter.metaCalls() != 2 {
		t.Fatalf("per-id cache calls = %d, want 2", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_WhitespacePromptOmitsInstructions(t *testing.T) {
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "   \n\t  "
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}

	// Profile still resolves from the same meta read; only prompt is trimmed away.
	if got := svc.resolveStepAgentProfile(ctx, step); got != "profile-wf" {
		t.Fatalf("resolveStepAgentProfile() = %q, want profile-wf", got)
	}
	got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	if got != "base" {
		t.Fatalf("whitespace prompt must omit instructions, got %q", got)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("whitespace path calls = %d, want 1", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_StepProfileOverrideStillReadsPromptOnce(t *testing.T) {
	// Step agent override short-circuits profile resolution before GetWorkflowMeta,
	// but prompt build still needs one read. Seeded cache + one call is enough.
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{
		ID:             "step-1",
		WorkflowID:     "wf-1",
		AgentProfileID: "profile-step",
	}

	if got := svc.resolveStepAgentProfile(ctx, step); got != "profile-step" {
		t.Fatalf("resolveStepAgentProfile() = %q, want step override", got)
	}
	if stepGetter.metaCalls() != 0 {
		t.Fatalf("step override should skip meta for profile, calls = %d", stepGetter.metaCalls())
	}
	got := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	if !strings.Contains(got, "Keep CI green.") {
		t.Fatalf("expected workflow prompt despite step profile override, got %q", got)
	}
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("prompt-only path calls = %d, want 1", stepGetter.metaCalls())
	}
}

func TestGetWorkflowMeta_ConcurrentReadersSinglePopulate(t *testing.T) {
	// singleflight + map cache: concurrent same-ID dual consumers share one load.
	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-wf"
	stepGetter.workflowPrompts["wf-1"] = "Keep CI green."
	// Hold the first provider call long enough that sibling goroutines enter
	// singleflight.Do while it is still in flight.
	stepGetter.workflowMetaDelay = 25 * time.Millisecond
	svc := createTestService(setupTestRepo(t), stepGetter, newMockTaskRepo())

	ctx := withWorkflowMetaCache(context.Background())
	step := &wfmodels.WorkflowStep{ID: "step-1", WorkflowID: "wf-1"}

	var wg sync.WaitGroup
	const n = 32
	wg.Add(n)
	errs := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if got := svc.resolveStepAgentProfile(ctx, step); got != "profile-wf" {
				errs <- "bad profile: " + got
				return
			}
			prompt := svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
			if !strings.Contains(prompt, "Keep CI green.") {
				errs <- "missing prompt: " + prompt
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
	if calls := stepGetter.metaCalls(); calls != 1 {
		t.Fatalf("concurrent same-ID load: GetWorkflowMeta calls = %d, want 1", calls)
	}
	// Follow-up after quiescence must stay on the map cache.
	_ = svc.resolveStepAgentProfile(ctx, step)
	_ = svc.buildWorkflowPrompt(ctx, "base", step, "task-1", "session-1", false)
	if stepGetter.metaCalls() != 1 {
		t.Fatalf("post-storm cache miss: calls = %d, want 1", stepGetter.metaCalls())
	}
}
