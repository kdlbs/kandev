package worktree

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// Repository contains repository information needed for script execution and
// worktree file materialization.
type Repository struct {
	ID            string
	SetupScript   string
	CleanupScript string
	// CopyFiles is the legacy newline/pattern list of gitignored files copied
	// into new worktrees (copy-only).
	CopyFiles string
	// WorktreeFiles are files materialized into each new worktree, each carrying
	// its own copy/symlink mode.
	WorktreeFiles []FileSpec
}

// RepositoryProvider provides access to repository information. Worktree
// creation may only know the repository's path (not its ID), so both lookups
// are supported.
type RepositoryProvider interface {
	GetRepository(ctx context.Context, repositoryID string) (*Repository, error)
	GetRepositoryByPath(ctx context.Context, localPath string) (*Repository, error)
}

// RepositoryService interface for task service repository operations.
type RepositoryService interface {
	GetRepository(ctx context.Context, id string) (*models.Repository, error)
	GetRepositoryByLocalPath(ctx context.Context, localPath string) (*models.Repository, error)
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

// GetRepository fetches repository information by ID from the task service.
func (a *RepositoryAdapter) GetRepository(ctx context.Context, repositoryID string) (*Repository, error) {
	repo, err := a.repoService.GetRepository(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	return modelToWorktreeRepository(repo), nil
}

// GetRepositoryByPath fetches repository information by local path. Returns
// (nil, nil) when no repository matches the path.
func (a *RepositoryAdapter) GetRepositoryByPath(ctx context.Context, localPath string) (*Repository, error) {
	repo, err := a.repoService.GetRepositoryByLocalPath(ctx, localPath)
	if err != nil {
		return nil, err
	}
	if repo == nil {
		return nil, nil
	}
	return modelToWorktreeRepository(repo), nil
}

// modelToWorktreeRepository maps a task-model repository to the worktree-local
// representation, normalizing each configured file's materialization mode.
func modelToWorktreeRepository(repo *models.Repository) *Repository {
	files := make([]FileSpec, 0, len(repo.WorktreeFiles))
	for _, f := range repo.WorktreeFiles {
		files = append(files, FileSpec{
			Path: f.Path,
			Mode: NormalizeFileMaterializeMode(f.Mode),
		})
	}
	return &Repository{
		ID:            repo.ID,
		SetupScript:   repo.SetupScript,
		CleanupScript: repo.CleanupScript,
		CopyFiles:     repo.CopyFiles,
		WorktreeFiles: files,
	}
}
