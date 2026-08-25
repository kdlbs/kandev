package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestOfficeAutoStartIdempotencyKey pins the key shape the fix depends on:
// office-default.yml's any_reject -> work transition routes a rejected
// review card back to the same (task, agent profile, step) tuple, so the key
// must carry task.UpdatedAt as a per-entry component or every re-entry
// collides with the first one forever (silently deduped within 24h, then a
// hard failure against the permanent idx_run_idempotency unique index).
func TestOfficeAutoStartIdempotencyKey(t *testing.T) {
	entryOne := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entryTwo := time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)

	taskAtEntryOne := &models.Task{ID: "t1", UpdatedAt: entryOne}
	taskAtEntryOneAgain := &models.Task{ID: "t1", UpdatedAt: entryOne}
	taskAtEntryTwo := &models.Task{ID: "t1", UpdatedAt: entryTwo}

	keyOne := officeAutoStartIdempotencyKey(taskAtEntryOne, "agent-1", "step2")
	keyOneRepeat := officeAutoStartIdempotencyKey(taskAtEntryOneAgain, "agent-1", "step2")
	keyTwo := officeAutoStartIdempotencyKey(taskAtEntryTwo, "agent-1", "step2")

	if keyOne != keyOneRepeat {
		t.Errorf("key must be identical within one step entry (same task.UpdatedAt): %q != %q", keyOne, keyOneRepeat)
	}
	if keyOne == keyTwo {
		t.Errorf("key must differ across step entries (different task.UpdatedAt), got identical key %q for both", keyOne)
	}
}

// TestAutoStartOfficeTaskLogsQueuedOutcomeAtInfo pins that a real insert
// (QueueOutcomeQueued) is logged as an info-level "queued" message, and the
// log is only emitted after the queue attempt resolves — not asserted
// upfront before the goroutine even runs.
func TestAutoStartOfficeTaskLogsQueuedOutcomeAtInfo(t *testing.T) {
	ctx := context.Background()
	const wantMsg = "task.moved: queued office run (no session, auto-start step)"

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:                     "t-office-log-queued",
		WorkspaceID:            "ws1",
		WorkflowID:             "wf1",
		WorkflowStepID:         "step1",
		ProjectID:              "proj1",
		Title:                  "Office Task",
		Description:            "prompt",
		State:                  v1.TaskStateCreated,
		AssigneeAgentProfileID: "assignee-profile",
		CreatedAt:              now,
		UpdatedAt:              now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Work", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
	log, logs, seen := observedTestLoggerWatching(t, wantMsg)
	svc.logger = log
	svc.engineRunQueue = &fakeRunQueueAdapter{outcome: engine.QueueOutcomeQueued}
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
		TaskID:   "t-office-log-queued",
		ToStepID: "step2",
	})

	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the queued-outcome log line")
	}

	if n := logs.FilterMessage(wantMsg).Len(); n != 1 {
		t.Errorf("expected exactly one %q log entry, got %d", wantMsg, n)
	}
}

// TestAutoStartOfficeTaskLogsDedupedOutcomeNotAsQueued pins the other half
// of the outcome-reporting fix: a deduped (or coalesced) QueueRun call must
// NOT be logged as a successful queue, because the caller only knows what
// actually happened once the attempt resolves. Before this fix, the "queued"
// log was written unconditionally before the async QueueRun call was even
// made.
func TestAutoStartOfficeTaskLogsDedupedOutcomeNotAsQueued(t *testing.T) {
	ctx := context.Background()
	const queuedMsg = "task.moved: queued office run (no session, auto-start step)"
	const dedupedMsg = "task.moved: office auto-start run not queued"

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:                     "t-office-log-deduped",
		WorkspaceID:            "ws1",
		WorkflowID:             "wf1",
		WorkflowStepID:         "step1",
		ProjectID:              "proj1",
		Title:                  "Office Task",
		Description:            "prompt",
		State:                  v1.TaskStateCreated,
		AssigneeAgentProfileID: "assignee-profile",
		CreatedAt:              now,
		UpdatedAt:              now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Work", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
	log, logs, seen := observedTestLoggerWatching(t, dedupedMsg)
	svc.logger = log
	svc.engineRunQueue = &fakeRunQueueAdapter{outcome: engine.QueueOutcomeDeduped}
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
		TaskID:   "t-office-log-deduped",
		ToStepID: "step2",
	})

	select {
	case <-seen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the deduped-outcome log line")
	}

	if n := logs.FilterMessage(queuedMsg).Len(); n != 0 {
		t.Errorf("a deduped QueueRun call must not be logged as %q, got %d such entries", queuedMsg, n)
	}
}

// TestOfficeAutoStartIdempotencyKeyAcrossRealDeliveries connects the two unit
// facts pinned above (a fixed task.UpdatedAt reproduces the same key; a
// changed one does not) to the actual re-entry path: handleTaskMovedNoSession
// reloads the task from the repository on every delivery, so what actually
// keeps two deliveries of the same step entry deduping is that nothing in
// between them writes to the task row. Drive that through the real SQLite
// repo instead of constructing *models.Task values by hand.
func TestOfficeAutoStartIdempotencyKeyAcrossRealDeliveries(t *testing.T) {
	ctx := context.Background()

	repo := setupTestRepo(t)
	now := time.Now().UTC()
	requireNoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}))
	requireNoError(t, repo.CreateTask(ctx, &models.Task{
		ID:                     "t-office-real-deliveries",
		WorkspaceID:            "ws1",
		WorkflowID:             "wf1",
		WorkflowStepID:         "step1",
		ProjectID:              "proj1",
		Title:                  "Office Task",
		Description:            "prompt",
		State:                  v1.TaskStateCreated,
		AssigneeAgentProfileID: "assignee-profile",
		CreatedAt:              now,
		UpdatedAt:              now,
	}))

	stepGetter := newMockStepGetter()
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Work", Position: 1,
		Events: wfmodels.StepEvents{
			OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
		},
	}

	adapter := &fakeRunQueueAdapter{calls: make(chan engine.QueueRunRequest, 3)}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
	svc.engineRunQueue = adapter
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	deliver := func() engine.QueueRunRequest {
		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:   "t-office-real-deliveries",
			ToStepID: "step2",
		})
		select {
		case req := <-adapter.calls:
			return req
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for QueueRun to be called")
			return engine.QueueRunRequest{}
		}
	}

	firstDelivery := deliver()
	secondDelivery := deliver()
	if firstDelivery.IdempotencyKey != secondDelivery.IdempotencyKey {
		t.Errorf("two deliveries of the same step entry must produce the same idempotency key, got %q and %q",
			firstDelivery.IdempotencyKey, secondDelivery.IdempotencyKey)
	}

	task, err := repo.GetTask(ctx, "t-office-real-deliveries")
	requireNoError(t, err)
	// No field on task is changed here: updateTaskTx always stamps a fresh
	// updated_at (task.go:538's r.nowUTC() call), so a bare UpdateTask is
	// enough to simulate the row write a real step re-entry performs.
	requireNoError(t, repo.UpdateTask(ctx, task))

	thirdDelivery := deliver()
	if thirdDelivery.IdempotencyKey == secondDelivery.IdempotencyKey {
		t.Errorf("a real re-entry (task row updated in between) must change the idempotency key, but it stayed %q",
			thirdDelivery.IdempotencyKey)
	}
}
