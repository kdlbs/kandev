package backendapp

import (
	"os/exec"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// initLocalGitFixture creates a real git repo so CreateRepository's local-path
// validation (which requires a `.git` directory) accepts the fixture.
func initLocalGitFixture(t *testing.T, path string) string {
	t.Helper()
	if err := exec.Command("git", "init", "-q", path).Run(); err != nil {
		t.Skipf("git unavailable: %v", err)
	}
	return path
}

// TestBootTaskDTOsWithSessionInfoEvaluatesRunnerMutability guards against the
// boot-payload projection path (one of the four REQ-TASKS-RUNNER-SWITCH-001.5
// paths that must run the evaluation) silently falling back to FromTask's
// fail-closed default instead of the task's real verdict.
func TestBootTaskDTOsWithSessionInfoEvaluatesRunnerMutability(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := t.Context()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, true)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("ListWorkflows: %v", err)
	}
	steps, err := harness.workflowSvc.ListStepsByWorkflow(ctx, workflows[0].ID)
	if err != nil || len(steps) == 0 {
		t.Fatalf("ListStepsByWorkflow: %v", err)
	}
	repository, err := harness.taskSvc.CreateRepository(ctx, &taskservice.CreateRepositoryRequest{
		WorkspaceID: workspaces[0].ID, Name: "Boot runner repo", SourceType: "local",
		LocalPath: initLocalGitFixture(t, t.TempDir()), DefaultBranch: "main",
	})
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	eligibleResult, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, WorkflowStepID: steps[0].ID,
		Title:        "Boot eligible task",
		Repositories: []taskservice.TaskRepositoryInput{{RepositoryID: repository.ID, BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask (eligible): %v", err)
	}

	ineligibleResult, err := harness.taskSvc.CreateTask(ctx, &taskservice.CreateTaskRequest{
		WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, WorkflowStepID: steps[0].ID,
		Title:        "Boot ineligible task",
		Repositories: []taskservice.TaskRepositoryInput{{RepositoryID: repository.ID, BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("CreateTask (ineligible): %v", err)
	}
	now := time.Now().UTC()
	if err := harness.taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "boot-runner-session", TaskID: ineligibleResult.Task.ID,
		State: models.TaskSessionStateWaitingForInput, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	builder := bootStateBuilder{p: routeParams{taskSvc: harness.taskSvc}}
	dtos := builder.taskDTOsWithSessionInfo(ctx, []*models.Task{eligibleResult.Task, ineligibleResult.Task})
	if len(dtos) != 2 {
		t.Fatalf("taskDTOsWithSessionInfo returned %d DTOs, want 2", len(dtos))
	}

	byID := map[string]int{}
	for i, dto := range dtos {
		byID[dto.ID] = i
	}
	eligible := dtos[byID[eligibleResult.Task.ID]]
	if !eligible.RunnerEditable || eligible.RunnerIneligibleReason != "eligible" {
		t.Fatalf("eligible task runner projection = editable=%v reason=%q, want editable=true reason=eligible",
			eligible.RunnerEditable, eligible.RunnerIneligibleReason)
	}

	ineligible := dtos[byID[ineligibleResult.Task.ID]]
	if ineligible.RunnerEditable || ineligible.RunnerIneligibleReason != "session_exists" {
		t.Fatalf("ineligible task runner projection = editable=%v reason=%q, want editable=false reason=session_exists",
			ineligible.RunnerEditable, ineligible.RunnerIneligibleReason)
	}

	// Confirms the whole boot-map pipeline, not just the DTO field, so a
	// future regression in mapKanbanTaskState's whitelist would fail here too.
	mapped := mapKanbanTaskState(eligible)
	if mapped["runnerEditable"] != true || mapped["runnerIneligibleReason"] != "eligible" {
		t.Fatalf("mapKanbanTaskState(eligible) = %#v, want runnerEditable=true reason=eligible", mapped)
	}
}
