package azuredevops

import (
	"context"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/common/securityutil"
)

// WatchRepositoryLookup validates the Kandev repository selected for an
// Azure watcher and supplies its default branch when the form leaves the
// branch blank. It intentionally differs from RepositoryLookup, which
// validates task↔Azure PR associations.
type WatchRepositoryLookup interface {
	GetRepository(ctx context.Context, id string) (workspaceID, defaultBranch string, ok bool)
}

// WatchDependencyValidator validates references that are used when a watcher
// creates a Kandev task. Keeping this seam narrow lets the integration remain
// independent of workflow and settings storage while still failing closed in
// production.
type WatchDependencyValidator interface {
	WorkflowStepBelongs(ctx context.Context, workspaceID, workflowID, stepID string) (bool, error)
	AgentProfileBelongs(ctx context.Context, workspaceID, profileID string) (bool, error)
	ExecutorProfileBelongs(ctx context.Context, workspaceID, profileID string) (bool, error)
}

func (s *Service) SetWatchRepositoryLookup(lookup WatchRepositoryLookup) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchRepositoryLookup = lookup
}

func (s *Service) SetWatchDependencyValidator(validator WatchDependencyValidator) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchDependencyValidator = validator
}

func (s *Service) validateWatchDependencies(
	ctx context.Context,
	workspaceID, workflowID, workflowStepID, agentProfileID, executorProfileID string,
) error {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(workflowID) == "" ||
		strings.TrimSpace(workflowStepID) == "" || strings.TrimSpace(agentProfileID) == "" ||
		strings.TrimSpace(executorProfileID) == "" {
		return fmt.Errorf("%w: workflow, step, agent profile, and executor profile are required", ErrInvalidConfig)
	}
	s.mu.RLock()
	validator := s.watchDependencyValidator
	s.mu.RUnlock()
	if validator == nil {
		return nil
	}
	checks := []func() (bool, error){
		func() (bool, error) {
			return validator.WorkflowStepBelongs(ctx, workspaceID, workflowID, workflowStepID)
		},
		func() (bool, error) { return validator.AgentProfileBelongs(ctx, workspaceID, agentProfileID) },
		func() (bool, error) {
			return validator.ExecutorProfileBelongs(ctx, workspaceID, executorProfileID)
		},
	}
	for _, check := range checks {
		valid, err := check()
		if err != nil {
			return fmt.Errorf("validate Azure DevOps watcher dependencies: %w", err)
		}
		if !valid {
			return fmt.Errorf("%w: watcher dependency does not belong to workspace", ErrInvalidConfig)
		}
	}
	return nil
}

func (s *Service) resolveWatchRepository(
	ctx context.Context, workspaceID, repositoryID, baseBranch string,
) (string, string, error) {
	repositoryID = strings.TrimSpace(repositoryID)
	baseBranch = strings.TrimSpace(baseBranch)
	if repositoryID == "" {
		return "", "", fmt.Errorf("%w: Kandev repository is required", ErrInvalidConfig)
	}
	if baseBranch != "" && !securityutil.IsValidBaseBranchRef(baseBranch) {
		return "", "", fmt.Errorf("%w: base branch %q is not a valid git ref", ErrInvalidConfig, baseBranch)
	}
	s.mu.RLock()
	lookup := s.watchRepositoryLookup
	s.mu.RUnlock()
	if lookup == nil {
		return repositoryID, baseBranch, nil
	}
	repoWorkspace, defaultBranch, ok := lookup.GetRepository(ctx, repositoryID)
	if !ok || repoWorkspace != workspaceID {
		return "", "", fmt.Errorf("%w: repository %q not found in workspace", ErrInvalidConfig, repositoryID)
	}
	if baseBranch == "" {
		baseBranch = defaultBranch
	}
	return repositoryID, baseBranch, nil
}
