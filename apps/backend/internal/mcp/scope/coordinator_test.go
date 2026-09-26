package scope

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestIsCanonicalCoordinatorTaskRequiresServerOwnedTaskGrant(t *testing.T) {
	t.Setenv("KANDEV_COORDINATOR_TASK_ID", "coordinator")
	sharedRepository := &models.TaskRepository{RepositoryID: "repository"}
	task := &models.Task{ID: "coordinator", WorkspaceID: "workspace", Repositories: []*models.TaskRepository{sharedRepository}}
	require.True(t, IsCanonicalCoordinatorTask(context.Background(), task, nil))

	competing := &models.Task{ID: "competing", WorkspaceID: "workspace", Repositories: []*models.TaskRepository{sharedRepository}}
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), competing, nil),
		"a matching repository must not grant coordinator authority to another task")

	deletedAt := time.Now()
	task.ArchivedAt = &deletedAt
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, nil), "archived tasks must fail closed")
	task.ArchivedAt = nil

	t.Setenv("KANDEV_COORDINATOR_TASK_ID", "other-task")
	require.False(t, IsCanonicalCoordinatorTask(context.Background(), task, nil), "only the server grant's exact task ID may receive coordinator authority")
}
