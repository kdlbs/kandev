package service

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
)

// ValidateInheritedWorkspaceRepositorySelection verifies explicit repository
// slots before a subtask attaches to a parent's materialized environment.
// A missing environment is left to the normal launch path because the parent
// may not have materialized its workspace yet.
func (s *Service) ValidateInheritedWorkspaceRepositorySelection(
	ctx context.Context,
	parentTaskID string,
	repositories []TaskRepositoryInput,
) error {
	if parentTaskID == "" || len(repositories) == 0 {
		return nil
	}
	env, err := s.taskEnvironments.GetTaskEnvironmentByTaskID(ctx, parentTaskID)
	if err != nil {
		return fmt.Errorf("%w: inspect inherited workspace", models.ErrWorkspaceReuseUnsafe)
	}
	if env == nil {
		return nil
	}
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, env.ID)
	if err != nil {
		return fmt.Errorf("%w: inspect inherited workspace inventory", models.ErrWorkspaceReuseUnsafe)
	}
	for _, repository := range repositories {
		if countActiveInventoryMatches(repository, rows) != 1 {
			return fmt.Errorf(
				"%w: repository %q branch %q is not an exact match in the inherited workspace; use workspace_mode=new_workspace for a different checkout",
				models.ErrWorkspaceReuseUnsafe,
				repository.RepositoryID,
				worktree.SanitizeBranchSlug(repository.BaseBranch),
			)
		}
	}
	return nil
}

func countActiveInventoryMatches(
	repository TaskRepositoryInput,
	rows []*models.TaskEnvironmentRepo,
) int {
	expectedBranch := worktree.SanitizeBranchSlug(repository.BaseBranch)
	matches := 0
	for _, row := range rows {
		if row == nil || row.RepositoryID != repository.RepositoryID ||
			row.DeletedAt != nil || row.Status == "failed" || row.Status == "deleted" {
			continue
		}
		if worktree.SanitizeBranchSlug(row.BranchSlug) == expectedBranch {
			matches++
		}
	}
	return matches
}
