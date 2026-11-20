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

func TestProcessOnChildrenCompleted_RequiresSuccessfulChildren(t *testing.T) {
	tests := []struct {
		name                string
		states              []v1.TaskState
		completeFailedChild bool
	}{
		{
			name:   "mixed_completed_failed",
			states: []v1.TaskState{v1.TaskStateCompleted, v1.TaskStateFailed},
		},
		{
			name:   "failed_failed",
			states: []v1.TaskState{v1.TaskStateFailed, v1.TaskStateFailed},
		},
		{
			name:   "completed_cancelled",
			states: []v1.TaskState{v1.TaskStateCompleted, v1.TaskStateCancelled},
		},
		{
			name:   "completed_unknown",
			states: []v1.TaskState{v1.TaskStateCompleted, v1.TaskState("UNKNOWN")},
		},
		{
			name:   "empty_child_set",
			states: nil,
		},
		{
			name:                "failed_child_later_completes",
			states:              []v1.TaskState{v1.TaskStateCompleted, v1.TaskStateFailed},
			completeFailedChild: true,
		},
		{
			name:   "all_completed",
			states: []v1.TaskState{v1.TaskStateCompleted, v1.TaskStateCompleted},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "parent", "parent-session", "step_wait")

			stepGetter := childrenCompletedStepGetter()

			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "parent", v1.TaskStateInProgress)
			svc := createEngineServiceWithTaskRepo(
				t, repo, stepGetter, taskRepo, &mockAgentManager{repoForExecutionLookup: repo},
			)
			onEnterDone := make(chan struct{}, 1)
			svc.onProcessOnEnterComplete = func() {
				select {
				case onEnterDone <- struct{}{}:
				default:
				}
			}

			now := time.Now().UTC()
			childIDs := []string{"child-a", "child-b"}
			for i, state := range tt.states {
				requireCreateTask(t, repo, &models.Task{
					ID: childIDs[i], WorkflowID: "wf1", Title: "Child",
					State: state, ParentID: "parent", CreatedAt: now, UpdatedAt: now,
				})
			}

			rows, err := repo.ListChildCompletionRows(ctx, "parent")
			if err != nil {
				t.Fatalf("list child completion rows: %v", err)
			}
			initialOperationID := childCompletionOperationID("parent", rows)

			if tt.name == "all_completed" {
				if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); !transitioned {
					t.Fatal("parent did not transition after every counted child completed successfully")
				}
				waitForChildrenCompletedOnEnter(t, onEnterDone)
				assertParentMovedToDone(t, ctx, repo)
				return
			}

			if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
				t.Fatal("parent transitioned before every counted child completed successfully")
			}
			assertParentWaitingForChildren(t, ctx, repo)
			assertChildCompletionOperationNotApplied(t, ctx, svc, initialOperationID)
			assertNoChildrenCompletedOnEnter(t, onEnterDone)

			if !tt.completeFailedChild {
				return
			}

			if err := repo.UpdateTaskState(ctx, "child-b", v1.TaskStateCompleted); err != nil {
				t.Fatalf("complete failed child: %v", err)
			}
			svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
				TaskID: "child-b", NewState: ptrToTaskStateCompleted(),
			})
			waitForChildrenCompletedOnEnter(t, onEnterDone)
			assertParentMovedToDone(t, ctx, repo)

			rows, err = repo.ListChildCompletionRows(ctx, "parent")
			if err != nil {
				t.Fatalf("list successful child completion rows: %v", err)
			}
			assertChildCompletionOperationApplied(t, ctx, svc, childCompletionOperationID("parent", rows))

			// A redelivery cannot consume the same successful child set twice.
			svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
				TaskID: "child-b", NewState: ptrToTaskStateCompleted(),
			})
			parent, err := repo.GetTask(ctx, "parent")
			if err != nil {
				t.Fatalf("load parent after duplicate child completion: %v", err)
			}
			if parent.WorkflowStepID != "step_done" {
				t.Fatalf("duplicate child completion moved parent to %q, want step_done", parent.WorkflowStepID)
			}
		})
	}
}

func TestProcessOnChildrenCompleted_ChildReadFailureDoesNotDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_wait")
	stepGetter := childrenCompletedStepGetter()
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{repoForExecutionLookup: repo})
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() { onEnterDone <- struct{}{} }
	requireCreateTask(t, repo, &models.Task{
		ID: "child-a", WorkflowID: "wf1", Title: "Child", State: v1.TaskStateCompleted,
		ParentID: "parent", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})
	if err := repo.DB().Close(); err != nil {
		t.Fatalf("close child repository to force readiness read failure: %v", err)
	}

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatal("parent transitioned after the child readiness read failed")
	}
	assertNoChildrenCompletedOnEnter(t, onEnterDone)
}

func TestProcessOnChildrenCompleted_DoesNotCreateSessionForParent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "parent", "step_wait")
	stepGetter := childrenCompletedStepGetter()
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{repoForExecutionLookup: repo})
	onEnterDone := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() { onEnterDone <- struct{}{} }
	requireCreateTask(t, repo, &models.Task{
		ID: "child-a", WorkflowID: "wf1", Title: "Child", State: v1.TaskStateCompleted,
		ParentID: "parent", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	})

	if transitioned := svc.processOnChildrenCompleted(ctx, "parent"); transitioned {
		t.Fatal("parent without a session transitioned on child completion")
	}
	assertNoChildrenCompletedOnEnter(t, onEnterDone)
	sessions, err := repo.ListTaskSessions(ctx, "parent")
	if err != nil {
		t.Fatalf("list parent sessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("created %d parent sessions for child completion, want none", len(sessions))
	}
}

func childrenCompletedStepGetter() *mockStepGetter {
	stepGetter := newMockStepGetter()
	stepGetter.steps["step_wait"] = &wfmodels.WorkflowStep{
		ID: "step_wait", WorkflowID: "wf1", Position: 0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}},
		},
	}
	stepGetter.steps["step_done"] = &wfmodels.WorkflowStep{ID: "step_done", WorkflowID: "wf1", Position: 1}
	stepGetter.steps["step_done"].Events.OnChildrenCompleted = []wfmodels.GenericAction{{Type: wfmodels.GenericActionMoveToNext}}
	stepGetter.steps["step_final"] = &wfmodels.WorkflowStep{ID: "step_final", WorkflowID: "wf1", Position: 2}
	return stepGetter
}

func assertParentWaitingForChildren(t *testing.T, ctx context.Context, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
}) {
	t.Helper()
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step_wait" {
		t.Fatalf("parent step = %q before child success, want step_wait", parent.WorkflowStepID)
	}
}

func assertParentMovedToDone(t *testing.T, ctx context.Context, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
}) {
	t.Helper()
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent after successful children: %v", err)
	}
	if parent.WorkflowStepID != "step_done" {
		t.Fatalf("parent step after successful children = %q, want step_done", parent.WorkflowStepID)
	}
}

func assertChildCompletionOperationNotApplied(t *testing.T, ctx context.Context, svc *Service, operationID string) {
	t.Helper()
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil {
		t.Fatalf("check blocked child completion operation: %v", err)
	}
	if applied {
		t.Fatal("blocked child completion operation was marked applied")
	}
}

func assertChildCompletionOperationApplied(t *testing.T, ctx context.Context, svc *Service, operationID string) {
	t.Helper()
	applied, err := svc.workflowStore.IsOperationApplied(ctx, operationID)
	if err != nil {
		t.Fatalf("check successful child completion operation: %v", err)
	}
	if !applied {
		t.Fatal("successful child completion operation was not marked applied")
	}
}

func assertNoChildrenCompletedOnEnter(t *testing.T, onEnterDone <-chan struct{}) {
	t.Helper()
	select {
	case <-onEnterDone:
		t.Fatal("parent on-enter callback ran before every counted child completed successfully")
	default:
	}
}
