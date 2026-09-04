package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// encodeMoveMarker builds a workflow_move_pending marker map carrying the given
// one-shot options, in the shape the orchestrator reader (workflowMovePendingOptions)
// and the task/service writer both use.
func encodeMoveMarker(t *testing.T, fromStepID, moveID string, opts *workflowmove.EntryOptions) map[string]interface{} {
	t.Helper()
	encoded, err := workflowmove.EncodeEntryOptionsJSON(opts)
	if err != nil {
		t.Fatalf("encode entry options: %v", err)
	}
	return map[string]interface{}{
		"from_step_id": fromStepID,
		"move_id":      moveID,
		"options":      string(encoded),
	}
}

// failUpdateTaskRepo fails the first failures UpdateTask calls, then delegates.
// Used to prove setWorkflowMovePendingMarker retries a transient write and
// surfaces a persistent failure (finding D).
type failUpdateTaskRepo struct {
	sessionExecutorStore
	failures int
	calls    int
}

func (r *failUpdateTaskRepo) UpdateTask(ctx context.Context, task *models.Task) error {
	r.calls++
	if r.calls <= r.failures {
		return errors.New("simulated update failure")
	}
	return r.sessionExecutorStore.UpdateTask(ctx, task)
}

func TestSetWorkflowMovePendingMarkerRetriesTransientWrite(t *testing.T) {
	ctx := context.Background()
	base := setupTestRepo(t)
	seedTaskWithoutSession(t, base, "marker-retry", "step1")
	svc := createTestService(base, newMockStepGetter(), newMockTaskRepo())
	repo := &failUpdateTaskRepo{sessionExecutorStore: base, failures: 2}
	svc.repo = repo

	err := svc.setWorkflowMovePendingMarker(ctx, "marker-retry", "step1", "move-1",
		&workflowmove.EntryOptions{Instructions: "do the thing"})
	if err != nil {
		t.Fatalf("expected marker persisted after retries, got %v", err)
	}
	if repo.calls != 3 {
		t.Fatalf("expected 3 UpdateTask attempts (2 failures + 1 success), got %d", repo.calls)
	}
	stored, err := base.GetTask(ctx, "marker-retry")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := stored.Metadata[models.MetaKeyWorkflowMovePending]; !ok {
		t.Fatal("marker must be persisted after a retried write succeeds")
	}
}

func TestSetWorkflowMovePendingMarkerReturnsErrorWhenPersistFails(t *testing.T) {
	ctx := context.Background()
	base := setupTestRepo(t)
	seedTaskWithoutSession(t, base, "marker-lost", "step1")
	svc := createTestService(base, newMockStepGetter(), newMockTaskRepo())
	repo := &failUpdateTaskRepo{sessionExecutorStore: base, failures: workflowMoveMarkerPersistAttempts}
	svc.repo = repo

	err := svc.setWorkflowMovePendingMarker(ctx, "marker-lost", "step1", "move-1",
		&workflowmove.EntryOptions{Instructions: "do the thing"})
	if err == nil {
		t.Fatal("expected an error when every persist attempt fails")
	}
	if repo.calls != workflowMoveMarkerPersistAttempts {
		t.Fatalf("expected %d attempts, got %d", workflowMoveMarkerPersistAttempts, repo.calls)
	}
	stored, err := base.GetTask(ctx, "marker-lost")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := stored.Metadata[models.MetaKeyWorkflowMovePending]; ok {
		t.Fatal("no marker must be persisted when every write fails")
	}
}

// TestSetWorkflowMovePendingMarkerNilOptionsIsNoop keeps the nil-options
// fast-path a no-op that never touches the repository.
func TestSetWorkflowMovePendingMarkerNilOptionsIsNoop(t *testing.T) {
	ctx := context.Background()
	base := setupTestRepo(t)
	seedTaskWithoutSession(t, base, "marker-nil", "step1")
	svc := createTestService(base, newMockStepGetter(), newMockTaskRepo())

	if err := svc.setWorkflowMovePendingMarker(ctx, "marker-nil", "step1", "", nil); err != nil {
		t.Fatalf("nil options must be a no-op, got %v", err)
	}
	stored, err := base.GetTask(ctx, "marker-nil")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, ok := stored.Metadata[models.MetaKeyWorkflowMovePending]; ok {
		t.Fatal("nil options must not write a marker")
	}
}

// waitForRestartCall polls the mock agent manager until at least one
// RestartAgentProcess call is recorded, tolerating whether the on-enter reset
// runs synchronously or on a nested goroutine.
func waitForRestartCall(t *testing.T, agentMgr *mockAgentManager) []string {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		agentMgr.mu.Lock()
		calls := append([]string(nil), agentMgr.restartProcessCalls...)
		agentMgr.mu.Unlock()
		if len(calls) > 0 {
			return calls
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for reset_context overlay to restart the agent process")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestQueuePromotionExistingSessionAppliesAndClearsMoveMarker proves finding B:
// when a WIP-queued optioned move's destination is admitted while the task still
// has an active session, the promotion path must apply the one-shot options
// (here reset_context, observed as the on-enter agent-process restart) and clear
// the marker so it cannot strand and reject a later optioned move.
func TestQueuePromotionExistingSessionAppliesAndClearsMoveMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "promote-session", "promote-session-s1", "destination-step")

	session, err := repo.GetTaskSession(ctx, "promote-session-s1")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.AgentExecutionID = "exec-promote"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("update session: %v", err)
	}
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "exec-promote")

	task, err := repo.GetTask(ctx, "promote-session")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	task.WIPAdmitted = true
	task.QueuedForStepID = ""
	task.Metadata = map[string]interface{}{
		models.MetaKeyQueuePromotionPending: true,
		models.MetaKeyWorkflowMovePending:   encodeMoveMarker(t, "source-step", "move-1", &workflowmove.EntryOptions{ResetContext: true}),
	}
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("persist task: %v", err)
	}

	steps := newMockStepGetter()
	// The durable destination step carries NO reset action; only the marker's
	// reset_context option should introduce it via the overlay.
	steps.steps["destination-step"] = &wfmodels.WorkflowStep{
		ID: "destination-step", WorkflowID: "wf1", Name: "Destination",
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, steps, newMockTaskRepo(), agentMgr)
	entryCompleted := make(chan struct{})
	svc.onTaskQueuePromotionEntryComplete = func() { close(entryCompleted) }

	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: "promote-session"})

	select {
	case <-entryCompleted:
	case <-time.After(2 * time.Second):
		t.Fatal("destination entry did not complete")
	}

	restartCalls := waitForRestartCall(t, agentMgr)
	if len(restartCalls) != 1 || restartCalls[0] != "exec-promote" {
		t.Fatalf("expected reset_context overlay to restart exec-promote once, got %v", restartCalls)
	}

	stored, err := repo.GetTask(ctx, "promote-session")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatal("workflow move marker must be cleared after the existing-session promotion entry")
	}
}

// seedOfficeTaskWithMarker creates an Office task (ProjectID makes IsFromOffice
// true) carrying a pending move marker with the given options.
func seedOfficeTaskWithMarker(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, taskID string, opts *workflowmove.EntryOptions) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step1",
		ProjectID: "proj1", Title: "Office", Description: "prompt", State: v1.TaskStateCreated,
		AssigneeAgentProfileID: "assignee-profile", CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.MetaKeyWorkflowMovePending: encodeMoveMarker(t, "step1", "move-1", opts),
		},
	}))
}

func officeAutoStartStep() *mockStepGetter {
	steps := newMockStepGetter()
	steps.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Work", Position: 1,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}}},
	}
	return steps
}

// TestOfficeAutoStartThreadsMoveInstructionsAndClearsMarker proves finding C:
// a move's one-shot instructions ride on the office run payload, and the marker
// is consumed (cleared) rather than stranded for an Office auto-start.
func TestOfficeAutoStartThreadsMoveInstructionsAndClearsMarker(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeTaskWithMarker(t, repo, "t-office-instr", &workflowmove.EntryOptions{Instructions: "focus on the auth bug"})

	svc := createTestServiceWithAgent(repo, officeAutoStartStep(), newMockTaskRepo(), failIfLaunched(t))
	queued := &fakeRunQueueAdapter{calls: make(chan engine.QueueRunRequest, 1)}
	svc.engineRunQueue = queued
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "t-office-instr", ToStepID: "step2", StepTransitionID: 7})

	select {
	case req := <-queued.calls:
		got, _ := req.Payload["one_time_instructions"].(string)
		if got != "focus on the auth bug" {
			t.Fatalf("office run payload one_time_instructions = %q, want the move instructions", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for office QueueRun")
	}

	stored, err := repo.GetTask(ctx, "t-office-instr")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatal("office auto-start must clear the move marker")
	}
}

// TestOfficeAutoStartSuppressesSkipPromptNoInstructions proves the office side
// of the skip_step_prompt-with-no-instructions suppression: no run is queued and
// the marker is still consumed. Suppression is synchronous (it returns before
// the office branch), so a default select safely asserts no queued run.
func TestOfficeAutoStartSuppressesSkipPromptNoInstructions(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeTaskWithMarker(t, repo, "t-office-skip", &workflowmove.EntryOptions{SkipStepPrompt: true})

	svc := createTestServiceWithAgent(repo, officeAutoStartStep(), newMockTaskRepo(), failIfLaunched(t))
	queued := &fakeRunQueueAdapter{calls: make(chan engine.QueueRunRequest, 1)}
	svc.engineRunQueue = queued
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{TaskID: "t-office-skip", ToStepID: "step2", StepTransitionID: 8})

	select {
	case <-queued.calls:
		t.Fatal("skip_step_prompt with no instructions must not queue an office run")
	default:
	}

	stored, err := repo.GetTask(ctx, "t-office-skip")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if _, pending := stored.Metadata[models.MetaKeyWorkflowMovePending]; pending {
		t.Fatal("suppressed office auto-start must still clear the move marker")
	}
}
