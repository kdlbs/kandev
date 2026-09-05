package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestProcessOnChildrenCompleted_TransitionsParentWhenAllActiveChildrenTerminal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID:         "step_done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	for _, child := range []*models.Task{
		{
			ID:         "child-complete",
			WorkflowID: "wf1",
			Title:      "Complete child",
			State:      v1.TaskStateCompleted,
			ParentID:   "parent",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         "child-open",
			WorkflowID: "wf1",
			Title:      "Open child",
			State:      v1.TaskStateInProgress,
			ParentID:   "parent",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	} {
		if err := repo.CreateTask(ctx, child); err != nil {
			t.Fatalf("create child %s: %v", child.ID, err)
		}
	}

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatalf("expected mixed terminal/non-terminal children not to transition")
	}
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step_wait" {
		t.Fatalf("expected parent to stay on step_wait, got %q", parent.WorkflowStepID)
	}

	if err := repo.UpdateTaskState(ctx, "child-open", v1.TaskStateCompleted); err != nil {
		t.Fatalf("complete child-open: %v", err)
	}
	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
		t.Fatalf("expected all-terminal active children to transition parent")
	}
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	parent, err = repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after transition: %v", err)
	}
	if parent.WorkflowStepID != "step_done" {
		t.Fatalf("expected parent to move to step_done, got %q", parent.WorkflowStepID)
	}
	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatalf("expected duplicate all-terminal evaluation not to transition parent again")
	}
	parent, err = repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after duplicate evaluation: %v", err)
	}
	if parent.WorkflowStepID != "step_done" {
		t.Fatalf("expected parent to remain on step_done after duplicate evaluation, got %q", parent.WorkflowStepID)
	}
}

// TestProcessOnChildrenCompleted_StillIdempotentOnRedelivery is the AC-EO-16
// pin for docs/specs/workflow-evaluate-only-operation-marking: this trigger
// passes no OperationID through the engine and keeps bracketing its own
// marker by hand (childCompletionAlreadyApplied / markChildCompletionApplied),
// unchanged by that spec's EvaluateOnly deferred-mark contract.
//
// step_done also declares an OnChildrenCompleted action (unlike the sibling
// TransitionsParentWhenAllActiveChildrenTerminal test's terminal step_done,
// which has none) so the marker check is load-bearing: without it, the
// unchanged, still-all-terminal child rows would let a redelivery re-evaluate
// step_done's own trigger and advance the parent a second time, to
// step_final. A mutation that deletes childCompletionAlreadyApplied's guard
// makes this test observe exactly that second move.
func TestProcessOnChildrenCompleted_StillIdempotentOnRedelivery(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID: "step_wait", WorkflowID: "wf1", Name: "Wait for Subtasks", Position: 0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID: "step_done", WorkflowID: "wf1", Name: "Done", Position: 1,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}},
		},
	}
	stepGetter.steps["step_final"] = &wfmodels.WorkflowStep{ID: "step_final", WorkflowID: "wf1", Name: "Final", Position: 2}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	seedMockTaskState(svc.taskRepo.(*mockTaskRepo), "parent", v1.TaskStateInProgress)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child-complete", WorkflowID: "wf1", Title: "Complete child",
		State: v1.TaskStateCompleted, ParentID: "parent", CreatedAt: now, UpdatedAt: now,
	})

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
		t.Fatalf("expected the first delivery to transition the parent")
	}
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	rows, err := repo.ListChildCompletionRows(ctx, "parent")
	if err != nil {
		t.Fatalf("list child completion rows: %v", err)
	}
	operationID := childCompletionOperationID("parent", rows)
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil || !applied {
		t.Fatalf("IsOperationApplied = %v, %v, want true, nil (on_children_completed marks its own "+
			"operation id through its own bracket, unaffected by this spec's EvaluateOnly change)", applied, err)
	}

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatalf("expected redelivery of the same completion to be idempotent (no second transition)")
	}
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after redelivery: %v", err)
	}
	if parent.WorkflowStepID != "step_done" {
		t.Fatalf("WorkflowStepID = %q after redelivery, want step_done unchanged (a broken guard would "+
			"re-fire step_done's own OnChildrenCompleted action and advance to step_final)", parent.WorkflowStepID)
	}
}

func TestProcessOnChildrenCompleted_TreatsTerminalStepAsCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step_parent_done"] = &wfmodels.WorkflowStep{
		ID:         "step_parent_done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}
	stepGetter.steps["child_done"] = &wfmodels.WorkflowStep{
		ID:         "child_done",
		WorkflowID: "wf-child",
		Name:       "Done",
		Position:   1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child-terminal-step", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Child in Done", State: v1.TaskStateReview, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
		t.Fatalf("expected child in terminal workflow step to transition parent")
	}
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after transition: %v", err)
	}
	if parent.WorkflowStepID != "step_parent_done" {
		t.Fatalf("expected parent to move to step_parent_done, got %q", parent.WorkflowStepID)
	}
}

func TestHandleTaskMovedToTerminalStepProcessesParentChildrenCompleted(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step_parent_done"] = &wfmodels.WorkflowStep{
		ID:         "step_parent_done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}
	stepGetter.steps["child_work"] = &wfmodels.WorkflowStep{
		ID:         "child_work",
		WorkflowID: "wf-child",
		Name:       "Work",
		Position:   0,
	}
	stepGetter.steps["child_done"] = &wfmodels.WorkflowStep{
		ID:         "child_done",
		WorkflowID: "wf-child",
		Name:       "Done",
		Position:   1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child-terminal-move", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Child moved to Done", State: v1.TaskStateReview, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc.handleTaskMoved(ctx, watcher.TaskMovedEventData{
		TaskID:     "child-terminal-move",
		FromStepID: "child_work",
		ToStepID:   "child_done",
		WorkflowID: "wf-child",
	})
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	child, err := repo.GetTask(ctx, "child-terminal-move")
	if err != nil {
		t.Fatalf("load child after terminal move: %v", err)
	}
	if child.State != v1.TaskStateCompleted {
		t.Fatalf("expected child state COMPLETED after terminal move, got %q", child.State)
	}

	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after child move: %v", err)
	}
	if parent.WorkflowStepID != "step_parent_done" {
		t.Fatalf("expected parent to move to step_parent_done, got %q", parent.WorkflowStepID)
	}
}

func TestTerminalStepMoveIgnoresStaleTaskStep(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
	}
	stepGetter.steps["child_work"] = &wfmodels.WorkflowStep{
		ID:         "child_work",
		WorkflowID: "wf-child",
		Name:       "Work",
		Position:   0,
	}
	stepGetter.steps["child_done"] = &wfmodels.WorkflowStep{
		ID:         "child_done",
		WorkflowID: "wf-child",
		Name:       "Done",
		Position:   1,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child-stale-terminal-move", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_work",
		Title: "Child already moved out", State: v1.TaskStateReview, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc.processParentChildrenCompletedForTerminalStepMove(ctx, "child-stale-terminal-move", "child_done")

	child, err := repo.GetTask(ctx, "child-stale-terminal-move")
	if err != nil {
		t.Fatalf("load child after stale move: %v", err)
	}
	if child.State != v1.TaskStateReview {
		t.Fatalf("expected child to remain REVIEW after stale terminal move, got %q", child.State)
	}
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after stale move: %v", err)
	}
	if parent.WorkflowStepID != "step_wait" {
		t.Fatalf("expected parent to remain on step_wait, got %q", parent.WorkflowStepID)
	}
}

func TestTerminalStepMoveSkipsAlreadyTerminalTaskState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID:         "step_wait",
		WorkflowID: "wf1",
		Name:       "Wait for Subtasks",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{
		ID:         "step_done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}
	stepGetter.steps["child_done"] = &wfmodels.WorkflowStep{
		ID:         "child_done",
		WorkflowID: "wf-child",
		Name:       "Done",
		Position:   0,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child-already-terminal", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_done",
		Title: "Child already terminal", State: v1.TaskStateCompleted, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})

	svc.processParentChildrenCompletedForTerminalStepMove(ctx, "child-already-terminal", "child_done")

	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after redundant terminal move: %v", err)
	}
	if parent.WorkflowStepID != "step_wait" {
		t.Fatalf("expected parent to remain on step_wait after redundant terminal move, got %q", parent.WorkflowStepID)
	}
}

func requireCreateTask(t *testing.T, repo interface {
	CreateTask(context.Context, *models.Task) error
}, task *models.Task) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task %s: %v", task.ID, err)
	}
}

func waitForChildrenCompletedOnEnter(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for processOnEnter goroutine")
	}
}

func TestLockChildCompletionOperationKeepsEntryUntilWaitersExit(t *testing.T) {
	svc := &Service{}
	unlockFirst := svc.lockChildCompletionOperation("op")

	secondAcquired := make(chan struct{})
	releaseSecond := make(chan struct{})
	done := make(chan struct{})
	go func() {
		unlockSecond := svc.lockChildCompletionOperation("op")
		close(secondAcquired)
		<-releaseSecond
		unlockSecond()
		close(done)
	}()

	waitForChildCompletionLockRefs(t, svc, "op", 2)
	unlockFirst()
	select {
	case <-secondAcquired:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second lock holder")
	}
	waitForChildCompletionLockRefs(t, svc, "op", 1)
	close(releaseSecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second lock release")
	}

	svc.childCompletionLocksMu.Lock()
	_, exists := svc.childCompletionLocks["op"]
	svc.childCompletionLocksMu.Unlock()
	if exists {
		t.Fatal("expected lock entry to be deleted after all holders exit")
	}
}

func waitForChildCompletionLockRefs(t *testing.T, svc *Service, operationID string, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.childCompletionLocksMu.Lock()
		got := 0
		if entry := svc.childCompletionLocks[operationID]; entry != nil {
			got = entry.refs
		}
		svc.childCompletionLocksMu.Unlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for lock refs %d", want)
}
