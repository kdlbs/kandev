package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPrepareResumeRepositorySettings_ReusesFreshMaterializationIdentity(t *testing.T) {
	for _, executorType := range []string{"local_docker", "remote_docker", "sprites"} {
		for _, inventory := range []string{"base", "foreign branch", "inactive", "duplicate", "foreign environment", "wrong placement", "explicit checkout"} {
			t.Run(executorType+"/"+inventory, func(t *testing.T) {
				repo := newMockRepository()
				repo.repositories["repo-1"] = &models.Repository{
					ID: "repo-1", Name: "repo", LocalPath: "/tmp/repo", RemoteURL: "https://example.com/repo.git",
				}
				repo.tasks["task-1"] = &models.Task{ID: "task-1"}
				repo.taskRepositories["tr-1"] = &models.TaskRepository{
					ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "release/x", CheckoutBranch: "feature/y",
				}
				exec := newTestExecutor(t, &mockAgentManager{}, repo)
				task := &v1.Task{ID: "task-1"}
				first := &models.TaskSession{ID: "materializer", TaskID: task.ID}
				fresh := &LaunchAgentRequest{TaskID: task.ID, SessionID: first.ID, ExecutorType: executorType}
				if _, _, _, err := exec.prepareResumeRepositorySettings(context.Background(), task, first, fresh, ResumeOptions{}); err != nil {
					t.Fatalf("prepare fresh workspace: %v", err)
				}
				rows := environmentReposForLaunch(fresh, &LaunchAgentResponse{})
				if len(rows) != 1 || rows[0].BranchSlug != "release-x" {
					t.Fatalf("fresh inventory = %+v, want release-x", rows)
				}
				rows[0].TaskEnvironmentID, rows[0].Status = "env-1", taskEnvironmentRepoStatusActive
				switch inventory {
				case "foreign branch":
					rows[0].BranchSlug = "foreign-branch"
				case "inactive":
					rows[0].Status = taskEnvironmentRepoStatusFailed
				case "foreign environment":
					rows[0].TaskEnvironmentID = "foreign-env"
				case "wrong placement":
					rows[0].WorkspaceRelativePath = "nested/repo"
				case "duplicate":
					duplicate := *rows[0]
					rows = append(rows, &duplicate)
				}
				env := &models.TaskEnvironment{
					ID: "env-1", TaskID: task.ID, ExecutorType: executorType, Status: models.TaskEnvironmentStatusReady,
					MaterializationSessionID: first.ID, WorkspacePath: "/workspace/repo", Repos: rows,
				}
				repo.taskEnvironments[env.ID], repo.taskEnvironmentRepos[env.ID] = env, rows
				later := &models.TaskSession{ID: "later-session", TaskID: task.ID, TaskEnvironmentID: env.ID}
				reuse := &LaunchAgentRequest{TaskID: task.ID, SessionID: later.ID, ExecutorType: executorType}
				if inventory == "explicit checkout" {
					reuse.CheckoutBranch = "feature/new"
				}
				_, selected, _, err := exec.prepareResumeRepositorySettings(context.Background(), task, later, reuse, ResumeOptions{})
				if inventory != "base" {
					if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) || !reuse.WorkspaceReuseRequired {
						t.Fatalf("invalid inventory returned err=%v reuse=%v, want guarded refusal", err, reuse.WorkspaceReuseRequired)
					}
					return
				}
				if err != nil || selected != env || !reuse.WorkspaceReuseRequired || reuse.BranchIdentitySlug != "release-x" {
					t.Fatalf("reuse returned err=%v env=%v reuse=%v identity=%q", err, selected, reuse.WorkspaceReuseRequired, reuse.BranchIdentitySlug)
				}
				if rows[0].BranchSlug != "release-x" || reuse.BaseBranch != "release/x" || reuse.CheckoutBranch != "" || reuse.Metadata["repository_clone_url"] != nil {
					t.Fatal("reuse changed inventory, comparison base, or requested materialization")
				}
			})
		}
	}
}
