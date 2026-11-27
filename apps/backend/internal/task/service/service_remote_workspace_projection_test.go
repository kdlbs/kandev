package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPluginRemoteWorkspaceRefreshUsesMaterializedRepositoryPath(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedComparisonServiceWorkspace(t, repo, "contributor/widget", "head-42")
	taskResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-comparison", WorkflowID: "wf-comparison", WorkflowStepID: "step-comparison", Title: "Remote workspace projection",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-comparison", BaseBranch: "main", CheckoutBranch: "feature/cursor-cost"}},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-plugin", TaskID: taskResult.Task.ID, ExecutorType: string(models.ExecutorTypePluginRemote),
		Status: models.TaskEnvironmentStatusReady,
		Repos:  []*models.TaskEnvironmentRepo{{ID: "env-plugin-repo", RepositoryID: "repo-comparison"}},
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	taskRepos, err := repo.ListTaskRepositories(ctx, taskResult.Task.ID)
	if err != nil || len(taskRepos) != 1 {
		t.Fatalf("ListTaskRepositories: %v rows=%d", err, len(taskRepos))
	}
	target, err := comparisonCandidate("contributor/widget", "head-42", "upstream/widget", "base-99").Build()
	if err != nil {
		t.Fatalf("Build comparison target: %v", err)
	}
	if _, _, err := repo.UpdateTaskRepositoryComparisonTarget(ctx, taskRepos[0].ID, &target, nil, false); err != nil {
		t.Fatalf("UpdateTaskRepositoryComparisonTarget: %v", err)
	}

	branches, err := svc.TaskBaseBranches(ctx, taskResult.Task.ID)
	if err != nil {
		t.Fatalf("TaskBaseBranches: %v", err)
	}
	if len(branches) != 1 || branches["widget-feature-cursor-cost"] != "main" {
		t.Fatalf("base branches = %v, want the materialized repository key", branches)
	}
	if _, hasRoot := branches[""]; hasRoot {
		t.Fatalf("base branches include a root alias for a plugin workspace: %v", branches)
	}

	targets, err := svc.TaskComparisonTargets(ctx, taskResult.Task.ID)
	if err != nil {
		t.Fatalf("TaskComparisonTargets: %v", err)
	}
	if len(targets) != 1 || !targets["widget-feature-cursor-cost"].Equal(target) {
		t.Fatalf("comparison targets = %v, want target at the materialized repository key", targets)
	}
	if _, hasRoot := targets[""]; hasRoot {
		t.Fatalf("comparison targets include a root alias for a plugin workspace: %v", targets)
	}
}
