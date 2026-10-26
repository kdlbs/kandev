package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
func TestPrepareResumeRepositorySettings_ReusesCheckoutBranchInventory(t *testing.T) {
	for _, executorType := range []string{"local_docker", "remote_docker", "sprites"} {
		t.Run(executorType, func(t *testing.T) {
			repo := newMockRepository()
			repo.repositories["repo-1"] = &models.Repository{
				ID: "repo-1", Name: "repo", LocalPath: "/tmp/repo", RemoteURL: "https://example.com/repo.git",
			}
			repo.tasks["task-1"] = &models.Task{ID: "task-1"}
			repo.taskRepositories["tr-1"] = &models.TaskRepository{
				ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1",
				BaseBranch: "release/x", CheckoutBranch: "feature/y",
			}
			row := &models.TaskEnvironmentRepo{
				TaskEnvironmentID: "env-1", RepositoryID: "repo-1", BranchSlug: "feature-y",
				WorktreePath: "/workspace/repo", WorktreeBranch: "feature/y", Status: taskEnvironmentRepoStatusActive,
			}
			env := &models.TaskEnvironment{
				ID: "env-1", TaskID: "task-1", ExecutorType: executorType,
				Status: models.TaskEnvironmentStatusReady, MaterializationSessionID: "sess-owner",
				WorkspacePath: "/workspace/repo", Repos: []*models.TaskEnvironmentRepo{row},
			}
			repo.taskEnvironments[env.ID] = env
			repo.taskEnvironmentRepos[env.ID] = env.Repos
			session := &models.TaskSession{ID: "sess-guest", TaskID: "task-1", TaskEnvironmentID: env.ID}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			req := &LaunchAgentRequest{TaskID: "task-1", SessionID: session.ID, ExecutorType: executorType}

			repositoryID, selected, _, err := exec.prepareResumeRepositorySettings(
				context.Background(), &v1.Task{ID: "task-1"}, session, req, ResumeOptions{},
			)
			if err != nil {
				t.Fatalf("prepareResumeRepositorySettings: %v", err)
			}
			if repositoryID != "repo-1" || selected != env || !req.WorkspaceReuseRequired {
				t.Fatalf("resume did not retain the canonical environment: repo=%q env=%v reuse=%v", repositoryID, selected, req.WorkspaceReuseRequired)
			}
			if req.BranchIdentitySlug != "feature-y" || req.BaseBranch != "release/x" {
				t.Fatalf("resume identity=%q base=%q, want feature-y and release/x", req.BranchIdentitySlug, req.BaseBranch)
			}
			if req.CheckoutBranch != "" || req.Metadata["repository_clone_url"] != nil {
				t.Fatalf("resume requested another checkout or clone: checkout=%q metadata=%v", req.CheckoutBranch, req.Metadata)
			}
			if selected.WorkspacePath != "/workspace/repo" || len(selected.Repos) != 1 || selected.Repos[0] != row ||
				row.BranchSlug != "feature-y" || row.WorktreePath != "/workspace/repo" || row.WorktreeBranch != "feature/y" {
				t.Fatal("resume changed the canonical checkout inventory")
			}
		})
	}
}

func TestPrepareResumeRepositorySettings_MaterializationUsesBaseBranchIdentity(t *testing.T) {
	repo := newMockRepository()
	repo.repositories["repo-1"] = &models.Repository{
		ID: "repo-1", Name: "repo", LocalPath: "/tmp/repo", RemoteURL: "https://example.com/repo.git",
	}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{
		ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1",
		BaseBranch: "release/x", CheckoutBranch: "feature/y",
	}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: session.ID, ExecutorType: "local_docker"}
	if _, _, _, err := exec.prepareResumeRepositorySettings(
		context.Background(), &v1.Task{ID: "task-1"}, session, req, ResumeOptions{},
	); err != nil {
		t.Fatalf("prepareResumeRepositorySettings: %v", err)
	}
	spec, ok := topLevelLaunchRepoSpec(req)
	if !ok || req.WorkspaceReuseRequired || launchRepoBranchIdentitySlug(spec) != "release-x" {
		t.Fatalf("fresh materialization identity=%q reuse=%v, want release-x without reuse", launchRepoBranchIdentitySlug(spec), req.WorkspaceReuseRequired)
	}
}
