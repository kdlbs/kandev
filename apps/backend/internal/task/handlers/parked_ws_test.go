package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// fakeParkedProjectionProvider is a test double for dto.ParkedProjectionProvider.
type fakeParkedProjectionProvider struct {
	parked bool
}

func (p fakeParkedProjectionProvider) ParkedProjectionSnapshot(string) bool {
	return p.parked
}

// TestWSListTaskSessions_StampsParkedOnBackgroundWork closes MUST-FIX 2 from
// review round 1: doListTaskSessions (the ws.list_task_sessions handler)
// enriched foreground activity and cancellation-pending summaries but never
// called dto.EnrichParkedSummary, unlike its HTTP sibling
// httpListTaskSessions — so a genuinely parked session's WS-delivered
// summary always reported parked_on_background_work false/absent.
func TestWSListTaskSessions_StampsParkedOnBackgroundWork(t *testing.T) {
	session := &models.TaskSession{ID: "sess-parked", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput}
	repo := &cancellationListRepo{
		mockRepository: mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}},
		sessionsByTask: []*models.TaskSession{session},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{
		service:          svc,
		parkedProjection: fakeParkedProjectionProvider{parked: true},
		logger:           newTestLogger(t),
	}

	request, err := ws.NewRequest("list", ws.ActionTaskSessionList, map[string]string{"task_id": "task-1"})
	require.NoError(t, err)
	response, err := h.doListTaskSessions(context.Background(), request, "task-1")
	require.NoError(t, err)
	var body dto.ListTaskSessionSummariesResponse
	require.NoError(t, response.ParsePayload(&body))
	require.Len(t, body.Sessions, 1)
	require.True(t, body.Sessions[0].ParkedOnBackgroundWork,
		"ws.list_task_sessions must stamp parked_on_background_work exactly as httpListTaskSessions does")
}
