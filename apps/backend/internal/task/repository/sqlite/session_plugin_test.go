package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestListTaskSessionsForPluginFiltersAndPaginatesInSQL(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	workspaceA := &models.Workspace{ID: "plugin-session-ws-a", Name: "Plugin session A"}
	workspaceB := &models.Workspace{ID: "plugin-session-ws-b", Name: "Plugin session B"}
	require.NoError(t, repo.CreateWorkspace(ctx, workspaceA))
	require.NoError(t, repo.CreateWorkspace(ctx, workspaceB))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "plugin-session-task-a", WorkspaceID: workspaceA.ID, Title: "A"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "plugin-session-task-b", WorkspaceID: workspaceB.ID, Title: "B"}))

	base := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	for _, session := range []*models.TaskSession{
		{ID: "plugin-session-a-old", TaskID: "plugin-session-task-a", State: models.TaskSessionStateRunning, StartedAt: base, UpdatedAt: base},
		{ID: "plugin-session-a-new", TaskID: "plugin-session-task-a", State: models.TaskSessionStateWaitingForInput, StartedAt: base.Add(time.Hour), UpdatedAt: base.Add(time.Hour)},
		{ID: "plugin-session-b", TaskID: "plugin-session-task-b", State: models.TaskSessionStateRunning, StartedAt: base.Add(2 * time.Hour), UpdatedAt: base.Add(2 * time.Hour)},
	} {
		require.NoError(t, repo.CreateTaskSession(ctx, session))
	}

	filter := models.PluginSessionFilter{
		WorkspaceIDs:    []string{workspaceA.ID},
		States:          []models.TaskSessionState{models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput},
		UpdatedSince:    ptrTime(base.Add(-time.Minute)),
		ExcludeInternal: true,
		Limit:           2,
	}
	page, err := repo.ListTaskSessionsForPlugin(ctx, filter)
	require.NoError(t, err)
	require.Len(t, page, 2)
	require.Equal(t, "plugin-session-a-new", page[0].ID)
	require.Equal(t, "plugin-session-a-old", page[1].ID)

	filter.Offset = 1
	filter.Limit = 1
	page, err = repo.ListTaskSessionsForPlugin(ctx, filter)
	require.NoError(t, err)
	require.Len(t, page, 1)
	require.Equal(t, "plugin-session-a-old", page[0].ID)
}

func ptrTime(value time.Time) *time.Time { return &value }
