package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/queue"
	"github.com/kandev/kandev/internal/orchestrator/scheduler"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestHandleTaskStateChanged_FiresBothChildrenCompletedAndDependency pins the
// v0.88 total-control plan's "需求 2: 父总控一次性消费 child terminal
// receipt 并派发既有下一步" guarantee. handleTaskStateChanged is the single
// entry point for terminal task events, and a child terminal receipt must
// not split its work across two phases: the children-completed rollup
// (which transitions the parent workflow) and the dependency resolution
// (which unblocks dependents) must both fire from the same call.
//
// Pre-fix risk: if either path returned early without doing its work, the
// dependent task could be stuck blocked-by-forever, or the parent could
// stay parked on a wait-for-children step. The two paths today are
// sequenced within handleTaskStateChanged (children_completed first,
// dependency dispatch second), each with their own atomicity guard
// (QueueAndInterruptForPeerMessage's cancelInFlight lock,
// deferred_launch_consume's claim+consume gate). The test seeds a parent
// at the children-wait step, a child task whose terminal receipt we fire,
// and a dependent carrying a start-when-unblocked intent whose
// dependency edge points at the child. After the call the parent must
// have moved to its post-children step AND the dependent must have
// launched exactly one session.
func TestHandleTaskStateChanged_FiresBothChildrenCompletedAndDependency(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	// Parent workflow with an OnChildrenCompleted transition on its first
	// step so the children-completed rollup has somewhere to go.
	const parentID = "parent-1"
	const parentSessionID = "parent-session-1"
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-wait"] = &wfmodels.WorkflowStep{
		ID:         "step-wait",
		WorkflowID: "wf1",
		Name:       "Wait for children",
		Position:   0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID:         "step-done",
		WorkflowID: "wf1",
		Name:       "Done",
		Position:   1,
	}
	seedSession(t, repo, parentID, parentSessionID, "step-wait")

	// A child that the parent is waiting on, plus a dependent whose
	// start-when-unblocked intent is the contract the dependency gate
	// must fire on.
	const childID = "child-1"
	const dependentID = "dependent-1"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: childID, WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Child",
		State: v1.TaskStateInProgress, ParentID: parentID,
		CreatedAt: now, UpdatedAt: now,
	}))
	// The orchestrator's allChildrenTerminal predicate reads each child's
	// DB state directly, so the child must already be terminal when the
	// receipt fires. Without this guard the children_completed path
	// silently no-ops because the DB row still says InProgress.
	require.NoError(t, repo.UpdateTaskState(ctx, childID, v1.TaskStateCompleted))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: dependentID, WorkspaceID: "ws1", Title: "Dependent",
		State:     v1.TaskStateCreated,
		CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":           "start",
				"agent_profile_id": "profile-1",
				"prompt":           "child terminal receipt test",
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}))

	// Spy on the agent launch so the test can assert the dependent's
	// start-when-unblocked intent fired exactly once, mirroring
	// TestDependencyResolutionLaunchesAnIntactIntent's positive control.
	counter := newLaunchCounter()
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			counter.record()
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-dependent-1"}, nil
		},
	}

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	// createEngineService wires the workflow engine but leaves the
	// scheduler nil. The dependency path (handleTaskDependenciesForTerminalState
	// → launchDeferredTask → LaunchSession → startTask → scheduler.GetTask)
	// dereferences s.scheduler — we must install a scheduler with a
	// mockTaskRepo so the dependent resolution does not panic. The
	// scheduler is a field the same-package test can write directly.
	if mockT, ok := svc.taskRepo.(*mockTaskRepo); ok {
		mockT.tasks[dependentID] = &v1.Task{
			ID: dependentID, Title: "Dependent", State: v1.TaskStateCreated,
		}
		svc.scheduler = scheduler.NewScheduler(
			queue.NewTaskQueue(100),
			executor.NewExecutor(agentMgr, repo, newTestLogger(), executor.ExecutorConfig{}),
			mockT,
			newTestLogger(),
			scheduler.SchedulerConfig{},
		)
	}
	// The child is the only predecessor of the dependent. The gate returns
	// unblocked the moment the child reaches a terminal state, so the
	// dependency dispatch path in handleTaskStateChanged must fire as
	// part of the same call.
	svc.SetTaskDependencyReader(&resolvedDependencyReader{dependents: []string{dependentID}})

	// Fire the child terminal receipt through the same single entry
	// point the bus delivers to.
	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
		TaskID:   childID,
		NewState: ptrToTaskStateCompleted(),
	})

	// children_completed path: the parent must have walked to its
	// post-children step in the same call, not the next event.
	parentAfter, err := repo.GetTask(ctx, parentID)
	require.NoError(t, err)
	if parentAfter.WorkflowStepID != "step-done" {
		t.Fatalf("children-completed path did not fire: parent.WorkflowStepID = %q, want %q",
			parentAfter.WorkflowStepID, "step-done")
	}

	// dependency path: the dependent's start-when-unblocked intent must
	// have been consumed in the same call, not deferred to the next
	// dependency event. A counter record is the precise signal —
	// awaiting a deadline would not distinguish "the intent fired during
	// this call" from "the intent fired later because a future event
	// unblocked it".
	if !counter.awaitLaunch(0) {
		t.Fatal("dependency dispatch path did not fire in the same call: counter is still at zero")
	}
	// The intent is single-shot: the same call must not launch a second
	// session on a future event because the gate already consumed the
	// intent.
	depAfter, err := repo.GetTask(ctx, dependentID)
	require.NoError(t, err)
	if _, still := depAfter.Metadata[models.MetaKeyDeferredLaunch]; still {
		t.Fatal("dependency dispatch did not consume the deferred-launch intent")
	}
	// A second deferred launch (e.g. a redundant dependency event)
	// must not spawn a second session replaying the same prompt.
	assert.Equal(t, 1, counter.count(),
		"the deferred-launch intent must fire exactly once per child terminal receipt")
}

// TestHandleTaskStateChanged_NonTerminalChildReceiptDoesNotFireParents
// pins the negative half: a child state change that is not terminal
// must be a no-op, even if the parent has a children-completed
// transition waiting. The children-completed path's predicate
// (models.IsTerminalTaskState) and the dependency path's gate
// (unblocked only on a terminal predecessor) both fail closed on
// non-terminal transitions, so this call leaves the parent at its
// current step and the dependent's deferred-launch intent intact.
func TestHandleTaskStateChanged_NonTerminalChildReceiptDoesNotFireParents(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	const parentID = "parent-2"
	const parentSessionID = "parent-session-2"
	stepGetter := newMockStepGetter()
	stepGetter.steps["step-wait"] = &wfmodels.WorkflowStep{
		ID: "step-wait", WorkflowID: "wf-2", Name: "Wait", Position: 0,
		Events: wfmodels.StepEvents{
			OnChildrenCompleted: []wfmodels.GenericAction{
				{Type: wfmodels.GenericActionMoveToNext},
			},
		},
	}
	stepGetter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-2", Name: "Done", Position: 1,
	}
	seedSession(t, repo, parentID, parentSessionID, "step-wait")

	const childID = "child-2"
	const dependentID = "dependent-2"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: childID, WorkspaceID: "ws1", WorkflowID: "wf-2", Title: "Child",
		State: v1.TaskStateInProgress, ParentID: parentID,
		CreatedAt: now, UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: dependentID, WorkspaceID: "ws1", Title: "Dependent",
		State:     v1.TaskStateCreated,
		CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":           "start",
				"agent_profile_id": "profile-1",
				"prompt":           "child non-terminal test",
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
	}))

	counter := newLaunchCounter()
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			counter.record()
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-dependent-2"}, nil
		},
	}

	svc := createEngineService(t, repo, stepGetter, agentMgr)
	svc.SetTaskDependencyReader(&resolvedDependencyReader{dependents: []string{dependentID}})

	svc.handleTaskStateChanged(ctx, watcher.TaskEventData{
		TaskID:   childID,
		NewState: ptrToTaskStateInProgress(),
	})

	parentAfter, err := repo.GetTask(ctx, parentID)
	require.NoError(t, err)
	if parentAfter.WorkflowStepID != "step-wait" {
		t.Fatalf("non-terminal child receipt must not advance parent, got %q", parentAfter.WorkflowStepID)
	}
	depAfter, err := repo.GetTask(ctx, dependentID)
	require.NoError(t, err)
	if _, still := depAfter.Metadata[models.MetaKeyDeferredLaunch]; !still {
		t.Fatal("non-terminal child receipt must not consume the deferred-launch intent")
	}
	if counter.count() != 0 {
		t.Fatalf("non-terminal child receipt must not launch, count=%d", counter.count())
	}
}

func ptrToTaskStateCompleted() *v1.TaskState {
	s := v1.TaskStateCompleted
	return &s
}

func ptrToTaskStateInProgress() *v1.TaskState {
	s := v1.TaskStateInProgress
	return &s
}
