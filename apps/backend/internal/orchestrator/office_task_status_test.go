package orchestrator

import (
	"context"
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

	svc.markTaskCompletedForTerminalStep(ctx, "office-done-fx", "child_done")

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
