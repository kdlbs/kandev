package orchestrator

// Coverage for the {step_entry_number} prompt placeholder (REQ-TWS-001,
// REQ-TWS-002): server-computed substitution at both buildWorkflowPrompt call
// sites, wired against a real sqlite repository so the ledger's COUNT(*) is
// exercised, not a stub.

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// stepEntryCountingRepo wraps a real repository and counts calls to
// CountStepEntries, so tests can assert a template without the token issues
// no query (AC-TWS-001.6, NFR-1).
type stepEntryCountingRepo struct {
	*sqliterepo.Repository
	calls int32
}

func (r *stepEntryCountingRepo) CountStepEntries(ctx context.Context, taskID, stepID string) (int, error) {
	atomic.AddInt32(&r.calls, 1)
	return r.Repository.CountStepEntries(ctx, taskID, stepID)
}

// stepEntryErrorRepo always fails CountStepEntries, exercising the
// AC-TWS-002.1/.2 degrade-visibly path.
type stepEntryErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *stepEntryErrorRepo) CountStepEntries(_ context.Context, _, _ string) (int, error) {
	return 0, r.err
}

// seedStepEntryTask creates a workspace, workflow and task at stepID so the
// task's genesis ledger row exists for (taskID, stepID). Reuses "ws-see"/
// workflowID across calls within one test, mirroring seedSession's pattern of
// ignoring an "already exists" error on the shared parents.
func seedStepEntryTask(t *testing.T, repo *sqliterepo.Repository, taskID, workflowID, stepID string) *models.Task {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-see", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: "ws-see", Name: "Test Workflow", CreatedAt: now, UpdatedAt: now})
	task := &models.Task{
		ID:             taskID,
		WorkspaceID:    "ws-see",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          "Test Task",
		State:          v1.TaskStateInProgress,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func moveTaskToStep(t *testing.T, repo *sqliterepo.Repository, task *models.Task, stepID string) {
	t.Helper()
	task.WorkflowStepID = stepID
	if err := repo.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("UpdateTask to %s: %v", stepID, err)
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_SubstitutesRecordedEntryCount(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-sen-1", "wf-sen-1", "step-a")
	// One leave-and-return after genesis: 2 recorded entries into step-a.
	moveTaskToStep(t, repo, task, "step-b")
	moveTaskToStep(t, repo, task, "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "This is entry {step_entry_number}."}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-1", "session-1", false)

	if got != "This is entry 2." {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "This is entry 2.")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_ZeroRowsFloorsAtOne(t *testing.T) {
	repo := setupTestRepo(t)
	// Task exists but never recorded an entry into "step-never-visited"
	// (the pre-ledger / zero-row case).
	seedStepEntryTask(t, repo, "task-sen-2", "wf-sen-2", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-never-visited", Prompt: "Entry {step_entry_number}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-2", "session-1", false)

	if got != "Entry 1" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "Entry 1")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_QueryErrorLeavesTokenLiteral(t *testing.T) {
	repo := &stepEntryErrorRepo{Repository: setupTestRepo(t), err: errors.New("db unavailable")}
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Entry {step_entry_number} of the review."}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-3", "session-1", false)

	if got != "Entry {step_entry_number} of the review." {
		t.Fatalf("buildWorkflowPrompt() on query error = %q, want token left literal", got)
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_NoTokenIssuesNoQuery(t *testing.T) {
	repo := &stepEntryCountingRepo{Repository: setupTestRepo(t)}
	seedStepEntryTask(t, repo.Repository, "task-sen-4", "wf-sen-4", "step-a")
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "No token here."}

	svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-4", "session-1", false)

	if calls := atomic.LoadInt32(&repo.calls); calls != 0 {
		t.Fatalf("CountStepEntries calls = %d, want 0 (template carries no token)", calls)
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_EmptyTaskIDNoQueryFloorsAtOne(t *testing.T) {
	repo := &stepEntryCountingRepo{Repository: setupTestRepo(t)}
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Entry {step_entry_number}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "", "session-1", false)

	if got != "Entry 1" {
		t.Fatalf("buildWorkflowPrompt() with empty task id = %q, want %q", got, "Entry 1")
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 0 {
		t.Fatalf("CountStepEntries calls = %d, want 0 for empty task id", calls)
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_EmptyStepIDNoQueryFloorsAtOne(t *testing.T) {
	repo := &stepEntryCountingRepo{Repository: setupTestRepo(t)}
	svc := createTestService(repo.Repository, newMockStepGetter(), newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{ID: "", Prompt: "Entry {step_entry_number}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-5", "session-1", false)

	if got != "Entry 1" {
		t.Fatalf("buildWorkflowPrompt() with empty step id = %q, want %q", got, "Entry 1")
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 0 {
		t.Fatalf("CountStepEntries calls = %d, want 0 for empty step id", calls)
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_BothTokensDoNotInteract(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-sen-6", "wf-sen-6", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Task {task_id}, entry {step_entry_number}."}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-6", "session-1", false)

	if got != "Task task-sen-6, entry 1." {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "Task task-sen-6, entry 1.")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_EveryOccurrenceSubstituted(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-sen-7", "wf-sen-7", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{step_entry_number} and {step_entry_number}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-7", "session-1", false)

	if got != "1 and 1" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "1 and 1")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_DoubleBraceRendersOuterBracesLiteral(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-sen-8", "wf-sen-8", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "{{step_entry_number}}"}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-8", "session-1", false)

	if got != "{1}" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", got, "{1}")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_SameEntryTwiceSameNumber(t *testing.T) {
	repo := setupTestRepo(t)
	seedStepEntryTask(t, repo, "task-sen-9", "wf-sen-9", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Entry {step_entry_number}"}

	first := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-9", "session-1", false)
	second := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-9", "session-1", false)

	if first != second {
		t.Fatalf("re-prompting the same entry gave different numbers: %q vs %q", first, second)
	}
	if first != "Entry 1" {
		t.Fatalf("buildWorkflowPrompt() = %q, want %q", first, "Entry 1")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_LeaveAndReturnIncrementsNumber(t *testing.T) {
	repo := setupTestRepo(t)
	task := seedStepEntryTask(t, repo, "task-sen-10", "wf-sen-10", "step-a")

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	step := &wfmodels.WorkflowStep{ID: "step-a", Prompt: "Entry {step_entry_number}"}

	first := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-10", "session-1", false)
	if first != "Entry 1" {
		t.Fatalf("first build = %q, want %q", first, "Entry 1")
	}

	moveTaskToStep(t, repo, task, "step-b")
	moveTaskToStep(t, repo, task, "step-a")

	second := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-10", "session-1", false)
	if second != "Entry 2" {
		t.Fatalf("second build after return = %q, want %q", second, "Entry 2")
	}
}

func TestBuildWorkflowPrompt_StepEntryNumber_BothCallSitesSubstituteOneQueryEach(t *testing.T) {
	repo := &stepEntryCountingRepo{Repository: setupTestRepo(t)}
	seedStepEntryTask(t, repo.Repository, "task-sen-11", "wf-sen-11", "step-a")

	stepGetter := newMockStepGetter()
	stepGetter.workflowPrompts["wf-sen-11"] = "Workflow-level entry {step_entry_number}."
	svc := createTestService(repo.Repository, stepGetter, newMockTaskRepo())
	svc.repo = repo
	step := &wfmodels.WorkflowStep{
		ID:         "step-a",
		WorkflowID: "wf-sen-11",
		Prompt:     "Step-level entry {step_entry_number}.",
	}

	got := svc.buildWorkflowPrompt(context.Background(), "base", step, "task-sen-11", "session-1", false)

	if !strings.Contains(got, "Workflow-level entry 1.") {
		t.Fatalf("expected workflow-level substitution, got %q", got)
	}
	if !strings.Contains(got, "Step-level entry 1.") {
		t.Fatalf("expected step-level substitution, got %q", got)
	}
	if calls := atomic.LoadInt32(&repo.calls); calls != 2 {
		t.Fatalf("CountStepEntries calls = %d, want 2 (one per token-bearing template, NFR-1)", calls)
	}
}
