package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.8
func TestSingleRepoWorktreeTransitionRetainsCanonicalSlot(t *testing.T) {
	for _, test := range []struct {
		name, checkout, base, expected string
	}{
		{name: "base branch", base: "main", expected: "main"},
		{name: "checkout branch", checkout: "feature/topic", base: "main", expected: "feature-topic"},
		{name: "default branch", expected: "main"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _ := remoteSelectionSQLiteTemplate.Open(t)
			repo := tasksqlite.NewWithInitializedDB(db, db, nil)
			ctx := t.Context()
			require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
			require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", WorkspaceID: "workspace", Title: "Task"}))
			env := &models.TaskEnvironment{
				ID: "environment", TaskID: "task", ExecutorType: "local",
				Status: models.TaskEnvironmentStatusReady, WorkspacePath: "/source",
			}
			require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: "session", TaskID: "task", TaskEnvironmentID: env.ID, State: models.TaskSessionStateCreated,
			}))
			require.NoError(t, repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
				ID: "canonical-slot", TaskEnvironmentID: env.ID, RepositoryID: "repository", BranchSlug: test.expected,
			}))
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			req := &LaunchAgentRequest{TaskID: "task", SessionID: "session", TaskEnvironmentID: env.ID}
			_, err := exec.applyRepositoryConfig(req, &v1.Task{ID: "task", Title: "Task"}, &repoInfo{
				RepositoryID: "repository", RepositoryPath: "/source", BaseBranch: test.base, CheckoutBranch: test.checkout,
				Repository: &models.Repository{ID: "repository", Name: "repo", DefaultBranch: "main"},
			}, executorConfig{ExecutorType: "worktree"}, nil)
			require.NoError(t, err)
			require.Empty(t, req.BranchSlug, "the primary checkout must keep its flat directory path")

			store, err := worktree.NewSQLiteStore(db, db)
			require.NoError(t, err)
			// The manager persists BranchIdentitySlug independently from the path slug.
			require.NoError(t, store.CreateWorktree(ctx, &worktree.Worktree{
				ID: "physical", SessionID: req.SessionID, TaskEnvironmentID: env.ID,
				RepositoryID: req.RepositoryID, BranchSlug: req.BranchIdentitySlug,
				Path: "/tasks/task/repo", Branch: "feature/task", Status: worktree.StatusActive,
			}))
			incoming := environmentReposForLaunch(req, &LaunchAgentResponse{
				WorktreeID: "physical", WorktreePath: "/tasks/task/repo", WorktreeBranch: "feature/task",
			})
			env.ExecutorType = "worktree"
			env.WorkspacePath = "/tasks/task"
			require.NoError(t, repo.PersistTaskEnvironmentTransition(ctx, env, incoming, true))
			rows, err := repo.ListTaskEnvironmentRepos(ctx, env.ID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "canonical-slot", rows[0].ID)
			require.Equal(t, test.expected, rows[0].BranchSlug)
			require.Equal(t, "physical", rows[0].WorktreeID)
		})
	}
}
