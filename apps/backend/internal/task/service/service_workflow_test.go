package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeWorkflowStepGetter struct {
	steps   map[string]*wfmodels.WorkflowStep
	nextErr error
}

func (f *fakeWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if step, ok := f.steps[stepID]; ok {
		return step, nil
	}
	return nil, errStepNotFoundForTest
}

func (f *fakeWorkflowStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	if f.nextErr != nil {
		return nil, f.nextErr
	}
	var next *wfmodels.WorkflowStep
	for _, step := range f.steps {
		if step.WorkflowID != workflowID || step.Position <= currentPosition {
			continue
		}
		if next == nil || step.Position < next.Position {
			next = step
		}
	}
	return next, nil
}

type testStepNotFound struct{}

func (testStepNotFound) Error() string { return "step not found" }

var errStepNotFoundForTest = testStepNotFound{}

// TestService_SetWorkflowHidden_HealsStaleRecord verifies the helper used by
// the improve-kandev bootstrap to flip Hidden=true on workflows created
// before the flag was honored on insert.
func TestService_SetWorkflowHidden_HealsStaleRecord(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-stale", WorkspaceID: "ws-1", Name: "Improve Kandev", Hidden: false})

	if err := svc.SetWorkflowHidden(ctx, "wf-stale", true); err != nil {
		t.Fatalf("SetWorkflowHidden: %v", err)
	}

	visible, err := svc.ListWorkflows(ctx, "ws-1", false)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	for _, wf := range visible {
		if wf.ID == "wf-stale" {
			t.Fatalf("hidden workflow leaked into default listing: %+v", wf)
		}
	}

	all, err := svc.ListWorkflows(ctx, "ws-1", true)
	if err != nil {
		t.Fatalf("ListWorkflows(includeHidden): %v", err)
	}
	var found *models.Workflow
	for _, wf := range all {
		if wf.ID == "wf-stale" {
			found = wf
		}
	}
	if found == nil || !found.Hidden {
		t.Fatalf("expected wf-stale to be hidden after heal, got %+v", found)
	}
}

func TestService_MoveTaskRejectsInvalidWorkflowTargets(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)

	tests := []struct {
		name     string
		taskID   string
		targetWF string
		targetSt string
	}{
		{
			name:     "step belongs to another workflow",
			taskID:   "task-invalid-step",
			targetWF: "wf-source",
			targetSt: "step-target",
		},
		{
			name:     "workflow belongs to another workspace",
			taskID:   "task-other-workspace",
			targetWF: "wf-other-workspace",
			targetSt: "step-other-workspace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createMoveTask(t, ctx, repo, tt.taskID, "wf-source", "step-source", nil)

			_, err := svc.MoveTask(ctx, tt.taskID, tt.targetWF, tt.targetSt, 0)
			if err == nil {
				t.Fatalf("expected move to be rejected")
			}

			task, err := repo.GetTask(ctx, tt.taskID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.WorkflowID != "wf-source" || task.WorkflowStepID != "step-source" {
				t.Fatalf("task moved despite validation error: workflow=%s step=%s", task.WorkflowID, task.WorkflowStepID)
			}
		})
	}
}

func TestService_MoveTaskAllowsPendingReviewWhenSessionIdle(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-pending-review", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-pending-review", "task-pending-review", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)

	moved, err := svc.MoveTask(ctx, "task-pending-review", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("pending review on idle session should not block manual move: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskToTerminalStepCompletesTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-terminal", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-terminal", "wf-source", "step-done", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if moved.Task.State != v1.TaskStateCompleted {
		t.Fatalf("moved task state = %q, want COMPLETED", moved.Task.State)
	}

	task, err := repo.GetTask(ctx, "task-terminal")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("persisted task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_MoveTaskToTerminalStepPreservesTerminalFailureStates(t *testing.T) {
	cases := []struct {
		name  string
		state v1.TaskState
	}{
		{name: "failed", state: v1.TaskStateFailed},
		{name: "cancelled", state: v1.TaskStateCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
			getter.steps["step-done"] = &wfmodels.WorkflowStep{
				ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
			}
			createMoveTask(t, ctx, repo, "task-terminal-"+tc.name, "wf-source", "step-source", nil)
			task, err := repo.GetTask(ctx, "task-terminal-"+tc.name)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			task.State = tc.state
			must(t, repo.UpdateTask(ctx, task))

			moved, err := svc.MoveTask(ctx, task.ID, "wf-source", "step-done", 0)
			if err != nil {
				t.Fatalf("MoveTask: %v", err)
			}
			if moved.Task.State != tc.state {
				t.Fatalf("moved task state = %q, want %q", moved.Task.State, tc.state)
			}

			task, err = repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.State != tc.state {
				t.Fatalf("persisted task state = %q, want %q", task.State, tc.state)
			}
		})
	}
}

func TestService_MoveTaskFailsWhenTerminalStatusLookupFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.nextErr = errors.New("next step lookup failed")
	createMoveTask(t, ctx, repo, "task-terminal-lookup-error", "wf-source", "step-source", nil)

	_, err := svc.MoveTask(ctx, "task-terminal-lookup-error", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected move to fail when terminal status lookup fails")
	}

	task, err := repo.GetTask(ctx, "task-terminal-lookup-error")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite lookup error: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskOutOfTerminalStepReopensTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-reopened", "wf-source", "step-done", nil)
	task, err := repo.GetTask(ctx, "task-reopened")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	task.State = v1.TaskStateCompleted
	must(t, repo.UpdateTask(ctx, task))
	eventBus.ClearEvents()

	moved, err := svc.MoveTask(ctx, "task-reopened", "wf-source", "step-source", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}
	if moved.Task.State != v1.TaskStateTODO {
		t.Fatalf("moved task state = %q, want TODO", moved.Task.State)
	}

	task, err = repo.GetTask(ctx, "task-reopened")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateTODO {
		t.Fatalf("persisted task state = %q, want TODO", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_ApproveSessionToTerminalStepCompletesTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Approved", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-approved", "wf-source", "step-review-target", nil)
	createMoveSession(t, ctx, repo, "session-approved", "task-approved", models.TaskSessionStateWaitingForInput, models.ReviewStatusPending)
	eventBus.ClearEvents()

	result, err := svc.ApproveSession(ctx, "session-approved")
	if err != nil {
		t.Fatalf("ApproveSession: %v", err)
	}
	if result.Task == nil || result.Task.WorkflowStepID != "step-done" {
		t.Fatalf("approved task step = %+v, want step-done", result.Task)
	}
	if result.Task.State != v1.TaskStateCompleted {
		t.Fatalf("approved task state = %q, want COMPLETED", result.Task.State)
	}

	task, err := repo.GetTask(ctx, "task-approved")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("persisted task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_MoveTaskRejectsRunningSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-running", "task-running", models.TaskSessionStateRunning, models.ReviewStatusNone)

	_, err := svc.MoveTask(ctx, "task-running", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected running session move to be rejected")
	}

	task, err := repo.GetTask(ctx, "task-running")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite running session: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskWithOptionsAllowsRunningPrimarySession(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-running-primary", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-running-primary", "task-running-primary", models.TaskSessionStateRunning, models.ReviewStatusNone)
	eventBus.ClearEvents()

	moved, err := svc.MoveTaskWithOptions(ctx, "task-running-primary", "wf-source", "step-review-target", 0, MoveTaskOptions{
		AllowActivePrimarySession: true,
	})
	if err != nil {
		t.Fatalf("running primary session should be movable with explicit option: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if got := data["session_id"]; got != "session-running-primary" {
		t.Fatalf("session_id = %v, want session-running-primary", got)
	}
}

func TestService_MoveTaskRejectsFullWIPLimitedTarget(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source": {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-full":   {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 1, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)

	_, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-full", 0)
	if err == nil {
		t.Fatalf("expected WIP-limited move to be rejected")
	}
	if !strings.Contains(err.Error(), "WIP limit") {
		t.Fatalf("error = %q, want WIP limit rejection", err.Error())
	}

	task, err := repo.GetTask(ctx, "task-moving")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != "step-source" {
		t.Fatalf("task moved despite WIP limit: %s", task.WorkflowStepID)
	}
}

func TestService_MoveTaskAllowsSameStepReorderWhenStepAlreadyOverLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-full": {ID: "step-full", WorkflowID: "wf-source", Name: "Full", Position: 0, WIPLimit: 1},
	}})
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-full", nil)
	createMoveTask(t, ctx, repo, "task-occupant", "wf-source", "step-full", nil)

	moved, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-full", 5)
	if err != nil {
		t.Fatalf("same-step reorder should be exempt from WIP limit: %v", err)
	}
	if moved.Task.Position != 5 {
		t.Fatalf("position = %d, want 5", moved.Task.Position)
	}
}

func TestService_MoveTaskIgnoresArchivedAndEphemeralOccupantsForWIPLimit(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source":  {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-limited": {ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 1, WIPLimit: 1},
	}})
	now := time.Now().UTC()
	createMoveTask(t, ctx, repo, "task-moving", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-archived", "wf-source", "step-limited", &now)
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-ephemeral",
		WorkspaceID:    "ws-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-limited",
		Title:          "Ephemeral",
		State:          v1.TaskStateTODO,
		Priority:       "medium",
		IsEphemeral:    true,
	}); err != nil {
		t.Fatalf("CreateTask(ephemeral): %v", err)
	}

	moved, err := svc.MoveTask(ctx, "task-moving", "wf-source", "step-limited", 0)
	if err != nil {
		t.Fatalf("archived/ephemeral occupants should not consume WIP: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-limited" {
		t.Fatalf("step = %s, want step-limited", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskPullsNextFeederTaskOnVacate(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Name: "Limited", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Name: "Feeder", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-low", "wf-source", "step-feeder", nil)
	createMoveTask(t, ctx, repo, "task-critical", "wf-source", "step-feeder", nil)
	setMoveTaskOrder(t, ctx, repo, "task-low", 0, "low")
	setMoveTaskOrder(t, ctx, repo, "task-critical", 0, "critical")
	eventBus.ClearEvents()

	_, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	pulled, err := repo.GetTask(ctx, "task-critical")
	if err != nil {
		t.Fatalf("GetTask(task-critical): %v", err)
	}
	if pulled.WorkflowStepID != "step-limited" {
		t.Fatalf("critical feeder task step = %s, want step-limited", pulled.WorkflowStepID)
	}
	notPulled, err := repo.GetTask(ctx, "task-low")
	if err != nil {
		t.Fatalf("GetTask(task-low): %v", err)
	}
	if notPulled.WorkflowStepID != "step-feeder" {
		t.Fatalf("low feeder task step = %s, want step-feeder", notPulled.WorkflowStepID)
	}

	movedEvents := 0
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskMoved {
			movedEvents++
		}
	}
	if movedEvents != 2 {
		t.Fatalf("task.moved events = %d, want 2", movedEvents)
	}
}

func TestService_MoveTaskRejectsArchivedTask(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	now := time.Now().UTC()
	createMoveTask(t, ctx, repo, "task-archived", "wf-source", "step-source", &now)

	_, err := svc.MoveTask(ctx, "task-archived", "wf-source", "step-review-target", 0)
	if err == nil {
		t.Fatalf("expected archived task move to be rejected")
	}
}

func TestService_MoveTaskMovedEventIncludesSourceWorkflow(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-cross-workflow", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.MoveTask(ctx, "task-cross-workflow", "wf-target", "step-target", 0)
	if err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	updatedEvent := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskUpdated)
	updatedData, ok := updatedEvent.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("updated event data type = %T, want map[string]interface{}", updatedEvent.Data)
	}
	if got := updatedData["old_workflow_id"]; got != "wf-source" {
		t.Fatalf("old_workflow_id = %v, want wf-source", got)
	}

	event := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskMoved)
	data, ok := event.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map[string]interface{}", event.Data)
	}
	if got := data["from_workflow_id"]; got != "wf-source" {
		t.Fatalf("from_workflow_id = %v, want wf-source", got)
	}
	if got := data["to_workflow_id"]; got != "wf-target" {
		t.Fatalf("to_workflow_id = %v, want wf-target", got)
	}
}

func TestService_BulkMoveTasksUpdatedEventIncludesSourceWorkflow(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-bulk-cross-workflow", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.BulkMoveTasks(ctx, "wf-source", "", "wf-target", "step-target")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}

	updatedEvent := findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskUpdated)
	updatedData, ok := updatedEvent.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("updated event data type = %T, want map[string]interface{}", updatedEvent.Data)
	}
	if got := updatedData["old_workflow_id"]; got != "wf-source" {
		t.Fatalf("old_workflow_id = %v, want wf-source", got)
	}
}

func TestService_BulkMoveTasksToTerminalStepCompletesTasks(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
	}
	createMoveTask(t, ctx, repo, "task-bulk-terminal", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	_, err := svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-source", "step-done")
	if err != nil {
		t.Fatalf("BulkMoveTasks: %v", err)
	}

	task, err := repo.GetTask(ctx, "task-bulk-terminal")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.State != v1.TaskStateCompleted {
		t.Fatalf("bulk-moved task state = %q, want COMPLETED", task.State)
	}
	findPublishedEvent(t, eventBus.GetPublishedEvents(), events.TaskStateChanged)
}

func TestService_BulkMoveTasksToTerminalStepPreservesTerminalFailureStates(t *testing.T) {
	cases := []struct {
		name  string
		state v1.TaskState
	}{
		{name: "failed", state: v1.TaskStateFailed},
		{name: "cancelled", state: v1.TaskStateCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			seedMoveWorkflows(t, ctx, repo)
			seedMoveSteps(svc)
			getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
			getter.steps["step-done"] = &wfmodels.WorkflowStep{
				ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2,
			}
			createMoveTask(t, ctx, repo, "task-bulk-terminal-"+tc.name, "wf-source", "step-source", nil)
			task, err := repo.GetTask(ctx, "task-bulk-terminal-"+tc.name)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			task.State = tc.state
			must(t, repo.UpdateTask(ctx, task))

			_, err = svc.BulkMoveTasks(ctx, "wf-source", "step-source", "wf-source", "step-done")
			if err != nil {
				t.Fatalf("BulkMoveTasks: %v", err)
			}

			task, err = repo.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			if task.State != tc.state {
				t.Fatalf("bulk-moved task state = %q, want %q", task.State, tc.state)
			}
		})
	}
}

func TestService_BulkMoveSelectedTasksValidatesBatchBeforeMoving(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-batch-ok", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-batch-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-batch-running", "task-batch-running", models.TaskSessionStateRunning, models.ReviewStatusNone)

	_, err := svc.BulkMoveSelectedTasks(ctx, []string{"task-batch-ok", "task-batch-running"}, "wf-target", "step-target")
	if err == nil {
		t.Fatalf("expected selected batch move to be rejected")
	}

	for _, id := range []string{"task-batch-ok", "task-batch-running"} {
		task, err := repo.GetTask(ctx, id)
		if err != nil {
			t.Fatalf("GetTask(%s): %v", id, err)
		}
		if task.WorkflowID != "wf-source" || task.WorkflowStepID != "step-source" {
			t.Fatalf("%s moved despite rejected batch: workflow=%s step=%s", id, task.WorkflowID, task.WorkflowStepID)
		}
	}
}

func TestService_BulkMoveSelectedTasksSkipsCurrentTargetAndAppendsInOrder(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-target-existing", "wf-target", "step-target", nil)
	createMoveTask(t, ctx, repo, "task-source-a", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-target-already", "wf-target", "step-target", nil)
	createMoveTask(t, ctx, repo, "task-source-b", "wf-source", "step-source", nil)
	eventBus.ClearEvents()

	result, err := svc.BulkMoveSelectedTasks(
		ctx,
		[]string{"task-source-a", "task-target-already", "task-source-b"},
		"wf-target",
		"step-target",
	)
	if err != nil {
		t.Fatalf("BulkMoveSelectedTasks: %v", err)
	}
	if result.MovedCount != 2 {
		t.Fatalf("MovedCount = %d, want 2", result.MovedCount)
	}

	sourceA, err := repo.GetTask(ctx, "task-source-a")
	if err != nil {
		t.Fatalf("GetTask(task-source-a): %v", err)
	}
	sourceB, err := repo.GetTask(ctx, "task-source-b")
	if err != nil {
		t.Fatalf("GetTask(task-source-b): %v", err)
	}
	if sourceA.Position != 2 || sourceB.Position != 3 {
		t.Fatalf("positions = (%d, %d), want (2, 3)", sourceA.Position, sourceB.Position)
	}

	movedEvents := 0
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskMoved {
			movedEvents++
		}
	}
	if movedEvents != 2 {
		t.Fatalf("task.moved events = %d, want 2", movedEvents)
	}
}

func seedMoveWorkflows(t *testing.T, ctx context.Context, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
}) {
	t.Helper()
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace 1"}))
	must(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Workspace 2"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-source", WorkspaceID: "ws-1", Name: "Source"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-target", WorkspaceID: "ws-1", Name: "Target"}))
	must(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-other-workspace", WorkspaceID: "ws-2", Name: "Other"}))
}

func seedMoveSteps(svc *Service) {
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-source":          {ID: "step-source", WorkflowID: "wf-source", Name: "Source", Position: 0},
		"step-review-target":   {ID: "step-review-target", WorkflowID: "wf-source", Name: "Review", Position: 1},
		"step-target":          {ID: "step-target", WorkflowID: "wf-target", Name: "Target", Position: 0},
		"step-other-workspace": {ID: "step-other-workspace", WorkflowID: "wf-other-workspace", Name: "Other", Position: 0},
	}})
}

func createMoveTask(t *testing.T, ctx context.Context, repo interface {
	CreateTask(context.Context, *models.Task) error
	ArchiveTask(context.Context, string) error
}, id, workflowID, stepID string, archivedAt *time.Time) {
	t.Helper()
	must(t, repo.CreateTask(ctx, &models.Task{
		ID:             id,
		WorkspaceID:    "ws-1",
		WorkflowID:     workflowID,
		WorkflowStepID: stepID,
		Title:          id,
		State:          v1.TaskStateTODO,
		ArchivedAt:     archivedAt,
	}))
	if archivedAt != nil {
		must(t, repo.ArchiveTask(ctx, id))
	}
}

func setMoveTaskOrder(t *testing.T, ctx context.Context, repo interface {
	GetTask(context.Context, string) (*models.Task, error)
	UpdateTask(context.Context, *models.Task) error
}, id string, position int, priority string) {
	t.Helper()
	task, err := repo.GetTask(ctx, id)
	if err != nil {
		t.Fatalf("GetTask(%s): %v", id, err)
	}
	task.Position = position
	task.Priority = priority
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask(%s): %v", id, err)
	}
}

func createMoveSession(t *testing.T, ctx context.Context, repo interface {
	CreateTaskSession(context.Context, *models.TaskSession) error
}, id, taskID string, state models.TaskSessionState, reviewStatus models.ReviewStatus) {
	t.Helper()
	must(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:           id,
		TaskID:       taskID,
		State:        state,
		IsPrimary:    true,
		ReviewStatus: reviewStatus,
	}))
}

func findPublishedEvent(t *testing.T, published []*bus.Event, eventType string) *bus.Event {
	t.Helper()
	for _, event := range published {
		if event.Type == eventType {
			return event
		}
	}
	t.Fatalf("event %s not published; got %d events", eventType, len(published))
	return nil
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
