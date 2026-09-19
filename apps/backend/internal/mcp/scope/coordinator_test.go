package scope

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type coordinatorLookupFake struct {
	repositories     map[string]*models.Repository
	taskRepositories map[string][]*models.TaskRepository
}

func (f coordinatorLookupFake) GetRepository(_ context.Context, id string) (*models.Repository, error) {
	return f.repositories[id], nil
}

func (f coordinatorLookupFake) ListTaskRepositories(_ context.Context, taskID string) ([]*models.TaskRepository, error) {
	return f.taskRepositories[taskID], nil
}

func TestIsCanonicalCoordinatorTaskRequiresExactLiveRepositoryIdentity(t *testing.T) {
	canonical := t.TempDir()
	t.Setenv(CanonicalCoordinatorRepositoryEnv, canonical)
	task := &models.Task{ID: "coordinator", WorkspaceID: "workspace"}
	lookup := coordinatorLookupFake{
		repositories: map[string]*models.Repository{
			"repository": {ID: "repository", WorkspaceID: "workspace", LocalPath: canonical},
		},
		taskRepositories: map[string][]*models.TaskRepository{
			"coordinator": {{TaskID: "coordinator", RepositoryID: "repository"}},
		},
	}
	require.True(t, IsCanonicalCoordinatorTask(context.Background(), task, lookup))

	lookup.taskRepositories["coordinator"] = append(lookup.taskRepositories["coordinator"],
		&models.TaskRepository{TaskID: "coordinator", RepositoryID: "other"})
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, lookup), "multiple repositories must fail closed")
	lookup.taskRepositories["coordinator"] = lookup.taskRepositories["coordinator"][:1]

	lookup.repositories["repository"].WorkspaceID = "other-workspace"
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, lookup), "workspace mismatch must fail closed")
	lookup.repositories["repository"].WorkspaceID = "workspace"

	deletedAt := time.Now()
	lookup.repositories["repository"].DeletedAt = &deletedAt
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, lookup), "deleted repository must fail closed")
	lookup.repositories["repository"].DeletedAt = nil

	link := filepath.Join(t.TempDir(), "coordinator")
	require.NoError(t, os.Symlink(canonical, link))
	t.Setenv(CanonicalCoordinatorRepositoryEnv, link)
	lookup.repositories["repository"].LocalPath = link
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, lookup), "symlinked identity must fail closed")
}
