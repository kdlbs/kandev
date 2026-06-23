package orchestrator

import (
	"context"
	"testing"
	"time"

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
