package scope

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

const (
	// CanonicalCoordinatorRepositoryEnv mirrors the deployment guard's
	// repository identity input. An unset value retains the local deployment's
	// established coordinator checkout identity.
	CanonicalCoordinatorRepositoryEnv     = "KANDEV_COORDINATOR_REPOSITORY"
	defaultCanonicalCoordinatorRepository = "/data/home/Code/coordinator"
)

type coordinatorRepositoryLookup interface {
	GetRepository(context.Context, string) (*models.Repository, error)
}

type coordinatorTaskRepositoryLister interface {
	ListTaskRepositories(context.Context, string) ([]*models.TaskRepository, error)
}

// IsCanonicalCoordinatorTask verifies the server-owned repository identity
// used to grant a normal Kanban task the narrow Coordinator control surface.
func IsCanonicalCoordinatorTask(ctx context.Context, task *models.Task, lookup any) bool {
	if task == nil || task.ID == "" || task.WorkspaceID == "" || task.ArchivedAt != nil {
		return false
	}
	taskRepository, ok := canonicalTaskRepository(ctx, task, lookup)
	if !ok {
		return false
	}
	reader, ok := lookup.(coordinatorRepositoryLookup)
	if !ok {
		return false
	}
	repository, err := reader.GetRepository(ctx, taskRepository.RepositoryID)
	if err != nil || repository == nil || repository.DeletedAt != nil ||
		repository.WorkspaceID != task.WorkspaceID {
		return false
	}
	return hasCanonicalCoordinatorPath(repository)
}

func canonicalTaskRepository(
	ctx context.Context,
	task *models.Task,
	lookup any,
) (*models.TaskRepository, bool) {
	repositories := task.Repositories
	if lister, ok := lookup.(coordinatorTaskRepositoryLister); ok {
		var err error
		repositories, err = lister.ListTaskRepositories(ctx, task.ID)
		if err != nil {
			return nil, false
		}
	}
	if len(repositories) != 1 || repositories[0] == nil {
		return nil, false
	}
	repository := repositories[0]
	return repository, repository.TaskID == task.ID && repository.RepositoryID != ""
}

func hasCanonicalCoordinatorPath(repository *models.Repository) bool {
	expected := strings.TrimSpace(os.Getenv(CanonicalCoordinatorRepositoryEnv))
	if expected == "" {
		expected = defaultCanonicalCoordinatorRepository
	}
	expected = filepath.Clean(expected)
	localPath := filepath.Clean(strings.TrimSpace(repository.LocalPath))
	if !filepath.IsAbs(expected) || localPath != expected {
		return false
	}
	resolved, err := filepath.EvalSymlinks(expected)
	if err != nil || filepath.Clean(resolved) != expected {
		return false
	}
	info, err := os.Stat(expected)
	return err == nil && info.IsDir()
}
