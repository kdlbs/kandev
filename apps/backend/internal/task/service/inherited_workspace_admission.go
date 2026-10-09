package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

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
	if s.taskEnvironments == nil {
		return nil
	}
	target, err := s.resolveBranchMaterializationTarget(ctx, parentTaskID)
	if err != nil {
		return fmt.Errorf("%w: inspect inherited workspace", models.ErrWorkspaceReuseUnsafe)
	}
	if target == nil || target.environment == nil ||
		target.environment.Status != models.TaskEnvironmentStatusReady {
		return nil
	}
	rows, err := s.taskEnvironments.ListTaskEnvironmentRepos(ctx, target.environment.ID)
	if err != nil {
		return fmt.Errorf("%w: inspect inherited workspace inventory", models.ErrWorkspaceReuseUnsafe)
	}
	for _, repository := range repositories {
		repository, err = s.resolveExistingInheritedRepository(ctx, target.environment.TaskID, repository)
		if err != nil {
			return err
		}
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

func (s *Service) resolveExistingInheritedRepository(
	ctx context.Context,
	taskID string,
	repository TaskRepositoryInput,
) (TaskRepositoryInput, error) {
	if repository.RepositoryID != "" {
		return repository, nil
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return repository, fmt.Errorf("%w: resolve inherited repository workspace", models.ErrWorkspaceReuseUnsafe)
	}
	found, err := s.findExistingInheritedRepository(ctx, task.WorkspaceID, repository)
	if err != nil {
		return repository, fmt.Errorf("%w: resolve inherited repository locator", models.ErrWorkspaceReuseUnsafe)
	}
	if found == nil {
		return repository, fmt.Errorf(
			"%w: repository locator is not an existing inherited repository; use workspace_mode=new_workspace for a different checkout",
			models.ErrWorkspaceReuseUnsafe,
		)
	}
	repository.RepositoryID = found.ID
	if repository.BaseBranch == "" {
		repository.BaseBranch = found.DefaultBranch
	}
	return repository, nil
}

func (s *Service) findExistingInheritedRepository(
	ctx context.Context,
	workspaceID string,
	repository TaskRepositoryInput,
) (*models.Repository, error) {
	if path := strings.TrimSpace(repository.LocalPath); path != "" {
		return s.repoEntities.GetRepositoryByLocalPath(ctx, workspaceID, filepath.Clean(path))
	}
	rawURL := effectiveRemoteURL(repository)
	if rawURL == "" {
		return nil, nil
	}
	provider, owner, name, _, err := parseRemoteRepositoryURL(rawURL, repository.Provider)
	if err != nil {
		return nil, err
	}
	repositories, err := s.repoEntities.ListRepositories(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return findRepositoryByProviderIdentity(repositories, provider, owner, name), nil
}

func findRepositoryByProviderIdentity(
	repositories []*models.Repository,
	provider, owner, name string,
) *models.Repository {
	for _, candidate := range repositories {
		if candidate != nil &&
			strings.EqualFold(candidate.Provider, provider) &&
			candidate.ProviderOwner == owner &&
			candidate.ProviderName == name {
			return candidate
		}
	}
	return nil
}

func countActiveInventoryMatches(
	repository TaskRepositoryInput,
	rows []*models.TaskEnvironmentRepo,
) int {
	expectedBranch := worktree.SanitizeBranchSlug(repository.BaseBranch)
	hasBranchScopedRow := false
	for _, row := range rows {
		if row != nil && row.RepositoryID == repository.RepositoryID &&
			worktree.SanitizeBranchSlug(row.BranchSlug) != "" {
			hasBranchScopedRow = true
			break
		}
	}
	matches := 0
	for _, row := range rows {
		if row == nil || row.RepositoryID != repository.RepositoryID ||
			row.DeletedAt != nil || row.Status == "failed" || row.Status == "deleted" {
			continue
		}
		branchMatches := worktree.SanitizeBranchSlug(row.BranchSlug) == expectedBranch
		if expectedBranch != "" && !hasBranchScopedRow && row.BranchSlug == "" {
			branchMatches = true
		}
		if branchMatches {
			matches++
		}
	}
	return matches
}
