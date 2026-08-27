package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.1
// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-001.2
// @covers AC-TASKS-RUNTIME-CLEANUP-001.2
func TestPersistTaskEnvironmentRepos_PreservesPhysicalWorktreeOnInventoryOnlyRefresh(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	const envID = "env-inventory-only"
	repo.taskEnvironmentRepos[envID] = []*models.TaskEnvironmentRepo{
		{
			ID:                "env-inventory-only-repo-main",
			TaskEnvironmentID: envID,
			RepositoryID:      "repo-kandev",
			BranchSlug:        "main",
			WorktreeID:        "wt-existing",
			WorktreePath:      "/tasks/task-1/repo",
			WorktreeBranch:    "feature/task-1",
			Position:          4,
			ErrorMessage:      "stale inventory error",
		},
	}

	repos := environmentReposForLaunch(
		&LaunchAgentRequest{
			RepositoryID:       "repo-kandev",
			RepositoryPath:     "/source/kandev",
			BranchIdentitySlug: "main",
		},
		&LaunchAgentResponse{WorkspacePath: "/source/kandev"},
	)
	exec.persistTaskEnvironmentRepos(context.Background(), envID, repos)

	row := repo.taskEnvironmentRepos[envID][0]
	if row.WorktreeID != "wt-existing" || row.WorktreePath != "/tasks/task-1/repo" || row.WorktreeBranch != "feature/task-1" {
		t.Fatalf("inventory-only refresh changed physical worktree: %+v", row)
	}
	if row.Position != 0 || row.ErrorMessage != "" {
		t.Fatalf("inventory-only refresh did not update inventory metadata: position=%d error=%q", row.Position, row.ErrorMessage)
	}
}
