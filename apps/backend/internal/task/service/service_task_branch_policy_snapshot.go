package service

import (
	"context"
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
		if s.branchPolicies == nil {
			return ErrRepositoryBranchPolicyStoreMissing
		}
		repository, err := s.repoEntities.GetRepository(ctx, input.RepositoryID)
		if err != nil {
			return err
		}
		if repository == nil || repository.WorkspaceID != workspaceID {
			return repoerrors.ErrRepositoryNotFound
		}
		policy, err := s.branchPolicies.GetRepositoryBranchPolicy(ctx, input.BranchPolicyID)
		if err != nil {
			return fmt.Errorf("%w: selected policy is no longer available", ErrInvalidRepositoryBranchPolicy)
		}
		if policy.RepositoryID != input.RepositoryID {
			return fmt.Errorf("%w: selected policy does not belong to repository", ErrInvalidRepositoryBranchPolicy)
		}
		input.BranchPolicySnapshot = cloneBranchPolicy(policy)
	}
	return nil
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
		return nil, fmt.Errorf("%w: selected policy is no longer available", ErrInvalidRepositoryBranchPolicy)
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
