package orchestrator

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
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
	repo             *sqliterepo.Repository
	newState         v1.TaskState // state to persist; defaults to COMPLETED
	err              error        // returned after the write, mirrors UpdateTaskStatus's contract
	mu               sync.Mutex
	calls            []dashboard.TaskStatusUpdateRequest
	committedChanges []models.TaskStateChange
	onCommit         func(context.Context, models.TaskStateChange)
}

func (u *recordingOfficeStatusUpdater) UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error {
	u.mu.Lock()
	u.calls = append(u.calls, req)
	u.mu.Unlock()
	state := v1.TaskStateCompleted
	if u.newState != "" {
		state = u.newState
	}
	expectedState := v1.TaskState(req.ExpectedCurrentState)
	write := models.TaskStateWrite{
		State:                  state,
		ExpectedWorkflowStepID: &req.ExpectedWorkflowStepID,
		ExpectedState:          &expectedState,
	}
	change, err := u.repo.CommitTaskState(ctx, req.TaskID, write)
	if err != nil {
		return err
	}
	if !change.Matched {
		return dashboard.ErrAutomaticCompletionStale
	}
	if change.Changed {
		u.mu.Lock()
		u.committedChanges = append(u.committedChanges, change)
		u.mu.Unlock()
		if u.onCommit != nil {
			u.onCommit(ctx, change)
		}
	}
	return u.err
}

func (u *recordingOfficeStatusUpdater) committed() []models.TaskStateChange {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]models.TaskStateChange(nil), u.committedChanges...)
}

func routeCommittedStateToOrchestrator(ctx context.Context, svc *Service, change models.TaskStateChange) {
	if change.Task == nil {
		return
	}
	oldState, newState := change.OldState, change.Task.State
	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
		TaskID:   change.Task.ID,
		Task:     change.Task.ToAPI(),
		OldState: &oldState,
		NewState: &newState,
	})
}

type recordingTaskEventPublisher struct {
	mu           sync.Mutex
	updated      []string
	stateChanges []recordedStateChange
}

type recordedStateChange struct {
	taskID   string
	oldState v1.TaskState
	newState v1.TaskState
}

func (r *recordingTaskEventPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	if task == nil {
		return
	}
	r.mu.Lock()
	r.updated = append(r.updated, task.ID)
	r.mu.Unlock()
}

func (r *recordingTaskEventPublisher) PublishTaskStateChanged(_ context.Context, task *models.Task, oldState v1.TaskState) {
	if task == nil {
		return
	}
	r.mu.Lock()
	r.stateChanges = append(r.stateChanges, recordedStateChange{taskID: task.ID, oldState: oldState, newState: task.State})
	r.mu.Unlock()
}

func (r *recordingTaskEventPublisher) PublishTaskActivityIfChanged(context.Context, string) {}

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

type staleOfficeTaskReloadRepo struct {
	*sqliterepo.Repository
	loads int
}

func (r *staleOfficeTaskReloadRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	task, err := r.Repository.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	r.loads++
	if r.loads == 2 {
		task.WorkflowStepID = "child_work"
	}
	return task, nil
}

func TestMarkOfficeTaskCompleted_RechecksStepAfterPerTaskLock(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "parent-recheck", "parent-recheck-session", "step_wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, baseRepo, &models.Task{
		ID: "office-recheck", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Office task", State: v1.TaskStateInProgress, ParentID: "parent-recheck",
		CreatedAt: now, UpdatedAt: now,
	})

	repo := &staleOfficeTaskReloadRepo{Repository: baseRepo}
	svc := &Service{logger: testLogger(), repo: repo}
	updater := &recordingOfficeStatusUpdater{repo: baseRepo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-recheck", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none after the status reload observes a new step", updater.calls)
	}
	task, err := baseRepo.GetTask(ctx, "office-recheck")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want unchanged IN_PROGRESS", task.State)
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

// TestMarkOfficeTaskCompleted_NoApprovers_FiresTerminalSideEffects verifies
// that the committed state event drives the parent's children-completed
// transition after the Office status pipeline completes a task.
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
	updater := &recordingOfficeStatusUpdater{repo: repo}
	updater.onCommit = func(ctx context.Context, change models.TaskStateChange) {
		routeCommittedStateToOrchestrator(ctx, svc, change)
	}
	svc.SetOfficeTaskStatusUpdater(updater)
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
	// before reading the parent state.
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	changes := updater.committed()
	if len(changes) != 1 || changes[0].OldState != v1.TaskStateInProgress || changes[0].Task.State != v1.TaskStateCompleted {
		t.Fatalf("committed state changes = %+v, want one IN_PROGRESS -> COMPLETED receipt", changes)
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
// verifies that a gate redirect to REVIEW does not drive terminal handling.
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
	updater := &recordingOfficeStatusUpdater{
		repo:     repo,
		newState: v1.TaskStateReview,
		err:      &dashboard.ApprovalsPendingError{Pending: []string{"agent-A"}},
	}
	updater.onCommit = func(ctx context.Context, change models.TaskStateChange) {
		routeCommittedStateToOrchestrator(ctx, svc, change)
	}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-pending-fx", "child_done")

	changes := updater.committed()
	if len(changes) != 1 || changes[0].Task.State != v1.TaskStateReview {
		t.Fatalf("committed state changes = %+v, want one redirect to REVIEW", changes)
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

// blockingOfficeStatusUpdater holds the first request while a second guarded
// state write races to commit.
type blockingOfficeStatusUpdater struct {
	inner   *recordingOfficeStatusUpdater
	started chan struct{}
	second  chan struct{}
	proceed chan struct{}
	calls   atomic.Int32
}

func (u *blockingOfficeStatusUpdater) UpdateTaskStatus(ctx context.Context, req dashboard.TaskStatusUpdateRequest) error {
	switch u.calls.Add(1) {
	case 1:
		close(u.started)
		<-u.proceed
	case 2:
		close(u.second)
	}
	return u.inner.UpdateTaskStatus(ctx, req)
}

// TestMarkOfficeTaskCompleted_ConcurrentDeliveries_FireSideEffectsOnce verifies
// that repository guards permit one completion receipt when two deliveries
// race from the same observed state.
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
		second:  make(chan struct{}),
		proceed: make(chan struct{}),
	}
	updater.inner.onCommit = func(ctx context.Context, change models.TaskStateChange) {
		routeCommittedStateToOrchestrator(ctx, svc, change)
	}
	svc.SetOfficeTaskStatusUpdater(updater)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	// Delivery A reaches the seam first and pauses before its guarded write.
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

	// Delivery B observes the same state and reaches the repository while A is
	// still paused.
	doneB := make(chan struct{})
	go func() {
		defer close(doneB)
		svc.markTaskCompletedForTerminalStep(ctx, "office-concurrent", "child_done")
	}()
	select {
	case <-updater.second:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for delivery B to reach the status seam")
	}

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
	// The winning delivery's parent transition launches session prep on a
	// detached goroutine. Join it before reading the parent state.
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	if got := updater.calls.Load(); got != 2 {
		t.Fatalf("seam calls = %d, want both deliveries to reach the guarded status write", got)
	}
	if changes := updater.inner.committed(); len(changes) != 1 {
		t.Fatalf("committed state changes = %+v, want exactly one completion receipt", changes)
	}

	task, err := repo.GetTask(ctx, "office-concurrent")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("state = %q, want COMPLETED", task.State)
	}

}

// staleOfficeStatusUpdater models a dashboard write that lost its state/step
// compare-and-set: it returns ErrAutomaticCompletionStale without writing.
type staleOfficeStatusUpdater struct {
	calls int
}

func (u *staleOfficeStatusUpdater) UpdateTaskStatus(context.Context, dashboard.TaskStatusUpdateRequest) error {
	u.calls++
	return dashboard.ErrAutomaticCompletionStale
}

// TestMarkOfficeTaskCompleted_Blocked_SkipsSeamAndSideEffects is the blocked
// terminal-step regression for #4889: a BLOCKED Office task must not reach the
// status seam, publish task events, or drive the parent's children-completed
// trigger.
func TestMarkOfficeTaskCompleted_Blocked_SkipsSeamAndSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := waitStepWithChildrenCompletedTrigger()
	seedSession(t, repo, "parent-office", "parent-office-session", "step-wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-blocked-fx", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Blocked Office task", State: v1.TaskStateBlocked, ParentID: "parent-office",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	updater := &recordingOfficeStatusUpdater{repo: repo}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-blocked-fx", "child_done")

	if len(updater.calls) != 0 {
		t.Fatalf("seam calls = %+v, want none for a BLOCKED task", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-blocked-fx")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateBlocked || task.WorkflowStepID != "child_done" {
		t.Fatalf("task = step %q state %q, want child_done/BLOCKED", task.WorkflowStepID, task.State)
	}
	parent, err := repo.GetTask(ctx, "parent-office")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step-wait" {
		t.Fatalf("parent.WorkflowStepID = %q, want unchanged step-wait", parent.WorkflowStepID)
	}
}

// TestMarkOfficeTaskCompleted_StaleAutomaticWrite_SkipsSideEffects pins the
// stale-completion suppression added by #4889: ErrAutomaticCompletionStale is
// a no-op, not a failure, and must not publish completion side effects.
func TestMarkOfficeTaskCompleted_StaleAutomaticWrite_SkipsSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	stepGetter := waitStepWithChildrenCompletedTrigger()
	seedSession(t, repo, "parent-office", "parent-office-session", "step-wait")

	now := time.Now().UTC()
	requireCreateOfficeTask(t, repo, &models.Task{
		ID: "office-stale-fx", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Stale Office task", State: v1.TaskStateInProgress, ParentID: "parent-office",
		CreatedAt: now, UpdatedAt: now,
	})

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	updater := &staleOfficeStatusUpdater{}
	svc.SetOfficeTaskStatusUpdater(updater)

	svc.markTaskCompletedForTerminalStep(ctx, "office-stale-fx", "child_done")

	if updater.calls != 1 {
		t.Fatalf("seam calls = %d, want 1", updater.calls)
	}
	task, err := repo.GetTask(ctx, "office-stale-fx")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.State != v1.TaskStateInProgress {
		t.Fatalf("state = %q, want IN_PROGRESS unchanged", task.State)
	}
	parent, err := repo.GetTask(ctx, "parent-office")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step-wait" {
		t.Fatalf("parent.WorkflowStepID = %q, want unchanged step-wait", parent.WorkflowStepID)
	}
}
