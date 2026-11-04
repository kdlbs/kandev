package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// TestRenameTitleBranchBindingPassesVerifiedOldBranch verifies the title
// handoff carries the recorded generated branch, repository subpath, and
// explicit primary identity into the lifecycle seam.
func TestRenameTitleBranchBindingPassesVerifiedOldBranch(t *testing.T) {
	runtime := &titleBranchRuntimeStub{}
	service := &Service{titleBranchRuntime: runtime}
	result := TitleBranchRenameResult{}
	service.renameTitleBranchBinding(
		context.Background(),
		&models.Task{ID: "task-binding", Title: "Final title"},
		"Final title",
		models.ExecutorTypeWorktree,
		&models.TaskEnvironment{
			WorkspacePath: "/workspace",
			Repos:         []*models.TaskEnvironmentRepo{{RepositoryID: "repo-backend", WorktreeBranch: "feature/provisional"}},
		},
		true,
		titleBranchBinding{
			sessionID:       "session-binding",
			taskRepository:  &models.TaskRepository{RepositoryID: "repo-backend", Position: 0},
			repository:      &models.Repository{ID: "repo-backend", Name: "backend", WorktreeBranchTemplate: "feature/{title}"},
			worktree:        &models.TaskEnvironmentRepo{RepositoryID: "repo-backend", WorktreeBranch: "feature/provisional", WorktreePath: "/workspace/backend"},
			environmentRepo: &models.TaskEnvironmentRepo{RepositoryID: "repo-backend", WorktreeBranch: "feature/provisional"},
		},
		&result,
	)

	require.Equal(t, TitleBranchStatusRenamed, aggregateTitleBranchRenameStatus(result.Renamed, result.Preserved, result.Failed))
	require.Equal(t, []titleBranchRuntimeCall{{
		sessionID: "session-binding", expectedBranch: "feature/provisional",
		newName: "feature/final-title", repo: "backend",
	}}, runtime.calls)
	require.Equal(t, []bool{true}, runtime.primaryCalls)
	require.Equal(t, "feature/final-title", result.Renamed[0].To)
}
