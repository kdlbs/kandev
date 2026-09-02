package dashboard_test

import (
	"context"
	"errors"
	"testing"
)

// projectBudgetEvaluatorSpy implements dashboard.ProjectBudgetEvaluator and
// records every call, so UpdateTaskProjectID tests can assert whether (and
// with what arguments) the reassignment budget hook fired.
type projectBudgetEvaluatorSpy struct {
	calls []projectBudgetEvaluatorCall
	err   error
}

type projectBudgetEvaluatorCall struct {
	workspaceID string
	projectID   string
}

func (s *projectBudgetEvaluatorSpy) EvaluateProjectBudget(_ context.Context, workspaceID, projectID string) error {
	s.calls = append(s.calls, projectBudgetEvaluatorCall{workspaceID: workspaceID, projectID: projectID})
	return s.err
}

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
