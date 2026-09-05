package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// recordingOfficeStatusUpdater is a test double for OfficeTaskStatusUpdater
// that performs the same kind of write UpdateTaskStatus does (persist a
// state via the shared repo) and returns a configurable error, so tests
// can observe both what the orchestrator asked for and what state ends
// up persisted — without standing up the full dashboard service.
type recordingOfficeStatusUpdater struct {
	repo     *sqliterepo.Repository
	newState v1.TaskState // state to persist; defaults to COMPLETED
	err      error        // returned after the write, mirrors UpdateTaskStatus's contract
	calls    []dashboard.TaskStatusUpdateRequest
}

func (u *recordingOfficeStatusUpdater) UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error {
	u.calls = append(u.calls, req)
	state := v1.TaskStateCompleted
	if u.newState != "" {
		state = u.newState
	}
	task, err := u.repo.GetTask(ctx, req.TaskID)
	if err != nil {
		return err
	}
	task.State = state
	task.UpdatedAt = time.Now().UTC()
	if err := u.repo.UpdateTask(ctx, task); err != nil {
		return err
	}
	return u.err
}

// requireCreateOfficeTask creates a task with a non-empty ProjectID so the
// repo's IsFromOffice projection reports true (see isFromOfficeProjection).
func requireCreateOfficeTask(t *testing.T, repo *sqliterepo.Repository, task *models.Task) {
	t.Helper()
	if task.ProjectID == "" {
		task.ProjectID = "proj-office"
	}
	requireCreateTask(t, repo, task)
}

// TestMarkOfficeTaskCompleted_NoApprovers_WritesDoneViaSeam is test-matrix
// case (4): an orchestrator terminal-step move for an Office task with no
// approvers pending routes through the seam and ends up COMPLETED.
func TestMarkOfficeTaskCompleted_NoApprovers_WritesDoneViaSeam(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-done", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-done", "child_done")

	if len(updater.calls) != 1 || updater.calls[0].NewStatus != "done" {
		t.Fatalf("seam calls = %+v, want one call with NewStatus=done", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-done")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED", task.State)
	}
}

// TestMarkOfficeTaskCompleted_ApproversPending_RedirectsToReview is
// test-matrix case (3): pending approvers redirect the persisted state to
// in_review (REVIEW), and the ApprovalsPendingError the seam returns is
// not a failure — it must not trigger the raw completion write.
func TestMarkOfficeTaskCompleted_ApproversPending_RedirectsToReview(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-pending", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{
		repo:     repo,
		newState: v1.TaskStateReview,
		err:      &dashboard.ApprovalsPendingError{Pending: []string{"agent-A"}},
	}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-pending", "child_done")

	if len(updater.calls) != 1 || updater.calls[0].NewStatus != "done" {
		t.Fatalf("seam calls = %+v, want one call with NewStatus=done", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-pending")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateReview {
		t.Fatalf("state = %q, want REVIEW (redirected, not overwritten by the raw completion path)", task.State)
	}
}

// TestMarkTaskCompleted_NonOfficeTask_UsesRawPathUnchanged is test-matrix
// case (c): a non-Office task's terminal-step completion is untouched by
// this change — it never reaches the seam and keeps the raw write.
func TestMarkTaskCompleted_NonOfficeTask_UsesRawPathUnchanged(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "kanban-task", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Kanban task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "kanban-task", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none: non-Office tasks must not reach the Office seam", updater.calls)
	}
	task, err := repo.GetTask(ctx, "kanban-task")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED via the unchanged raw path", task.State)
	}
}

// TestMarkOfficeTaskCompleted_StaleStep_SkipsSeam asserts an Office task
// whose current WorkflowStepID no longer matches the terminal step named by
// a replayed/stale move event never reaches the seam and keeps its state.
func TestMarkOfficeTaskCompleted_StaleStep_SkipsSeam(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-stale-step", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_work",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-stale-step", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none: a stale-step move must not reach the Office seam", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-stale-step")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want unchanged IN_PROGRESS", task.State)
	}
	if task.WorkflowStepID != "child_work" {
		t.Fatalf("workflow_step_id = %q, want unchanged child_work", task.WorkflowStepID)
	}
}

// TestMarkOfficeTaskCompleted_AlreadyCancelled_SkipsSeam asserts an Office
// task already in a terminal state (CANCELLED) never reaches the seam and
// is not resurrected as COMPLETED.
func TestMarkOfficeTaskCompleted_AlreadyCancelled_SkipsSeam(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-cancelled", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateCancelled, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-cancelled", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none: an already-terminal task must not reach the Office seam", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-cancelled")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCancelled {
		t.Fatalf("state = %q, want unchanged CANCELLED (not resurrected as COMPLETED)", task.State)
	}
}

// TestMarkOfficeTaskCompleted_AlreadyCompleted_SkipsSeam asserts a
// redelivered terminal-step-move event for an already-COMPLETED Office task
// never reaches the seam, so it does not re-run the status pipeline
// (duplicate activity rows, duplicate reactivity cascades).
func TestMarkOfficeTaskCompleted_AlreadyCompleted_SkipsSeam(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-completed", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateCompleted, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-completed", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none: a redelivered terminal move for an already-completed task must not reach the Office seam", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-completed")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want unchanged COMPLETED", task.State)
	}
}

// TestMarkOfficeTaskCompleted_SeamNil_SkipsWrite is test-matrix case (d):
// with no seam wired, an Office task's terminal completion is skipped
// rather than falling back to the raw write, which would bypass the
// approval gate.
func TestMarkOfficeTaskCompleted_SeamNil_SkipsWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-no-seam", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := &Service{logger: testLogger(), repo: repo}
	// No SetOfficeTaskStatusUpdater call: the seam is nil.

	svc.markTaskCompletedForTerminalStep(ctx, "office-no-seam", "child_done")

	task, err := repo.GetTask(ctx, "office-no-seam")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want unchanged IN_PROGRESS: a nil seam must skip the write, not fall through", task.State)
	}
}

// recordingTaskEventPublisher captures PublishTaskUpdated /
// PublishTaskStateChanged calls so tests can assert the emitted events
// themselves, not just the persisted task state.
type recordingTaskEventPublisher struct {
	updated      []string
	stateChanges []recordedStateChange
}

type recordedStateChange struct {
	taskID   string
	oldState v1.TaskState
	newState v1.TaskState
}

func (r *recordingTaskEventPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	if task != nil {
		r.updated = append(r.updated, task.ID)
	}
}

func (r *recordingTaskEventPublisher) PublishTaskStateChanged(_ context.Context, task *models.Task, oldState v1.TaskState) {
	if task == nil {
		return
	}
	r.stateChanges = append(r.stateChanges, recordedStateChange{taskID: task.ID, oldState: oldState, newState: task.State})
}

func (r *recordingTaskEventPublisher) PublishTaskActivityIfChanged(context.Context, string) {}

// waitStepWithChildrenCompletedTrigger seeds a two-step workflow (position 0
// "step-wait" with an on_children_completed move-to-next action, position 1
// "step-done") on the mockStepGetter and returns it. seedSession always
// creates its task on workflow "wf1", so the steps are registered there.
func waitStepWithChildrenCompletedTrigger() *mockStepGetter {
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-wait"] = &wfmodels.WorkflowStep{
		ID: "step-wait", WorkflowID: "wf1", Name: "Wait for children", Position: 0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf1", Name: "Done", Position: 1,
	}
	return stepGetter
}

// TestMarkOfficeTaskCompleted_NoApprovers_FiresTerminalSideEffects is the
// BLOCKER-1 regression test (review round 2): the raw (non-Office)
// completion path publishes task.updated / task.state_changed and drives
// the parent's on_children_completed trigger. The Office seam dropped all
// three when it landed on COMPLETED, so dependency resolution, the parent
// workflow transition, and every task.updated subscriber silently stopped
// seeing Office completions. Assert the emitted events and the parent's
// resulting workflow step, not just the child's persisted state.
func TestMarkOfficeTaskCompleted_NoApprovers_FiresTerminalSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := waitStepWithChildrenCompletedTrigger()
	seedSession(t, repo, "parent-office", "parent-office-session", "step-wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-done-fx", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent-office",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	svc.SetOfficeTaskStatusUpdater(&recordingOfficeStatusUpdater{repo: repo})
	events := &recordingTaskEventPublisher{}
	svc.SetTaskEventPublisher(events)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	svc.markTaskCompletedForTerminalStep(ctx, "office-done-fx", "child_done")
	// The parent's on_children_completed transition launches its own step's
	// session prep on a detached goroutine (launchProcessOnEnter); join it
	// before reading events/parent state below, or those reads race the
	// goroutine's own writes (recordingTaskEventPublisher is not
	// synchronized, matching event_handlers_children_completed_test.go's
	// waitForChildrenCompletedOnEnter pattern for the same trigger).
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	// The parent's own on_children_completed transition legitimately
	// republishes task.updated for the parent itself; only assert on the
	// completed child's events, which are what BLOCKER 1 dropped.
	childUpdates := 0
	for _, id := range events.updated {
		if id == "office-done-fx" {
			childUpdates++
		}
	}
	if childUpdates != 1 {
		t.Fatalf("PublishTaskUpdated calls = %+v, want exactly one for office-done-fx", events.updated)
	}
	var childChanges []recordedStateChange
	for _, c := range events.stateChanges {
		if c.taskID == "office-done-fx" {
			childChanges = append(childChanges, c)
		}
	}
	if len(childChanges) != 1 {
		t.Fatalf("PublishTaskStateChanged calls for office-done-fx = %+v, want exactly one", childChanges)
	}
	if change := childChanges[0]; change.oldState != v1.TaskStateInProgress || change.newState != v1.TaskStateCompleted {
		t.Fatalf("state change = %+v, want IN_PROGRESS -> COMPLETED", change)
	}

	parentAfter, err := repo.GetTask(ctx, "parent-office")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parentAfter.WorkflowStepID != "step-done" {
		t.Fatalf("parent.WorkflowStepID = %q, want step-done: on_children_completed must fire for an Office completion",
			parentAfter.WorkflowStepID)
	}
}

// TestMarkOfficeTaskCompleted_ApproversPending_DoesNotFireCompletionSideEffects
// is the negative half of the BLOCKER-1 regression: a gate redirect to
// in_review is not a completion, so it must not publish task.updated /
// task.state_changed, nor drive the parent's on_children_completed trigger.
func TestMarkOfficeTaskCompleted_ApproversPending_DoesNotFireCompletionSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := waitStepWithChildrenCompletedTrigger()
	seedSession(t, repo, "parent-office", "parent-office-session", "step-wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-pending-fx", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent-office",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	svc.SetOfficeTaskStatusUpdater(&recordingOfficeStatusUpdater{
		repo:     repo,
		newState: v1.TaskStateReview,
		err:      &dashboard.ApprovalsPendingError{Pending: []string{"agent-A"}},
	})
	events := &recordingTaskEventPublisher{}
	svc.SetTaskEventPublisher(events)

	svc.markTaskCompletedForTerminalStep(ctx, "office-pending-fx", "child_done")

	if len(events.updated) != 0 || len(events.stateChanges) != 0 {
		t.Fatalf("events = updated=%v stateChanges=%v, want none: a gate redirect is not a completion",
			events.updated, events.stateChanges)
	}
	parentAfter, err := repo.GetTask(ctx, "parent-office")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parentAfter.WorkflowStepID != "step-wait" {
		t.Fatalf("parent.WorkflowStepID = %q, want unchanged step-wait: a redirect must not drive on_children_completed",
			parentAfter.WorkflowStepID)
	}
}

// blockingOfficeStatusUpdater wraps recordingOfficeStatusUpdater so a test
// can hold the FIRST UpdateTaskStatus call open (simulating a slow seam
// call) while a second, concurrent delivery races in. Only the first call
// blocks: if the fix under test fails to serialize duplicate deliveries, a
// second call must still return promptly (recorded, not blocked) so the
// test fails on an assertion instead of hanging.
type blockingOfficeStatusUpdater struct {
	inner   *recordingOfficeStatusUpdater
	started chan struct{}
	proceed chan struct{}
	calls   atomic.Int32
}

func (u *blockingOfficeStatusUpdater) UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error {
	if u.calls.Add(1) == 1 {
		close(u.started)
		<-u.proceed
	}
	return u.inner.UpdateTaskStatus(ctx, req)
}

// waitForOfficeTerminalCompletionLockRefs polls
// officeTerminalCompletionLocks until the named task's ref count reaches
// want, mirroring waitForChildCompletionLockRefs in
// event_handlers_children_completed_test.go for the twin lock.
func waitForOfficeTerminalCompletionLockRefs(t *testing.T, svc *Service, taskID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.officeTerminalCompletionLocksMu.Lock()
		got := 0
		if entry := svc.officeTerminalCompletionLocks[taskID]; entry != nil {
			got = entry.refs
		}
		svc.officeTerminalCompletionLocksMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for office terminal completion lock refs = %d", want)
}

// TestMarkOfficeTaskCompleted_ConcurrentDeliveries_FireSideEffectsOnce is
// the review-round-3 MAJOR-2 regression test: two deliveries for the same
// Office task's terminal-step completion (e.g. a duplicate event) can both
// pass markTaskCompletedForTerminalStep's non-terminal check before either
// has written, since that check-then-act is not atomic across the Office
// seam call. Only one delivery may reach the seam and fire the completion
// side effects; the other must find the task already terminal and no-op.
func TestMarkOfficeTaskCompleted_ConcurrentDeliveries_FireSideEffectsOnce(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := waitStepWithChildrenCompletedTrigger()
	seedSession(t, repo, "parent-office", "parent-office-session", "step-wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-concurrent", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent-office",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	updater := &blockingOfficeStatusUpdater{
		inner:   &recordingOfficeStatusUpdater{repo: repo},
		started: make(chan struct{}),
		proceed: make(chan struct{}),
	}
	svc.SetOfficeTaskStatusUpdater(updater)
	events := &recordingTaskEventPublisher{}
	svc.SetTaskEventPublisher(events)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	// Delivery A: reaches the seam first and blocks inside it, holding the
	// per-task completion lock with the task row still IN_PROGRESS.
	doneA := make(chan struct{})
	go func() {
		defer close(doneA)
		svc.markTaskCompletedForTerminalStep(ctx, "office-concurrent", "child_done")
	}()
	select {
	case <-updater.started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for delivery A to reach the seam")
	}

	// Delivery B: a duplicate delivery for the same task. Its own
	// non-terminal check (under taskRuntimeStateMu) races in while the row
	// is still IN_PROGRESS, then it blocks behind A on the per-task lock.
	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		svc.markTaskCompletedForTerminalStep(ctx, "office-concurrent", "child_done")
	}()
	waitForOfficeTerminalCompletionLockRefs(t, svc, "office-concurrent", 2)

	close(updater.proceed)

	waitOrFatal := func(ch <-chan struct{}, who string) {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s to finish", who)
		}
	}
	waitOrFatal(doneA, "delivery A")
	waitOrFatal(doneB, "delivery B")
	// The winning delivery's parent on_children_completed transition launches
	// its own step's session prep on a detached goroutine
	// (launchProcessOnEnter); join it before reading events below, or the
	// read races that goroutine's writes to the shared, unsynchronized
	// recordingTaskEventPublisher.
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	if got := updater.calls.Load(); got != 1 {
		t.Fatalf("seam calls = %d, want exactly 1: a duplicate delivery must not re-run the status pipeline", got)
	}

	childUpdates := 0
	for _, id := range events.updated {
		if id == "office-concurrent" {
			childUpdates++
		}
	}
	if childUpdates != 1 {
		t.Fatalf("PublishTaskUpdated calls for office-concurrent = %d, want exactly 1 (both deliveries fired side effects)", childUpdates)
	}
	var childChanges []recordedStateChange
	for _, c := range events.stateChanges {
		if c.taskID == "office-concurrent" {
			childChanges = append(childChanges, c)
		}
	}
	if len(childChanges) != 1 {
		t.Fatalf("PublishTaskStateChanged calls for office-concurrent = %+v, want exactly 1", childChanges)
	}

	task, err := repo.GetTask(ctx, "office-concurrent")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED", task.State)
	}

	svc.officeTerminalCompletionLocksMu.Lock()
	_, exists := svc.officeTerminalCompletionLocks["office-concurrent"]
	svc.officeTerminalCompletionLocksMu.Unlock()
	if exists {
		t.Fatal("expected office terminal completion lock entry to be deleted after both deliveries exit")
	}
}
