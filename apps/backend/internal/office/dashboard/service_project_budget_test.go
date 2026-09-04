package dashboard_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// projectBudgetEvaluatorSpy implements dashboard.ProjectBudgetEvaluator and
// records every call, so UpdateTaskProjectID tests can assert whether (and
// with what arguments) the reassignment budget hook fired.
type projectBudgetEvaluatorSpy struct {
	calls      []projectBudgetEvaluatorCall
	err        error
	onEvaluate func()
}

type projectBudgetEvaluatorCall struct {
	workspaceID string
	projectID   string
}

func (s *projectBudgetEvaluatorSpy) EvaluateProjectBudget(_ context.Context, workspaceID, projectID string) error {
	s.calls = append(s.calls, projectBudgetEvaluatorCall{workspaceID: workspaceID, projectID: projectID})
	if s.onEvaluate != nil {
		s.onEvaluate()
	}
	return s.err
}

// projectAssignmentOrderingEventBus records whether the reassignment budget
// hook ran before the task-updated event was published.
type projectAssignmentOrderingEventBus struct {
	evaluated          *bool
	published          bool
	evaluatedAtPublish bool
}

func (b *projectAssignmentOrderingEventBus) Publish(_ context.Context, subject string, _ *bus.Event) error {
	if subject == events.OfficeTaskUpdated {
		b.published = true
		b.evaluatedAtPublish = *b.evaluated
	}
	return nil
}

func (b *projectAssignmentOrderingEventBus) Subscribe(string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *projectAssignmentOrderingEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (b *projectAssignmentOrderingEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (b *projectAssignmentOrderingEventBus) Close()            {}
func (b *projectAssignmentOrderingEventBus) IsConnected() bool { return true }

func insertTestProject(t *testing.T, deps *testDeps, id, wsID string) {
	t.Helper()
	_, err := deps.db.Exec(`
		INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
	`, id, wsID, "Test Project "+id)
	if err != nil {
		t.Fatalf("insert project %s: %v", id, err)
	}
}

// TestUpdateTaskProjectID_EvaluatesDestinationProjectBudget covers the
// reassignment defect: assigning a task into a project must re-check that
// project's budget policies, since #2903 made project-cost rollups follow
// the task's live project_id — a reassignment alone can cross a threshold
// with no cost event or agent launch to trigger the existing budget hooks.
func TestUpdateTaskProjectID_EvaluatesDestinationProjectBudget(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-1", "ws-1", "Task", "todo", 1)
	insertTestProject(t, deps, "proj-1", "ws-1")

	spy := &projectBudgetEvaluatorSpy{}
	deps.svc.SetProjectBudgetEvaluator(spy)

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", "proj-1"); err != nil {
		t.Fatalf("UpdateTaskProjectID: %v", err)
	}

	if len(spy.calls) != 1 {
		t.Fatalf("evaluator calls = %d, want 1", len(spy.calls))
	}
	if spy.calls[0].workspaceID != "ws-1" || spy.calls[0].projectID != "proj-1" {
		t.Errorf("evaluator called with %+v, want {ws-1 proj-1}", spy.calls[0])
	}
}

// TestUpdateTaskProjectID_ClearingProjectSkipsEvaluation covers clearing a
// project (projectID==""): there is no destination project to evaluate.
func TestUpdateTaskProjectID_ClearingProjectSkipsEvaluation(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-1", "ws-1", "Task", "todo", 1)

	spy := &projectBudgetEvaluatorSpy{}
	deps.svc.SetProjectBudgetEvaluator(spy)

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", ""); err != nil {
		t.Fatalf("UpdateTaskProjectID: %v", err)
	}

	if len(spy.calls) != 0 {
		t.Errorf("expected no evaluator calls when clearing project, got %+v", spy.calls)
	}
}

// TestUpdateTaskProjectID_UnchangedProjectSkipsEvaluation covers a no-op
// reassignment (PATCH project_id to the project the task is already in):
// nothing crossed a threshold, so the budget evaluator must not fire. Without
// this guard every retried/duplicate PATCH would write a fresh budget.alert
// activity row for an already over-threshold project.
func TestUpdateTaskProjectID_UnchangedProjectSkipsEvaluation(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-1", "ws-1", "Task", "todo", 1)
	insertTestProject(t, deps, "proj-1", "ws-1")

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", "proj-1"); err != nil {
		t.Fatalf("UpdateTaskProjectID (initial assign): %v", err)
	}

	spy := &projectBudgetEvaluatorSpy{}
	deps.svc.SetProjectBudgetEvaluator(spy)

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", "proj-1"); err != nil {
		t.Fatalf("UpdateTaskProjectID (no-op reassign): %v", err)
	}

	if len(spy.calls) != 0 {
		t.Errorf("expected no evaluator calls for a no-op reassignment, got %+v", spy.calls)
	}
}

// TestUpdateTaskProjectID_NoEvaluatorWired covers the default (unwired)
// dependency: reassignment must still succeed when no evaluator is set.
func TestUpdateTaskProjectID_NoEvaluatorWired(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-1", "ws-1", "Task", "todo", 1)
	insertTestProject(t, deps, "proj-1", "ws-1")

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", "proj-1"); err != nil {
		t.Fatalf("UpdateTaskProjectID: %v", err)
	}
}

// TestUpdateTaskProjectID_EvaluatorErrorDoesNotFailReassignment covers the
// best-effort contract: the tasks.project_id write has already committed by
// the time the evaluator runs, so an evaluator error must be swallowed
// (logged), not surfaced as a failed reassignment.
func TestUpdateTaskProjectID_EvaluatorErrorDoesNotFailReassignment(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-1", "ws-1", "Task", "todo", 1)
	insertTestProject(t, deps, "proj-1", "ws-1")

	spy := &projectBudgetEvaluatorSpy{err: errors.New("boom")}
	deps.svc.SetProjectBudgetEvaluator(spy)

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-1", "proj-1"); err != nil {
		t.Fatalf("UpdateTaskProjectID should not fail on evaluator error, got: %v", err)
	}

	task, err := deps.repo.GetTaskByID(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("GetTaskByID: %v", err)
	}
	if task.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want proj-1 (write must persist despite evaluator error)", task.ProjectID)
	}
}

func TestUpdateTaskProjectID_EvaluatesBeforePublishingTaskUpdated(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "task-order", "ws-1", "Task", "todo", 1)
	insertTestProject(t, deps, "proj-order", "ws-1")

	evaluated := false
	spy := &projectBudgetEvaluatorSpy{onEvaluate: func() { evaluated = true }}
	eventsBus := &projectAssignmentOrderingEventBus{evaluated: &evaluated}
	deps.svc.SetProjectBudgetEvaluator(spy)
	deps.svc.SetEventBus(eventsBus)

	if err := deps.svc.UpdateTaskProjectID(context.Background(), "task-order", "proj-order"); err != nil {
		t.Fatalf("UpdateTaskProjectID: %v", err)
	}

	if !eventsBus.published {
		t.Fatal("expected office.task.updated to be published")
	}
	if !eventsBus.evaluatedAtPublish {
		t.Fatal("budget evaluation must complete before office.task.updated is published")
	}
}
