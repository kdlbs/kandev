package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type taskMentionService interface {
	SearchMentionTasks(
		ctx context.Context,
		workspaceID, query, excludeTaskID string,
		limit int,
	) ([]*models.Task, error)
}

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

type taskMentionRepositoryStub struct {
	repository.TaskRepository
	workspaceID   string
	query         string
	excludeTaskID string
	limit         int
	result        []*models.Task
	err           error
}

func (r *taskMentionRepositoryStub) SearchMentionTasks(
	_ context.Context,
	workspaceID, query, excludeTaskID string,
	limit int,
) ([]*models.Task, error) {
	r.workspaceID = workspaceID
	r.query = query
	r.excludeTaskID = excludeTaskID
	r.limit = limit
	return r.result, r.err
}

func TestSearchMentionTasksDelegatesToMentionRepository(t *testing.T) {
	want := []*models.Task{{ID: "task-1", Title: "Auth failure"}}
	repo := &taskMentionRepositoryStub{result: want}
	svc := &Service{tasks: repo}
	searcher, ok := any(svc).(taskMentionService)
	require.True(t, ok, "Service must expose task mention search")

	got, err := searcher.SearchMentionTasks(context.Background(), "ws-1", "auth", "current", 7)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, "ws-1", repo.workspaceID)
	require.Equal(t, "auth", repo.query)
	require.Equal(t, "current", repo.excludeTaskID)
	require.Equal(t, 7, repo.limit)
}

func TestSearchMentionTasksRejectsRepositoryWithoutMentionSearch(t *testing.T) {
	svc := &Service{tasks: struct{ repository.TaskRepository }{}}
	searcher, ok := any(svc).(taskMentionService)
	require.True(t, ok, "Service must expose task mention search")

	got, err := searcher.SearchMentionTasks(context.Background(), "ws-1", "auth", "", 7)
	require.Nil(t, got)
	require.EqualError(t, err, "task mention search is unavailable")
}
