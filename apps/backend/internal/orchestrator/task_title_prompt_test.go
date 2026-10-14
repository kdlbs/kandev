package orchestrator

// Coverage for the {task_title} prompt placeholder (REQ-TWS-006): substitution
// at both buildWorkflowPrompt call sites, the no-lookup fast path, graceful
// degradation on lookup failure, and title text never being re-scanned for
// other tokens.

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap/zapcore"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// taskTitleCountingRepo counts GetTask calls so tests can assert a template
// without the token performs no lookup.
type taskTitleCountingRepo struct {
	*sqliterepo.Repository
	calls int32
}

func (r *taskTitleCountingRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	atomic.AddInt32(&r.calls, 1)
	return r.Repository.GetTask(ctx, id)
}

// taskTitleErrorRepo always fails GetTask.
type taskTitleErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *taskTitleErrorRepo) GetTask(_ context.Context, _ string) (*models.Task, error) {
	return nil, r.err
}

func setTaskTitle(t *testing.T, repo *sqliterepo.Repository, task *models.Task, title string) {
	t.Helper()
	task.Title = title
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask title: %v", err)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_NoTokenIssuesNoLookup(t *testing.T) {
	repo := &taskTitleCountingRepo{Repository: setupTestRepo(t)}
	seedStepEntryTask(t, repo.Repository, "task-tt-1", "wf-tt-1", "step-a")
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Review {task_id}: {{task_prompt}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-tt-1", "session-1", false)

	if got != "Review task-tt-1: base" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "Review task-tt-1: base")
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 0 {
		t.Fatalf("GetTask calls = %d, want 0 (template carries no token)", calls)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_SubstitutesEveryOccurrence(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-tt-2", "wf-tt-2", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Work on: {task_title} ({task_title})"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-tt-2", "session-1", false)

	if got != "Work on: Test Task (Test Task)" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "Work on: Test Task (Test Task)")
	}
}

func TestBuildWorkflowPrompt_TaskTitle_LookupErrorLeavesTokenLiteral(t *testing.T) {
	repo := &taskTitleErrorRepo{Repository: setupTestRepo(t), err: errors.New("db unavailable")}
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	log, logs := observingTestLogger(t)
	svc.logger = log
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Task {task_title}:\n\n{{task_prompt}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-tt-3", "session-1", false)

	want := "Task {task_title}:\n\nbase"
	if got != want {
		t.Fatalf("buildWorkflowPrompt() on lookup error = %q, want %q", got, want)
	}
	var warnings []map[string]interface{}
	for _, e := range logs.All() {
		if e.Level == zapcore.WarnLevel {
			warnings = append(warnings, e.ContextMap())
		}
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warn entries, want exactly 1 (all entries: %+v)", len(warnings), logs.All())
	}
	if warnings[0]["task_id"] != "task-tt-3" {
		t.Errorf("warning task_id = %v, want %q", warnings[0]["task_id"], "task-tt-3")
	}
}

func TestBuildWorkflowPrompt_TaskTitle_EmptyTaskIDNoLookupTokenLiteral(t *testing.T) {
	repo := &taskTitleCountingRepo{Repository: setupTestRepo(t)}
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Task {task_title}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "", "session-1", false)

	if got != "Task {task_title}" {
		t.Fatalf("buildWorkflowPrompt() with empty task id = %q, want %q", got, "Task {task_title}")
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 0 {
		t.Fatalf("GetTask calls = %d, want 0 for empty task id", calls)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_BothCallSitesSubstitute(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-tt-5", "wf-tt-5", "step-a")
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-tt-5"] = "Workflow-level: {task_title}."
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf-tt-5", Prompt: "Step-level: {task_title}."}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-tt-5", "session-1", false)

	if !strings.Contains(got, "Workflow-level: Test Task.") {
		t.Fatalf("expected workflow-level substitution, got %q", got)
	}
	if !strings.Contains(got, "Step-level: Test Task.") {
		t.Fatalf("expected step-level substitution, got %q", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_CoexistsWithOtherPlaceholders(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-tt-6", "wf-tt-6", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{
		ID:     "step-a",
		Prompt: "{task_id} / {step_entry_number} / {task_title}: {{task_prompt}}",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "the description", step, "task-tt-6", "session-1", false)

	want := "task-tt-6 / 1 / Test Task: the description"
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_LiteralTokenInBasePromptNeverSubstituted(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-tt-7", "wf-tt-7", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{task_title}: {{task_prompt}}"}
	basePrompt := "The description literally says {task_title} here."

	got := svc.buildWorkflowPrompt(context.Background(), basePrompt, step, "task-tt-7", "session-1", false)

	want := "Test Task: The description literally says {task_title} here."
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_TitleTextNeverReinterpreted(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-tt-8", "wf-tt-8", "step-a")
	title := "Fix {{task_prompt}} and {task_id} and {step_entry_number}"
	setTaskTitle(t, repo, task, title)
	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-tt-8"] = "Workflow: {task_title}"
	svc := createTestService(repo, stepGetter, newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf-tt-8", Prompt: "Step: {task_title}\n\n{{task_prompt}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "the description", step, "task-tt-8", "session-1", false)

	if !strings.Contains(got, "Workflow: "+title+"\n") {
		t.Fatalf("workflow-level title was not inserted verbatim, got %q", got)
	}
	if !strings.HasSuffix(got, "Step: "+title+"\n\nthe description") {
		t.Fatalf("step-level title was not inserted verbatim before the description, got %q", got)
	}
}

func TestBuildWorkflowPrompt_TaskTitle_FirstTaskPromptOnlyReplaced(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-tt-9", "wf-tt-9", "step-a")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{task_title}: {{task_prompt}} then {{task_prompt}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-tt-9", "session-1", false)

	want := "Test Task: base then {{task_prompt}}"
	if got != want {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, want)
	}
}
