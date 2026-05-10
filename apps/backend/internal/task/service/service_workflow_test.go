package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeWorkflowStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (f *fakeWorkflowStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if step, ok := f.steps[stepID]; ok {
		return step, nil
	}
	return nil, errStepNotFoundForTest
}

func (f *fakeWorkflowStepGetter) GetNextStepByPosition(_ context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	for _, step := range f.steps {
		if step.WorkflowID == workflowID && step.Position == currentPosition+1 {
			return step, nil
		}
	}
	return nil, nil
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
	pending := "pending"
	createMoveSession(t, ctx, repo, "session-pending-review", "task-pending-review", models.TaskSessionStateWaitingForInput, &pending)

	moved, err := svc.MoveTask(ctx, "task-pending-review", "wf-source", "step-review-target", 0)
	if err != nil {
		t.Fatalf("pending review on idle session should not block manual move: %v", err)
	}
	if moved.Task.WorkflowStepID != "step-review-target" {
		t.Fatalf("expected step-review-target, got %s", moved.Task.WorkflowStepID)
	}
}

func TestService_MoveTaskRejectsRunningSession(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-running", "task-running", models.TaskSessionStateRunning, nil)

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

func TestService_BulkMoveSelectedTasksValidatesBatchBeforeMoving(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-batch-ok", "wf-source", "step-source", nil)
	createMoveTask(t, ctx, repo, "task-batch-running", "wf-source", "step-source", nil)
	createMoveSession(t, ctx, repo, "session-batch-running", "task-batch-running", models.TaskSessionStateRunning, nil)

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

func createMoveSession(t *testing.T, ctx context.Context, repo interface {
	CreateTaskSession(context.Context, *models.TaskSession) error
}, id, taskID string, state models.TaskSessionState, reviewStatus *string) {
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
