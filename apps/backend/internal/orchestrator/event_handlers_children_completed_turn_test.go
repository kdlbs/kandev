package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestOnChildrenCompleted_LastChildCompletesInItsOwnTurn pins issue #4553:
// the last child reaches its complete_task_on_enter step through its own
// on_turn_complete transition, so the parent's on_children_completed move is
// evaluated with the child's turn context. The child's workflow delivery
// effect must not be reused for the parent's commit — its key was already
// consumed by the child's own move, which used to make the parent commit a
// silent no-op while the target step's lifecycle still ran.
func TestOnChildrenCompleted_LastChildCompletesInItsOwnTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "parent", "parent-session", "step_await")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_await"] = &wfmodels.WorkflowStep{
		ID: "step_await", WorkflowID: "wf1", Name: "Await children", Position: 0,
		Events: wfmodels.StepEvents{OnChildrenCompleted: []wfmodels.GenericAction{{
			Type:   wfmodels.GenericActionMoveToStep,
			Config: map[string]interface{}{"step_id": "step_inspect"},
		}}},
	}
	stepGetter.steps["step_inspect"] = &wfmodels.WorkflowStep{
		ID: "step_inspect", WorkflowID: "wf1", Name: "Inspect results", Position: 1,
	}
	stepGetter.steps["child_work"] = &wfmodels.WorkflowStep{
		ID: "child_work", WorkflowID: "wf-child", Name: "Child work", Position: 0,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToNext,
		}}},
	}
	stepGetter.steps["child_done"] = &wfmodels.WorkflowStep{
		ID: "child_done", WorkflowID: "wf-child", Name: "Child done", Position: 1,
		CompleteTaskOnEnter: true,
	}

	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createEngineService(t, repo, stepGetter, agentMgr)
	onEnterDone := make(chan struct{}, 4)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterDone <- struct{}{}:
		default:
		}
	}

	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "child", WorkspaceID: "ws1", WorkflowID: "wf-child", WorkflowStepID: "child_work",
		Title: "Child", State: v1.TaskStateInProgress, ParentID: "parent",
		CreatedAt: now, UpdatedAt: now,
	})
	childSession := &models.TaskSession{
		ID: "child-session", TaskID: "child", State: models.TaskSessionStateRunning,
		StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, childSession); err != nil {
		t.Fatalf("create child session: %v", err)
	}

	effect := workflowEffectForTurn("child-turn-1")
	effect.TaskID, effect.SessionID = "child", "child-session"
	if !svc.processOnTurnCompleteViaEngineWithCause(
		withWorkflowEffect(ctx, effect), "child", childSession, turnCompletionCauseAgentTurn,
	) {
		t.Fatal("expected the child's turn to move it to child_done")
	}
	waitForChildrenCompletedOnEnter(t, onEnterDone)

	child, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatalf("load child: %v", err)
	}
	if child.WorkflowStepID != "child_done" || child.State != v1.TaskStateCompleted {
		t.Fatalf("child = step %q state %q, want child_done/COMPLETED", child.WorkflowStepID, child.State)
	}
	parent, err := repo.GetTask(ctx, "parent")
	if err != nil {
		t.Fatalf("load parent: %v", err)
	}
	if parent.WorkflowStepID != "step_inspect" {
		t.Fatalf("parent.WorkflowStepID = %q, want step_inspect", parent.WorkflowStepID)
	}
}

// TestApplyEngineTransition_NotAppliedCommitSkipsLifecycle pins the second
// half of issue #4553: when the transition commit is refused because the
// workflow delivery effect in ctx was already consumed, the lifecycle must
// report the move as not applied instead of recording history and running
// the target step's on_enter.
func TestApplyEngineTransition_NotAppliedCommitSkipsLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "task-session", "step_from")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step_from"] = &wfmodels.WorkflowStep{
		ID: "step_from", WorkflowID: "wf1", Name: "From", Position: 0,
	}
	stepGetter.steps["step_to"] = &wfmodels.WorkflowStep{
		ID: "step_to", WorkflowID: "wf1", Name: "To", Position: 1,
	}
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{repoForExecutionLookup: repo})
	onEnterRan := make(chan struct{}, 1)
	svc.onProcessOnEnterComplete = func() {
		select {
		case onEnterRan <- struct{}{}:
		default:
		}
	}

	// Consume the effect key with an unrelated committed move first.
	effect := workflowEffectForTurn("consumed-turn")
	effect.TaskID, effect.SessionID = "other", "other-session"
	now := time.Now().UTC()
	requireCreateTask(t, repo, &models.Task{
		ID: "other", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step_from",
		Title: "Other", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	})
	other, err := repo.GetTask(ctx, "other")
	if err != nil {
		t.Fatalf("load other: %v", err)
	}
	other.WorkflowStepID = "step_to"
	if _, applied, err := repo.UpdateTaskWithWorkflowStepAdmissionAndEffect(
		ctx, other, "step_from", "step_to", 0, effect,
	); err != nil || !applied {
		t.Fatalf("consume effect: applied=%v err=%v", applied, err)
	}

	session, err := repo.GetTaskSession(ctx, "task-session")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if svc.applyEngineTransitionWithMode(
		withWorkflowEffect(ctx, effect), "task", session,
		engine.HandleResult{Transitioned: true, FromStepID: "step_from", ToStepID: "step_to"},
		engine.TriggerOnChildrenCompleted, "", transitionLifecycleWithOnEnter,
	) {
		t.Fatal("expected a refused commit to report the transition as not applied")
	}

	task, err := repo.GetTask(ctx, "task")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.WorkflowStepID != "step_from" {
		t.Fatalf("task.WorkflowStepID = %q, want step_from", task.WorkflowStepID)
	}
	transitions, err := repo.ListTaskStepTransitions(ctx, "task", 0, 50)
	if err != nil {
		t.Fatalf("list step transitions: %v", err)
	}
	for _, tr := range transitions {
		if tr.ToWorkflowStepID != nil && *tr.ToWorkflowStepID == "step_to" {
			t.Fatalf("unexpected step transition row for a refused commit: %+v", tr)
		}
	}
	select {
	case <-onEnterRan:
		t.Fatal("target step on_enter ran for a refused commit")
	case <-time.After(200 * time.Millisecond):
	}
}
