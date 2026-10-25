package github

import (
	"context"
	"fmt"
)

// validateTaskRepositoryID accepts only the canonical repositories.ID for a
// task association. task_repositories.ID is a different row identifier and
// must not reach the github_task_prs repository_id column.
func (s *Service) validateTaskRepositoryID(ctx context.Context, taskID, repositoryID string) error {
	if repositoryID == "" {
		return nil
	}
	store := s.getTaskIssueStore()
	if store == nil {
		// Package-level consumers can construct Service without the task-facing
		// adapter. Keep the legacy path available for those consumers; the
		// backend wiring always installs the adapter before handling requests.
		return nil
	}
	taskRepos, err := store.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return fmt.Errorf("list task repositories: %w", err)
	}
	for _, taskRepo := range taskRepos {
		if taskRepo != nil && taskRepo.RepositoryID == repositoryID {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrTaskPRRepositoryMismatch, repositoryID)
}
