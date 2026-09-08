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
// must use the immutable transition row and ignore unrelated task writes.
func TestOfficeAutoStartIdempotencyKey(t *testing.T) {
	entryOne := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	unrelatedWrite := entryOne.Add(time.Hour)

	taskAtEntryOne := &models.Task{ID: "t1", UpdatedAt: entryOne, WorkflowStepTransitionID: 17}
	taskAtEntryOneAgain := &models.Task{ID: "t1", UpdatedAt: unrelatedWrite, WorkflowStepTransitionID: 17}
	taskAtEntryTwo := &models.Task{ID: "t1", UpdatedAt: unrelatedWrite, WorkflowStepTransitionID: 18}

	keyOne := officeAutoStartIdempotencyKey(taskAtEntryOne, "agent-1", "step2", taskAtEntryOne.WorkflowStepTransitionID)
	keyOneRepeat := officeAutoStartIdempotencyKey(taskAtEntryOneAgain, "agent-1", "step2", taskAtEntryOneAgain.WorkflowStepTransitionID)
	keyTwo := officeAutoStartIdempotencyKey(taskAtEntryTwo, "agent-1", "step2", taskAtEntryTwo.WorkflowStepTransitionID)

	if keyOne != keyOneRepeat {
		t.Errorf("key must ignore unrelated task writes within one step entry: %q != %q", keyOne, keyOneRepeat)
	}
	if keyOne == keyTwo {
		t.Errorf("key must differ across step entries, got identical key %q", keyOne)
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

	adapter := &fakeRunQueueAdapter{calls: make(chan engine.QueueRunRequest, 4)}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), failIfLaunched(t))
	svc.engineRunQueue = adapter
	svc.enginePrimary = &fakePrimaryAgentResolver{agentProfileID: "resolved-primary"}

	deliver := func(stepTransitionID int64) engine.QueueRunRequest {
		svc.handleTaskMovedNoSession(ctx, watcher.TaskMovedEventData{
			TaskID:           "t-office-real-deliveries",
			ToStepID:         "step2",
			StepTransitionID: stepTransitionID,
		})
		select {
		case req := <-adapter.calls:
			return req
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for QueueRun to be called")
			return engine.QueueRunRequest{}
		}
	}

	task, err := repo.GetTask(ctx, "t-office-real-deliveries")
	requireNoError(t, err)
	task.WorkflowStepID = "step2"
	requireNoError(t, repo.UpdateTask(ctx, task))
	entryID := task.WorkflowStepTransitionID
	if entryID == 0 {
		t.Fatal("step transition did not return a ledger identifier")
	}

	firstDelivery := deliver(entryID)
	secondDelivery := deliver(entryID)
	if firstDelivery.IdempotencyKey != secondDelivery.IdempotencyKey {
		t.Errorf("two deliveries of the same step entry must produce the same idempotency key, got %q and %q",
			firstDelivery.IdempotencyKey, secondDelivery.IdempotencyKey)
	}

	// An unrelated task write changes updated_at but must not change the entry ID.
	requireNoError(t, repo.UpdateTask(ctx, task))

	thirdDelivery := deliver(entryID)
	if thirdDelivery.IdempotencyKey != secondDelivery.IdempotencyKey {
		t.Errorf("an unrelated task write must not change the idempotency key, got %q and %q",
			secondDelivery.IdempotencyKey, thirdDelivery.IdempotencyKey)
	}

	task.WorkflowStepID = "step1"
	requireNoError(t, repo.UpdateTask(ctx, task))
	task.WorkflowStepID = "step2"
	requireNoError(t, repo.UpdateTask(ctx, task))
	reentryID := task.WorkflowStepTransitionID
	if reentryID == 0 || reentryID == entryID {
		t.Fatalf("re-entry must have a new ledger identifier, got first=%d re-entry=%d", entryID, reentryID)
	}
	fourthDelivery := deliver(reentryID)
	if fourthDelivery.IdempotencyKey == thirdDelivery.IdempotencyKey {
		t.Errorf("a leave-and-re-enter transition must change the idempotency key, but it stayed %q",
			fourthDelivery.IdempotencyKey)
	}
}
