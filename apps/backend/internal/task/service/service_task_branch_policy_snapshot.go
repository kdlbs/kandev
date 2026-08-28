package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// validateTaskRepositoryPolicies resolves every explicit policy before the task
// row is inserted. This keeps a stale or cross-repository policy from creating
// a task that cannot later materialize its requested branch workflow.
func (s *Service) validateTaskRepositoryPolicies(ctx context.Context, workspaceID string, inputs []TaskRepositoryInput) error {
	for index := range inputs {
		input := &inputs[index]
		if input.BranchPolicyID == "" {
			continue
		}
		if input.RepositoryID == "" {
			return fmt.Errorf("%w: branch policy selection requires a repository id", ErrInvalidRepositoryBranchPolicy)
		}
		repository, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
		if err != nil {
			return err
		}
		if repository == nil || repository.WorkspaceID != workspaceID {
			return repoerrors.ErrRepositoryNotFound
		}
		policy, err := s.resolveTaskRepositoryPolicy(ctx, input.RepositoryID, *input)
		if err != nil {
			return err
		}
		if input.RemoteContribution != nil && policy.BaseBranch != input.RemoteContribution.BaseBranch {
			return fmt.Errorf("remote contribution base branch %q does not match branch policy base branch %q", input.RemoteContribution.BaseBranch, policy.BaseBranch)
		}
		input.BranchPolicySnapshot = cloneBranchPolicy(policy)
	}
	return nil
}

// preserveTaskRepositoryPolicySnapshots carries the immutable policy fields
// across an association replacement. Fresh-branch creation replaces the row
// after the branch exists, so resolving the policy again could otherwise pick
// up a later edit or fail after the policy was deleted.
func preserveTaskRepositoryPolicySnapshots(inputs []TaskRepositoryInput, existing []*models.TaskRepository) {
	for index := range inputs {
		input := &inputs[index]
		if input.BranchPolicyID == "" || input.BranchPolicySnapshot != nil {
			continue
		}
		for _, repository := range existing {
			if repository.RepositoryID != input.RepositoryID || repository.BranchPolicyID != input.BranchPolicyID {
				continue
			}
			if snapshot := branchPolicyFromTaskRepository(repository); snapshot != nil {
				input.BranchPolicySnapshot = snapshot
			}
			break
		}
	}
}

func branchPolicyFromTaskRepository(repository *models.TaskRepository) *models.RepositoryBranchPolicy {
	if repository == nil || repository.BranchPolicyID == "" || repository.BranchPolicyName == "" ||
		repository.BranchPolicyBaseBranch == "" || repository.BranchPolicyBranchTemplate == "" ||
		repository.BranchPolicyPullRequestTarget == "" {
		return nil
	}
	return &models.RepositoryBranchPolicy{
		ID:                repository.BranchPolicyID,
		RepositoryID:      repository.RepositoryID,
		Name:              repository.BranchPolicyName,
		BaseBranch:        repository.BranchPolicyBaseBranch,
		BranchTemplate:    repository.BranchPolicyBranchTemplate,
		PullRequestTarget: repository.BranchPolicyPullRequestTarget,
	}
}

func (s *Service) resolveTaskRepositoryPolicy(ctx context.Context, repositoryID string, input TaskRepositoryInput) (*models.RepositoryBranchPolicy, error) {
	if input.BranchPolicyID == "" {
		return nil, nil
	}
	if input.BranchPolicySnapshot != nil {
		if input.BranchPolicySnapshot.ID == input.BranchPolicyID && input.BranchPolicySnapshot.RepositoryID == repositoryID {
			return cloneBranchPolicy(input.BranchPolicySnapshot), nil
		}
	}
	if s.branchPolicies == nil {
		return nil, ErrRepositoryBranchPolicyStoreMissing
	}
	policy, err := s.branchPolicies.GetRepositoryBranchPolicy(ctx, input.BranchPolicyID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrRepositoryBranchPolicyNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrInvalidRepositoryBranchPolicy, ErrRepositoryBranchPolicyStale)
		}
		return nil, fmt.Errorf("%w: resolve selected policy: %v", ErrInvalidRepositoryBranchPolicy, err)
	}
	if policy == nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRepositoryBranchPolicy, ErrRepositoryBranchPolicyStale)
	}
	if policy.RepositoryID != repositoryID {
		return nil, fmt.Errorf("%w: selected policy does not belong to repository", ErrInvalidRepositoryBranchPolicy)
	}
	return cloneBranchPolicy(policy), nil
}

func cloneBranchPolicy(policy *models.RepositoryBranchPolicy) *models.RepositoryBranchPolicy {
	if policy == nil {
		return nil
	}
	copy := *policy
	return &copy
}
