package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestService_MoveTaskPromotesMovedTaskFromFeederAndRefreshesResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-feeder-move", "wf-source", "step-c", nil)

	result, err := svc.MoveTask(ctx, "task-feeder-move", "wf-source", "step-a", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	if result.Task.WorkflowStepID != "step-b" {
		t.Fatalf("returned task step = %q, want step-b", result.Task.WorkflowStepID)
	}
	if result.WorkflowStep == nil || result.WorkflowStep.ID != "step-b" {
		t.Fatalf("returned workflow step = %+v, want step-b", result.WorkflowStep)
	}

	stored, err := repo.GetTask(ctx, "task-feeder-move")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-b" {
		t.Fatalf("stored task step = %q, want step-b", stored.WorkflowStepID)
	}
	if !stored.WIPAdmitted || stored.QueuedForStepID != "" {
		t.Fatalf("stored task admission = admitted:%v queued_for:%q, want admitted in step-b", stored.WIPAdmitted, stored.QueuedForStepID)
	}
}

func TestService_MoveTaskWithActiveSessionDefersFeederPullUntilLifecycleCompletes(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-feeder-move-session", "wf-source", "step-c", nil)
	createMoveSession(t, ctx, repo, "session-feeder-move", "task-feeder-move-session", models.TaskSessionStateWaitingForInput, models.ReviewStatusNone)

	result, err := svc.MoveTask(ctx, "task-feeder-move-session", "wf-source", "step-a", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if result.Task.WorkflowStepID != "step-a" {
		t.Fatalf("returned task step = %q, want step-a until lifecycle completion", result.Task.WorkflowStepID)
	}

	stored, err := repo.GetTask(ctx, "task-feeder-move-session")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-a" {
		t.Fatalf("stored task step = %q, want step-a until lifecycle completion", stored.WorkflowStepID)
	}
	if _, pending := stored.Metadata[models.MetaKeyManualMoveLifecyclePending]; !pending {
		t.Fatalf("stored move metadata = %#v, want durable lifecycle barrier", stored.Metadata)
	}
}

func TestService_MoveTaskLeavesFeederTaskWhenPullTargetIsFull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-move-into-full-feeder", "wf-source", "step-c", nil)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-full-occupant", WorkspaceID: "ws-1", WorkflowID: "wf-source", WorkflowStepID: "step-b",
		Title: "Full occupant", State: v1.TaskStateTODO, WIPAdmitted: true,
	}); err != nil {
		t.Fatalf("CreateTask(full occupant): %v", err)
	}

	if _, err := svc.MoveTask(ctx, "task-move-into-full-feeder", "wf-source", "step-a", 0); err != nil {
		t.Fatalf("MoveTask into feeder: %v", err)
	}
	queued, err := repo.GetTask(ctx, "task-move-into-full-feeder")
	if err != nil {
		t.Fatalf("GetTask(feeder task): %v", err)
	}
	if queued.WorkflowStepID != "step-a" {
		t.Fatalf("feeder task step = %q, want step-a while target is full", queued.WorkflowStepID)
	}

	if _, err := svc.MoveTask(ctx, "task-full-occupant", "wf-source", "step-c", 0); err != nil {
		t.Fatalf("MoveTask vacating full target: %v", err)
	}
	promoted, err := repo.GetTask(ctx, "task-move-into-full-feeder")
	if err != nil {
		t.Fatalf("GetTask(promoted task): %v", err)
	}
	if promoted.WorkflowStepID != "step-b" {
		t.Fatalf("promoted task step = %q, want step-b after vacancy", promoted.WorkflowStepID)
	}
}

func TestService_MoveTaskSameStepReorderDoesNotWakeFeederPull(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-same-step-feeder", "wf-source", "step-a", nil)

	if _, err := svc.MoveTask(ctx, "task-same-step-feeder", "wf-source", "step-a", 1); err != nil {
		t.Fatalf("MoveTask same-step reorder: %v", err)
	}
	stored, err := repo.GetTask(ctx, "task-same-step-feeder")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-a" {
		t.Fatalf("same-step task step = %q, want step-a", stored.WorkflowStepID)
	}
}

func TestService_MoveTaskFeederPullPublishesMovesInCausalOrder(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-causal-move", "wf-source", "step-c", nil)
	eventBus.ClearEvents()

	if _, err := svc.MoveTask(ctx, "task-causal-move", "wf-source", "step-a", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	var movedSteps [][2]string
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskMoved {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok || data["task_id"] != "task-causal-move" {
			continue
		}
		from, _ := data["from_step_id"].(string)
		to, _ := data["to_step_id"].(string)
		movedSteps = append(movedSteps, [2]string{from, to})
	}
	if len(movedSteps) != 2 {
		t.Fatalf("task.moved sequence = %v, want C->A then A->B", movedSteps)
	}
	if movedSteps[0] != [2]string{"step-c", "step-a"} || movedSteps[1] != [2]string{"step-a", "step-b"} {
		t.Fatalf("task.moved sequence = %v, want C->A then A->B", movedSteps)
	}
}

type failAfterMoveRefreshTaskRepository struct {
	*sqliterepo.Repository
	failRefresh atomic.Bool
}

func (r *failAfterMoveRefreshTaskRepository) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if r.failRefresh.Load() {
		return nil, errors.New("task refresh failed")
	}
	return r.Repository.GetTask(ctx, id)
}

func (r *failAfterMoveRefreshTaskRepository) UpdateTaskWithWorkflowStepAdmission(
	ctx context.Context,
	task *models.Task,
	targetStepID string,
	limit int,
) (bool, error) {
	admitted, err := r.Repository.UpdateTaskWithWorkflowStepAdmission(ctx, task, targetStepID, limit)
	if err == nil {
		r.failRefresh.Store(true)
	}
	return admitted, err
}

func (r *failAfterMoveRefreshTaskRepository) UpdateTaskWithWorkflowStepAdmissionAndState(
	ctx context.Context,
	task *models.Task,
	targetStepID string,
	limit int,
	admittedState *v1.TaskState,
	queueExitPending bool,
	expectedWorkflowID string,
) (bool, error) {
	admitted, err := r.Repository.UpdateTaskWithWorkflowStepAdmissionAndState(
		ctx, task, targetStepID, limit, admittedState, queueExitPending, expectedWorkflowID,
	)
	if err == nil {
		r.failRefresh.Store(true)
	}
	return admitted, err
}
func TestService_MoveTaskReturnsErrorWhenFeederRefreshFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-c": {ID: "step-c", WorkflowID: "wf-source", Name: "C", Position: 2},
		"step-a": {ID: "step-a", WorkflowID: "wf-source", Name: "A", Position: 0},
		"step-b": {
			ID: "step-b", WorkflowID: "wf-source", Name: "B", Position: 1,
			WIPLimit: 1, PullFromStepID: "step-a",
		},
	}})
	createMoveTask(t, ctx, repo, "task-feeder-refresh-failure", "wf-source", "step-c", nil)
	svc.tasks = &failAfterMoveRefreshTaskRepository{Repository: repo}

	result, err := svc.MoveTask(ctx, "task-feeder-refresh-failure", "wf-source", "step-a", 0)
	if err == nil {
		t.Fatalf("MoveTask returned success with an unverified task response: result=%+v", result)
	}
	if result != nil {
		t.Fatalf("MoveTask returned a result alongside refresh error: %+v", result)
	}

	stored, err := repo.GetTask(ctx, "task-feeder-refresh-failure")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.WorkflowStepID != "step-b" {
		t.Fatalf("stored task step = %q, want durable feeder promotion at step-b", stored.WorkflowStepID)
	}
}
