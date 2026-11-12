package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakeOfficeApprovalGuard struct {
	pending []PendingApprover
	err     error
	calls   int
	stepIDs []string
}

func (g *fakeOfficeApprovalGuard) PendingApproversForCompletion(_ context.Context, _, stepID string) ([]PendingApprover, error) {
	g.calls++
	g.stepIDs = append(g.stepIDs, stepID)
	return g.pending, g.err
}

type approvalGuardFixture struct {
	svc  *Service
	repo interface {
		GetTask(context.Context, string) (*models.Task, error)
	}
	guard *fakeOfficeApprovalGuard
}

func newApprovalGuardFixture(t *testing.T, office bool, guard *fakeOfficeApprovalGuard) (*approvalGuardFixture, func() []string) {
	t.Helper()
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	getter := svc.workflowStepGetter.(*fakeWorkflowStepGetter)
	getter.steps["step-done"] = &wfmodels.WorkflowStep{
		ID: "step-done", WorkflowID: "wf-source", Name: "Done", Position: 2, CompleteTaskOnEnter: true,
	}
	task := &models.Task{
		ID: "task-guard", WorkspaceID: "ws-1", WorkflowID: "wf-source", WorkflowStepID: "step-source",
		Title: "task-guard", State: v1.TaskStateTODO,
	}
	if office {
		task.ProjectID = "proj-1"
	}
	must(t, repo.CreateTask(ctx, task))
	if guard != nil {
		svc.SetOfficeApprovalGuard(guard)
	}
	eventBus.ClearEvents()
	eventTypes := func() []string {
		var types []string
		for _, e := range eventBus.GetPublishedEvents() {
			types = append(types, e.Type)
		}
		return types
	}
	return &approvalGuardFixture{svc: svc, repo: repo, guard: guard}, eventTypes
}

func humanMove() MoveTaskOptions {
	return MoveTaskOptions{AllowActivePrimarySession: true, StepHistoryActor: wfmodels.StepTransitionActorHuman}
}

func (f *approvalGuardFixture) mustState(t *testing.T, wantStep, wantState string) {
	t.Helper()
	task, err := f.repo.GetTask(context.Background(), "task-guard")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.WorkflowStepID != wantStep || string(task.State) != wantState {
		t.Fatalf("task = (%q, %q), want (%q, %q)", task.WorkflowStepID, task.State, wantStep, wantState)
	}
}

// @covers AC-OFFICE-HUMAN-DONE-002.1 AC-OFFICE-HUMAN-DONE-002.2
func TestMoveTask_OfficeHumanIntoCompletingStep_ApproversPending_RefusedBeforeWrite(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{pending: []PendingApprover{{AgentProfileID: "ceo", Name: "CEO"}}}
	f, eventTypes := newApprovalGuardFixture(t, true, guard)

	_, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, humanMove())

	var pendingErr *ApprovalsPendingMoveError
	if !errors.As(err, &pendingErr) {
		t.Fatalf("err = %v, want ApprovalsPendingMoveError", err)
	}
	if len(pendingErr.Pending) != 1 || pendingErr.Pending[0].Name != "CEO" {
		t.Fatalf("pending = %+v, want CEO", pendingErr.Pending)
	}
	f.mustState(t, "step-source", string(v1.TaskStateTODO))
	if types := eventTypes(); len(types) != 0 {
		t.Fatalf("published events = %v, want none", types)
	}
}

func TestMoveTask_OfficeHumanIntoCompletingStep_NoPendingApprovers_Completes(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{}
	f, _ := newApprovalGuardFixture(t, true, guard)

	if _, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, humanMove()); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if guard.calls != 1 || guard.stepIDs[0] != "step-done" {
		t.Fatalf("guard calls = %d for steps %v, want 1 for the destination step-done", guard.calls, guard.stepIDs)
	}
	f.mustState(t, "step-done", string(v1.TaskStateCompleted))
}

func TestMoveTask_OfficeHumanApprovalGuardError_AbortsMove(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{err: errors.New("read failed")}
	f, eventTypes := newApprovalGuardFixture(t, true, guard)

	_, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, humanMove())
	if err == nil || !errors.Is(err, guard.err) {
		t.Fatalf("err = %v, want wrapped guard error", err)
	}
	f.mustState(t, "step-source", string(v1.TaskStateTODO))
	if types := eventTypes(); len(types) != 0 {
		t.Fatalf("published events = %v, want none", types)
	}
}

// @covers AC-OFFICE-HUMAN-DONE-002.3
func TestMoveTask_OfficeEngineMove_SkipsApprovalGuard(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{pending: []PendingApprover{{AgentProfileID: "ceo"}}}
	f, _ := newApprovalGuardFixture(t, true, guard)

	opts := MoveTaskOptions{StepHistoryActor: wfmodels.StepTransitionActorSystem}
	if _, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, opts); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if guard.calls != 0 {
		t.Fatalf("guard calls = %d, want 0", guard.calls)
	}
	f.mustState(t, "step-done", string(v1.TaskStateCompleted))
}

// @covers AC-OFFICE-HUMAN-DONE-002.3
func TestMoveTask_NonOfficeTask_SkipsApprovalGuard(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{pending: []PendingApprover{{AgentProfileID: "ceo"}}}
	f, _ := newApprovalGuardFixture(t, false, guard)

	if _, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, humanMove()); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if guard.calls != 0 {
		t.Fatalf("guard calls = %d, want 0", guard.calls)
	}
	f.mustState(t, "step-done", string(v1.TaskStateCompleted))
}

// @covers AC-OFFICE-HUMAN-DONE-002.3
func TestMoveTask_OfficeHumanIntoNonCompletingStep_SkipsApprovalGuard(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{pending: []PendingApprover{{AgentProfileID: "ceo"}}}
	f, _ := newApprovalGuardFixture(t, true, guard)

	if _, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-review-target", 0, humanMove()); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	if guard.calls != 0 {
		t.Fatalf("guard calls = %d, want 0", guard.calls)
	}
	f.mustState(t, "step-review-target", string(v1.TaskStateTODO))
}

func TestMoveTask_OfficeHumanWithoutGuard_Completes(t *testing.T) {
	f, _ := newApprovalGuardFixture(t, true, nil)

	if _, err := f.svc.MoveTaskWithOptions(context.Background(), "task-guard", "wf-source", "step-done", 0, humanMove()); err != nil {
		t.Fatalf("MoveTaskWithOptions: %v", err)
	}
	f.mustState(t, "step-done", string(v1.TaskStateCompleted))
}

// @covers AC-OFFICE-HUMAN-DONE-002.1
func TestOfficeHumanCompletionLegacyScalarGuard(t *testing.T) {
	guard := &fakeOfficeApprovalGuard{pending: []PendingApprover{{AgentProfileID: "ceo", Name: "CEO"}}}
	f, eventTypes := newApprovalGuardFixture(t, true, guard)
	_, err := f.svc.UpdateTaskStateAsHuman(context.Background(), "task-guard", v1.TaskStateCompleted)
	var pending *ApprovalsPendingMoveError
	if !errors.As(err, &pending) {
		t.Fatalf("legacy scalar error = %v, want pending approvers", err)
	}
	f.mustState(t, "step-source", string(v1.TaskStateTODO))
	if types := eventTypes(); len(types) != 0 {
		t.Fatalf("refused scalar published %v", types)
	}
	guard.pending = nil
	if _, err := f.svc.UpdateTaskStateAsHuman(context.Background(), "task-guard", v1.TaskStateCompleted); err != nil {
		t.Fatal(err)
	}
	f.mustState(t, "step-source", string(v1.TaskStateCompleted))
	if len(eventTypes()) != 1 {
		t.Fatalf("successful scalar events = %v, want one", eventTypes())
	}
}
