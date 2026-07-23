package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

type referenceConversationResolver interface {
	ResolveWorkspace(context.Context, string, string) (string, error)
}

func TestResolveWorkspaceUsesPersistedSessionTaskScope(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	for _, workspace := range []*models.Workspace{
		{ID: "ws-1", Name: "One"},
		{ID: "ws-2", Name: "Two"},
	} {
		require.NoError(t, repo.CreateWorkspace(ctx, workspace))
	}
	for _, task := range []*models.Task{
		{ID: "task-1", WorkspaceID: "ws-1", Title: "One"},
		{ID: "task-2", WorkspaceID: "ws-2", Title: "Two"},
	} {
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "passthrough-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
		IsPassthrough: true,
	}))

	resolver, ok := any(svc).(referenceConversationResolver)
	require.True(t, ok)
	workspaceID, err := resolver.ResolveWorkspace(ctx, "session-1", "task-1")
	require.NoError(t, err)
	require.Equal(t, "ws-1", workspaceID)

	_, err = resolver.ResolveWorkspace(ctx, "session-1", "task-2")
	require.Error(t, err)
	require.True(t, errors.Is(err, errReferenceConversationScope))

	_, err = resolver.ResolveWorkspace(ctx, "passthrough-1", "task-1")
	require.Error(t, err)
	require.True(t, errors.Is(err, errReferenceConversationScope))
}
