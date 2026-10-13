package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/stretchr/testify/require"
)

type fixedTaskLinks struct {
	repository.TaskRepoRepository
	links map[string][]*models.TaskRepository
}

func (f fixedTaskLinks) ListTaskRepositoriesByTaskIDs(context.Context, []string) (map[string][]*models.TaskRepository, error) {
	return f.links, nil
}

func TestListTaskRepositoryIDsByTaskIDs(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"}))
	for _, id := range []string{"t-none", "t-multi"} {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "ws-1", Title: id}))
	}
	for _, id := range []string{"repo-a", "repo-b"} {
		require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "ws-1", Name: id, LocalPath: "/p/" + id, DefaultBranch: "main"}))
		require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "l-" + id, TaskID: "t-multi", RepositoryID: id, BaseBranch: "main"}))
	}

	t.Run("empty id list", func(t *testing.T) {
		got, err := svc.ListTaskRepositoryIDsByTaskIDs(ctx, nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})
	t.Run("task without links has no key", func(t *testing.T) {
		got, err := svc.ListTaskRepositoryIDsByTaskIDs(ctx, []string{"t-none"})
		require.NoError(t, err)
		require.NotContains(t, got, "t-none")
	})
	t.Run("multiple repositories per task", func(t *testing.T) {
		got, err := svc.ListTaskRepositoryIDsByTaskIDs(ctx, []string{"t-none", "t-multi"})
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"repo-a", "repo-b"}, got["t-multi"])
		require.Len(t, got, 1)
	})
	t.Run("blank repository id is skipped", func(t *testing.T) {
		stub := &Service{taskRepos: fixedTaskLinks{links: map[string][]*models.TaskRepository{
			"t-blank": {{TaskID: "t-blank", RepositoryID: ""}, nil},
			"t-mixed": {{TaskID: "t-mixed", RepositoryID: ""}, {TaskID: "t-mixed", RepositoryID: "repo-a"}},
		}}}
		got, err := stub.ListTaskRepositoryIDsByTaskIDs(ctx, []string{"t-blank", "t-mixed"})
		require.NoError(t, err)
		require.NotContains(t, got, "t-blank")
		require.Equal(t, []string{"repo-a"}, got["t-mixed"])
	})
}
