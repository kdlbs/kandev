package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func retargetReuseFixture(t *testing.T) (*Executor, *mockRepository, *LaunchAgentRequest, *models.TaskEnvironment) {
	t.Helper()
	repo := newMockRepository()
	row := &models.TaskEnvironmentRepo{ID: "slot-1", TaskEnvironmentID: "env-1", RepositoryID: "repo-1", BranchSlug: "feature-parent", WorktreeID: "wt-1", WorktreePath: "/tmp/task/repo", WorktreeBranch: "feature/work", WorktreeSourceClonePath: "/tmp/repo", WorktreeSourceCommonDir: "/tmp/repo/.git", Status: "active"}
	env := &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", ExecutorType: "worktree", MaterializationSessionID: "owner", Repos: []*models.TaskEnvironmentRepo{row}}
	repo.taskEnvironments[env.ID] = env
	repo.taskEnvironmentRepos[env.ID] = env.Repos
	repo.repositories["repo-1"] = &models.Repository{ID: "repo-1", LocalPath: "/tmp/repo"}
	repo.tasks["task-1"] = &models.Task{ID: "task-1"}
	repo.taskRepositories["tr-1"] = &models.TaskRepository{ID: "tr-1", TaskID: "task-1", RepositoryID: "repo-1", BaseBranch: "main"}
	req := &LaunchAgentRequest{TaskID: "task-1", SessionID: "guest", TaskEnvironmentID: env.ID, ExecutorType: "worktree", RepositoryID: "repo-1", BaseBranch: "main", UseWorktree: true, WorkspaceReuseRequired: true}
	return newTestExecutor(t, &mockAgentManager{}, repo), repo, req, env
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.4
func TestRetargetedBasePreservesCanonicalWorktreeIdentity(t *testing.T) {
	for _, mode := range []string{"attach", "materializer resume", "guest resume"} {
		t.Run(mode, func(t *testing.T) {
			e, _, req, env := retargetReuseFixture(t)
			before := *env.Repos[0]
			switch mode {
			case "attach":
				require.NoError(t, e.validateReuseEnvironmentInventory(context.Background(), req, env))
				e.reuseExistingRepositoryWorktrees(context.Background(), req, env)
			case "materializer resume":
				req.WorkspaceReuseRequired = false
				e.reuseExistingRepositoryWorktrees(context.Background(), req, env)
			case "guest resume":
				session := &models.TaskSession{ID: req.SessionID, TaskID: req.TaskID, TaskEnvironmentID: env.ID, RepositoryID: req.RepositoryID, BaseBranch: "feature/parent", Worktrees: env.Repos}
				_, _, _, err := e.prepareResumeRepositorySettings(context.Background(), &v1.Task{ID: req.TaskID}, session, req, ResumeOptions{})
				require.NoError(t, err)
			}
			require.Equal(t, "wt-1", req.WorktreeID)
			require.Equal(t, "feature-parent", req.BranchIdentitySlug)
			require.Equal(t, "main", req.BaseBranch)
			require.Equal(t, before, *env.Repos[0])
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.4
func TestCanonicalWorktreeIdentityRetargetMultipleRepositories(t *testing.T) {
	e, repo, req, env := retargetReuseFixture(t)
	second := &models.TaskEnvironmentRepo{ID: "slot-2", TaskEnvironmentID: env.ID, RepositoryID: "repo-2", BranchSlug: "other-parent", WorktreeID: "wt-2", WorktreePath: "/tmp/task/repo2", WorktreeBranch: "feature/work2", WorktreeSourceClonePath: "/tmp/repo2", WorktreeSourceCommonDir: "/tmp/repo2/.git", Status: "active"}
	env.Repos = append(env.Repos, second)
	repo.taskEnvironmentRepos[env.ID] = env.Repos
	req.Repositories = []RepoSpec{{RepositoryID: "repo-1", BaseBranch: "main", BranchIdentitySlug: "main", BranchSlug: "main"}, {RepositoryID: "repo-2", BaseBranch: "main", BranchIdentitySlug: "main", BranchSlug: "main"}}
	require.NoError(t, e.validateReuseEnvironmentInventory(context.Background(), req, env))
	e.reuseExistingRepositoryWorktrees(context.Background(), req, env)
	for i, want := range []string{"wt-1", "wt-2"} {
		require.Equal(t, want, req.Repositories[i].WorktreeID)
		require.Equal(t, "main", req.Repositories[i].BaseBranch)
		require.Equal(t, env.Repos[i].BranchSlug, req.Repositories[i].BranchSlug)
		require.Equal(t, env.Repos[i].BranchSlug, req.Repositories[i].BranchIdentitySlug)
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.4
func TestCanonicalWorktreeIdentityRetargetRefusesUnprovenSelection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*LaunchAgentRequest, *models.TaskEnvironment)
	}{
		{"explicit checkout", func(r *LaunchAgentRequest, _ *models.TaskEnvironment) { r.CheckoutBranch = "feature/requested" }},
		{"conflicting worktree", func(r *LaunchAgentRequest, _ *models.TaskEnvironment) { r.WorktreeID = "another-worktree" }},
		{"wrong placement", func(r *LaunchAgentRequest, _ *models.TaskEnvironment) { r.WorkspaceRelativePath = "different" }},
		{"foreign owner", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.TaskID = "other-task" }},
		{"foreign environment", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].TaskEnvironmentID = "foreign" }},
		{"missing recorded source", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].WorktreeSourceClonePath = "" }},
		{"missing recorded common directory", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].WorktreeSourceCommonDir = "" }},
		{"missing checkout path", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].WorktreePath = "" }},
		{"missing checkout branch", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].WorktreeBranch = "" }},
		{"failed", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].Status = "failed" }},
		{"deleted", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) { e.Repos[0].Status = "deleted" }},
		{"multiple physical slots", func(_ *LaunchAgentRequest, e *models.TaskEnvironment) {
			other := *e.Repos[0]
			other.ID = "slot-other"
			other.BranchSlug = "other-parent"
			other.WorktreeID = "wt-other"
			e.Repos = append(e.Repos, &other)
		}},
		{"multiple logical slots", func(r *LaunchAgentRequest, _ *models.TaskEnvironment) {
			r.Repositories = []RepoSpec{{RepositoryID: "repo-1", BranchIdentitySlug: "main"}, {RepositoryID: "repo-1", BranchIdentitySlug: "develop"}}
		}},
		{"invalid selected sibling", func(r *LaunchAgentRequest, _ *models.TaskEnvironment) {
			r.Repositories = []RepoSpec{{RepositoryID: "repo-1", BranchIdentitySlug: "main"}, {RepositoryID: "repo-missing", BranchIdentitySlug: "main"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, repo, req, env := retargetReuseFixture(t)
			tc.mutate(req, env)
			repo.taskEnvironmentRepos[env.ID] = env.Repos
			require.ErrorIs(t, e.validateReuseEnvironmentInventory(context.Background(), req, env), models.ErrWorkspaceReuseUnsafe)
		})
	}
}

func TestCanonicalWorktreeIdentityRetargetIgnoresInactiveCandidates(t *testing.T) {
	e, repo, req, env := retargetReuseFixture(t)
	inactive := *env.Repos[0]
	inactive.ID = "failed-slot"
	inactive.WorktreeID = "failed-worktree"
	inactive.Status = "failed"
	env.Repos = append(env.Repos, &inactive)
	repo.taskEnvironmentRepos[env.ID] = env.Repos
	require.NoError(t, e.validateReuseEnvironmentInventory(context.Background(), req, env))
	require.Equal(t, "wt-1", req.WorktreeID)
}

func TestCanonicalWorktreeIdentityRetargetKeepsExplicitCheckoutSelection(t *testing.T) {
	e, repo, req, env := retargetReuseFixture(t)
	req.CheckoutBranch = "feature/head"
	env.Repos[0].BranchSlug = "feature-head"
	repo.taskEnvironmentRepos[env.ID] = env.Repos
	require.NoError(t, e.validateReuseEnvironmentInventory(context.Background(), req, env))
	e.reuseExistingRepositoryWorktrees(context.Background(), req, env)
	require.Equal(t, "wt-1", req.WorktreeID)
	require.Equal(t, "feature-head", req.BranchIdentitySlug)
	require.Equal(t, "main", req.BaseBranch)
}
