package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
)

const (
	inheritedWorkspaceRepoStatusFailed  = "failed"
	inheritedWorkspaceRepoStatusDeleted = "deleted"
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
	resolved := make([]TaskRepositoryInput, len(repositories))
	for index, repository := range repositories {
		resolved[index], err = s.resolveExistingInheritedRepository(ctx, target.environment.TaskID, repository)
		if err != nil {
			return err
		}
	}
	var branchPlans []worktree.BranchIdentityPlan
	if target.environment.ExecutorType == string(models.ExecutorTypeWorktree) {
		branchPlans, err = inheritedWorkspaceBranchPlans(ctx, s.repoEntities, resolved)
		if err != nil {
			return fmt.Errorf("%w: inspect inherited repository defaults", models.ErrWorkspaceReuseUnsafe)
		}
	}
	usedRows := make(map[*models.TaskEnvironmentRepo]struct{}, len(resolved))
	for index, repository := range resolved {
		if repository.InheritedWorkspaceSlot && target.environment.ExecutorType == string(models.ExecutorTypeWorktree) {
			matches := activeInheritedWorktreeMatches(repository, branchPlans[index], target.environment.ID, rows)
			if len(matches) != 1 {
				return inheritedWorkspaceRepositoryMismatch(repository)
			}
			if _, used := usedRows[matches[0]]; used {
				return inheritedWorkspaceRepositoryMismatch(repository)
			}
			usedRows[matches[0]] = struct{}{}
			continue
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

func inheritedWorkspaceBranchPlans(
	ctx context.Context,
	repositories repository.RepositoryEntityRepository,
	inputs []TaskRepositoryInput,
) ([]worktree.BranchIdentityPlan, error) {
	if repositories == nil {
		return nil, fmt.Errorf("repository service is unavailable")
	}
	branchInputs := make([]worktree.BranchIdentityInput, len(inputs))
	defaults := make(map[string]string)
	for index, input := range inputs {
		defaultBranch, known := defaults[input.RepositoryID]
		if !known {
			repository, err := repositories.GetRepository(ctx, input.RepositoryID)
			if err != nil || repository == nil {
				return nil, fmt.Errorf("load repository %q", input.RepositoryID)
			}
			defaultBranch = repository.DefaultBranch
			defaults[input.RepositoryID] = defaultBranch
		}
		branchInputs[index] = worktree.BranchIdentityInput{
			RepositoryID: input.RepositoryID, BaseBranch: input.BaseBranch,
			CheckoutBranch: input.CheckoutBranch, DefaultBranch: defaultBranch,
			PRNumber: input.PRNumber, Position: input.Position,
		}
	}
	return worktree.BuildBranchIdentityPlans(branchInputs), nil
}

func activeInheritedWorktreeMatches(
	repository TaskRepositoryInput,
	plan worktree.BranchIdentityPlan,
	environmentID string,
	rows []*models.TaskEnvironmentRepo,
) []*models.TaskEnvironmentRepo {
	identity := worktree.SanitizeBranchSlug(plan.IdentitySlug)
	canonical := make([]*models.TaskEnvironmentRepo, 0, 1)
	for _, row := range rows {
		if !inheritedWorktreeInventoryRowEligible(row, repository, environmentID) {
			continue
		}
		if worktree.SanitizeBranchSlug(row.BranchSlug) == identity {
			canonical = append(canonical, row)
		}
	}
	if len(canonical) > 0 {
		return canonical
	}
	if !inheritedRepositoryHasBranchScopedRows(repository, environmentID, rows) {
		legacy := make([]*models.TaskEnvironmentRepo, 0, 1)
		for _, row := range rows {
			if inheritedWorktreeInventoryRowEligible(row, repository, environmentID) && row.BranchSlug == "" {
				legacy = append(legacy, row)
			}
		}
		if len(legacy) > 0 {
			return legacy
		}
	}
	if repository.CheckoutBranch == "" {
		return nil
	}
	return inheritedWorktreeRowsByExactBranch(repository, environmentID, rows)
}

func inheritedWorktreeRowsByExactBranch(
	repository TaskRepositoryInput,
	environmentID string,
	rows []*models.TaskEnvironmentRepo,
) []*models.TaskEnvironmentRepo {
	matches := make([]*models.TaskEnvironmentRepo, 0, 1)
	for _, row := range rows {
		if inheritedWorktreeInventoryRowEligible(row, repository, environmentID) &&
			row.WorktreeBranch == repository.CheckoutBranch {
			matches = append(matches, row)
		}
	}
	return matches
}

func inheritedWorktreeInventoryRowEligible(
	row *models.TaskEnvironmentRepo,
	repository TaskRepositoryInput,
	environmentID string,
) bool {
	return inheritedWorktreeInventoryRowLive(row, repository.RepositoryID, environmentID) &&
		row.WorkspaceRelativePath == repository.WorkspaceRelativePath
}

func inheritedWorktreeInventoryRowLive(
	row *models.TaskEnvironmentRepo,
	repositoryID, environmentID string,
) bool {
	return row != nil && row.TaskEnvironmentID == environmentID && row.RepositoryID == repositoryID &&
		row.WorktreeID != "" && row.DeletedAt == nil && row.Status != inheritedWorkspaceRepoStatusFailed &&
		row.Status != inheritedWorkspaceRepoStatusDeleted && row.Status != models.TaskEnvironmentRepoStatusDetached
}

func inheritedRepositoryHasBranchScopedRows(
	repository TaskRepositoryInput,
	environmentID string,
	rows []*models.TaskEnvironmentRepo,
) bool {
	for _, row := range rows {
		if inheritedWorktreeInventoryRowLive(row, repository.RepositoryID, environmentID) &&
			worktree.SanitizeBranchSlug(row.BranchSlug) != "" {
			return true
		}
	}
	return false
}

func inheritedWorkspaceRepositoryMismatch(repository TaskRepositoryInput) error {
	checkout := repository.CheckoutBranch
	if checkout == "" {
		checkout = repository.BaseBranch
	}
	if checkout == "" {
		checkout = "default"
	}
	return fmt.Errorf(
		"%w: repository %q checkout %q is not an exact active Worktree match in the inherited workspace; use workspace_mode=new_workspace for a separate checkout",
		models.ErrWorkspaceReuseUnsafe,
		repository.RepositoryID,
		checkout,
	)
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
	branch := repository.CheckoutBranch
	if branch == "" {
		branch = repository.BaseBranch
	}
	expectedBranch := worktree.SanitizeBranchSlug(branch)
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
