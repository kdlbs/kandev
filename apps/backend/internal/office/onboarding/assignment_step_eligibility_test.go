package onboarding

// TestMaybeCreateOnboardingTask_StepIneligible_SkipsWake and its eligible
// counterpart are the ISSUE-5 regression for onboarding's own task_assigned
// producer (maybeCreateOnboardingTask): onboarding never sets
// StartAgent/PlanMode when creating the task, so it resolves the landing
// step via ResolveStartStep (is_start_step), not ResolveAutoStartStep
// (auto_start_agent) — for a custom workflow those can genuinely differ,
// which is exactly the ISSUE-5 defect class. The task itself must still be
// created either way; only the wake is gated.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeOnboardingStepGetter is a minimal shared.AssignmentStepGetter test
// double: a static step map.
type fakeOnboardingStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (f *fakeOnboardingStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return f.steps[stepID], nil
}

// createOnboardingTasksTable brings up a minimal tasks table: the office
// repository (unlike the task-domain one) does not own this table's schema,
// and no existing onboarding test needed it since CreateOfficeTask is always
// mocked out. GetTaskWorkflowStepID needs a real row to gate against.
func createOnboardingTasksTable(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(), `CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		workflow_step_id TEXT DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
}

func TestMaybeCreateOnboardingTask_StepIneligible_SkipsWake(t *testing.T) {
	const stepBacklog = "step-backlog"

	svc, _, repo, _ := newTestOnboardingServiceWithRepo(t)
	createOnboardingTasksTable(t, repo)
	svc.taskCreator = &mockTaskCreatorOnboarding{} // always returns "task-001"
	queuer := &fakeOnboardingRunQueuer{}
	svc.runQueuer = queuer
	svc.SetWorkflowStepGetter(&fakeOnboardingStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepBacklog: {ID: stepBacklog, Events: wfmodels.StepEvents{}},
	}})
	if _, err := repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id, workflow_step_id) VALUES ('task-001', 'ws-1', ?)`,
		stepBacklog,
	); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	taskID := svc.maybeCreateOnboardingTask(context.Background(), "ws-1", "agent-1", CompleteRequest{
		TaskTitle:       "Explore the codebase",
		TaskDescription: "Create an engineering roadmap",
	})
	if taskID != "task-001" {
		t.Fatalf("taskID = %q, want task-001 (task creation must not be gated)", taskID)
	}
	if queuer.called {
		t.Fatalf("QueueRun called for a task landing on an ineligible (non-auto-start) step")
	}
}

func TestMaybeCreateOnboardingTask_StepEligible_QueuesWake(t *testing.T) {
	const stepWork = "step-work"

	svc, _, repo, _ := newTestOnboardingServiceWithRepo(t)
	createOnboardingTasksTable(t, repo)
	svc.taskCreator = &mockTaskCreatorOnboarding{} // always returns "task-001"
	queuer := &fakeOnboardingRunQueuer{}
	svc.runQueuer = queuer
	svc.SetWorkflowStepGetter(&fakeOnboardingStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepWork: {
			ID: stepWork,
			Events: wfmodels.StepEvents{
				OnEnter: []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterAutoStartAgent}},
			},
		},
	}})
	if _, err := repo.ExecRaw(context.Background(),
		`INSERT INTO tasks (id, workspace_id, workflow_step_id) VALUES ('task-001', 'ws-1', ?)`,
		stepWork,
	); err != nil {
		t.Fatalf("insert task: %v", err)
	}

	taskID := svc.maybeCreateOnboardingTask(context.Background(), "ws-1", "agent-1", CompleteRequest{
		TaskTitle:       "Explore the codebase",
		TaskDescription: "Create an engineering roadmap",
	})
	if taskID != "task-001" {
		t.Fatalf("taskID = %q, want task-001", taskID)
	}
	if !queuer.called {
		t.Fatalf("QueueRun not called for a task landing on an eligible (auto-start) step")
	}
}
