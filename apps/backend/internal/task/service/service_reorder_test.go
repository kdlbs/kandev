package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// seedReorderTestStep inserts a workflow_steps row directly (same shape as
// the repository-level reorder tests) and wires a fakeWorkflowStepGetter so
// Service.ReorderStepTasks can resolve it without a full workflow service.
func seedReorderTestStep(t *testing.T, svc *Service, repo *sqliterepo.Repository, stepID, workflowID string, stepPosition int) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.DB().Exec(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stepID, workflowID, stepID, stepPosition, now, now); err != nil {
		t.Fatalf("insert step %s: %v", stepID, err)
	}
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepID: {ID: stepID, WorkflowID: workflowID, Name: stepID, Position: stepPosition},
	}})
}

func mustCreateReorderServiceTask(t *testing.T, ctx context.Context, repo *sqliterepo.Repository, id, workspaceID, workflowID, stepID string) {
	t.Helper()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: id, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: stepID, Title: id,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
}

func TestService_ReorderStepTasksCommitsAndPublishesEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-svc", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-svc", WorkspaceID: "ws-reorder-svc", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-svc", "wf-reorder-svc", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "svc-task-a", "ws-reorder-svc", "wf-reorder-svc", "step-reorder-svc")
	mustCreateReorderServiceTask(t, ctx, repo, "svc-task-b", "ws-reorder-svc", "wf-reorder-svc", "step-reorder-svc")
	eventBus.ClearEvents()

	result, err := svc.ReorderStepTasks(ctx, "step-reorder-svc", "admitted", []string{"svc-task-b", "svc-task-a"})
	if err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}
	if result.WorkflowStepID != "step-reorder-svc" || result.Revision != 1 {
		t.Fatalf("result = %+v, want step-reorder-svc/revision 1", result)
	}
	if len(result.Tasks) != 2 || result.Tasks[0].ID != "svc-task-b" || result.Tasks[1].ID != "svc-task-a" {
		t.Fatalf("result.Tasks = %+v, want [svc-task-b svc-task-a]", result.Tasks)
	}

	found := false
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TaskReordered {
			continue
		}
		found = true
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("task.reordered payload is not a map: %#v", event.Data)
		}
		if data["workflow_step_id"] != "step-reorder-svc" {
			t.Fatalf("payload workflow_step_id = %v, want step-reorder-svc", data["workflow_step_id"])
		}
		if data["band"] != "admitted" {
			t.Fatalf("payload band = %v, want admitted", data["band"])
		}
		if data["workspace_id"] != "ws-reorder-svc" {
			t.Fatalf("payload workspace_id = %v, want ws-reorder-svc", data["workspace_id"])
		}
	}
	if !found {
		t.Fatal("no task.reordered event published")
	}
}

func TestService_ReorderStepTasksStepChangedCarriesAuthoritativeOrder(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-conflict", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-conflict", WorkspaceID: "ws-reorder-conflict", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-conflict", "wf-reorder-conflict", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "conflict-a", "ws-reorder-conflict", "wf-reorder-conflict", "step-reorder-conflict")
	mustCreateReorderServiceTask(t, ctx, repo, "conflict-b", "ws-reorder-conflict", "wf-reorder-conflict", "step-reorder-conflict")
	eventBus.ClearEvents()

	// Submitted set is missing conflict-b: the classic membership-drift race.
	result, err := svc.ReorderStepTasks(ctx, "step-reorder-conflict", "admitted", []string{"conflict-a"})
	if !errors.Is(err, repoerrors.ErrStepChanged) {
		t.Fatalf("err = %v, want ErrStepChanged", err)
	}
	if result == nil || len(result.Tasks) != 2 {
		t.Fatalf("result = %+v, want the authoritative 2-task order", result)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskReordered {
			t.Fatal("task.reordered must not publish on a rejected reorder")
		}
	}
}

// TestService_ReorderStepTasksLeavesTaskIdentityAndSessionUntouched pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.21: a reorder changes only position —
// never workflow, step, WIP admission, queued destination/time, or state —
// and publishes no task.updated or task.moved event alongside task.reordered.
func TestService_ReorderStepTasksLeavesTaskIdentityAndSessionUntouched(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-untouched", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-untouched", WorkspaceID: "ws-reorder-untouched", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-untouched", "wf-reorder-untouched", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "untouched-a", "ws-reorder-untouched", "wf-reorder-untouched", "step-reorder-untouched")
	mustCreateReorderServiceTask(t, ctx, repo, "untouched-b", "ws-reorder-untouched", "wf-reorder-untouched", "step-reorder-untouched")
	before, err := repo.GetTask(ctx, "untouched-a")
	if err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()

	if _, err := svc.ReorderStepTasks(ctx, "step-reorder-untouched", "admitted", []string{"untouched-b", "untouched-a"}); err != nil {
		t.Fatalf("ReorderStepTasks: %v", err)
	}

	after, err := repo.GetTask(ctx, "untouched-a")
	if err != nil {
		t.Fatal(err)
	}
	if after.Position == before.Position {
		t.Fatal("position did not change; the reorder did not commit")
	}
	if after.WorkflowID != before.WorkflowID || after.WorkflowStepID != before.WorkflowStepID {
		t.Fatalf("workflow/step changed: before=%s/%s after=%s/%s", before.WorkflowID, before.WorkflowStepID, after.WorkflowID, after.WorkflowStepID)
	}
	if after.WIPAdmitted != before.WIPAdmitted || after.QueuedForStepID != before.QueuedForStepID {
		t.Fatalf("WIP admission changed: before admitted=%v queued=%q after admitted=%v queued=%q",
			before.WIPAdmitted, before.QueuedForStepID, after.WIPAdmitted, after.QueuedForStepID)
	}
	if after.State != before.State {
		t.Fatalf("state changed: before=%s after=%s", before.State, after.State)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskMoved || event.Type == events.TaskUpdated {
			t.Fatalf("reorder must not publish %s", event.Type)
		}
	}
}

// TestService_ReorderStepTasksDeniesCallerWithoutWorkflowAccess pins
// AC-TASKS-KANBAN-TASK-REORDERING-001.23: reorder authority is exactly the
// existing move authority (authorizeWorkflowID), so a caller who cannot see
// the workflow's workspace is denied before any membership is validated or
// written.
func TestService_ReorderStepTasksDeniesCallerWithoutWorkflowAccess(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-denied", Name: "Workspace", OwnerID: "user-b"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-denied", WorkspaceID: "ws-reorder-denied", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-denied", "wf-reorder-denied", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "denied-a", "ws-reorder-denied", "wf-reorder-denied", "step-reorder-denied")
	mustCreateReorderServiceTask(t, ctx, repo, "denied-b", "ws-reorder-denied", "wf-reorder-denied", "step-reorder-denied")
	before, err := repo.GetTask(ctx, "denied-a")
	if err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()

	_, err = svc.ReorderStepTasks(ctxAs("user-a"), "step-reorder-denied", "admitted", []string{"denied-b", "denied-a"})
	if err == nil {
		t.Fatal("ReorderStepTasks: want a denial for a caller with no access to the workflow's workspace")
	}

	after, err := repo.GetTask(ctx, "denied-a")
	if err != nil {
		t.Fatal(err)
	}
	if after.Position != before.Position {
		t.Fatalf("position changed on denied reorder: before=%d after=%d", before.Position, after.Position)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("denied reorder must publish no event, got %+v", eventBus.GetPublishedEvents())
	}

	// The workspace's own owner remains authorized.
	if _, err := svc.ReorderStepTasks(ctxAs("user-b"), "step-reorder-denied", "admitted", []string{"denied-b", "denied-a"}); err != nil {
		t.Fatalf("owner reorder: %v", err)
	}
}

// TestService_ReorderStepTasksNamingTaskInAnotherWorkspaceDoesNotLeakExistence
// pins the "no existence leak" invariant (service_access.go) against
// ReorderStepTasks: authorizeWorkflowID only authorizes the step's own
// workflow, never the individual submitted task ids, so naming a real task
// id from a workspace the caller cannot see must resolve exactly like naming
// a nonexistent id (ErrInvalidReorder) — not ErrStepChanged, which would
// confirm the id exists somewhere the caller has no access to.
func TestService_ReorderStepTasksNamingTaskInAnotherWorkspaceDoesNotLeakExistence(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-a-leak", Name: "A", OwnerID: "user-a"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-a-leak", WorkspaceID: "ws-a-leak", Name: "WF-A"}); err != nil {
		t.Fatal(err)
	}
	// seedReorderTestStep wires the fake workflow-step getter to know only
	// about the step being reordered, matching what the service actually
	// resolves for this call.
	seedReorderTestStep(t, svc, repo, "step-a-leak", "wf-a-leak", 0)
	mustCreateReorderServiceTask(t, ctx, repo, "task-a-leak", "ws-a-leak", "wf-a-leak", "step-a-leak")

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-b-leak", Name: "B", OwnerID: "user-b"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-b-leak", WorkspaceID: "ws-b-leak", Name: "WF-B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().Exec(`INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"step-b-leak", "wf-b-leak", "step-b-leak", 0, time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	mustCreateReorderServiceTask(t, ctx, repo, "task-b-secret-leak", "ws-b-leak", "wf-b-leak", "step-b-leak")

	// user-a is authorized for step-a-leak's own workflow, but has zero
	// access to ws-b-leak. Naming task-b-secret-leak (real, but foreign)
	// must not be distinguishable from naming an id that does not exist.
	_, err := svc.ReorderStepTasks(ctxAs("user-a"), "step-a-leak", "admitted", []string{"task-a-leak", "task-b-secret-leak"})
	if !errors.Is(err, repoerrors.ErrInvalidReorder) {
		t.Fatalf("err = %v, want ErrInvalidReorder (a foreign-workspace task id must read the same as a nonexistent one)", err)
	}
}

func TestService_ReorderStepTasksInvalidRequestReturnsNilResult(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-invalid", Name: "Workspace"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-invalid", WorkspaceID: "ws-reorder-invalid", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedReorderTestStep(t, svc, repo, "step-reorder-invalid", "wf-reorder-invalid", 0)

	result, err := svc.ReorderStepTasks(ctx, "step-reorder-invalid", "not-a-band", []string{"whatever"})
	if !errors.Is(err, repoerrors.ErrInvalidReorder) {
		t.Fatalf("err = %v, want ErrInvalidReorder", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil on a malformed request", result)
	}
}
