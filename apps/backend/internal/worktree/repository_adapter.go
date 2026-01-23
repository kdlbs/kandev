package worktree

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// Repository contains repository information needed for script execution.
type Repository struct {
	ID            string
	SetupScript   string
	CleanupScript string
}

// RepositoryProvider provides access to repository information.
type RepositoryProvider interface {
	GetRepository(ctx context.Context, repositoryID string) (*Repository, error)
}

// RepositoryService interface for task service repository operations.
type RepositoryService interface {
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
}

// RepositoryAdapter adapts the task service repository interface to the worktree manager's needs.
type RepositoryAdapter struct {
	repoService RepositoryService
}

// NewRepositoryAdapter creates a new RepositoryAdapter.
func NewRepositoryAdapter(repoService RepositoryService) *RepositoryAdapter {
	return &RepositoryAdapter{
		repoService: repoService,
	}
}

// GetRepository fetches repository information from the task service.
func (a *RepositoryAdapter) GetRepository(ctx context.Context, repositoryID string) (*Repository, error) {
	repo, err := a.repoService.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}

	return &Repository{
		ID:            repo.ID,
		SetupScript:   repo.SetupScript,
		CleanupScript: repo.CleanupScript,
	}, nil
}
