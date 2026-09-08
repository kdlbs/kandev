package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/task/repository"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type fakeCancellationPendingProvider struct {
	pending bool
}

func (p fakeCancellationPendingProvider) CancellationPending(string) bool {
	return p.pending
}

type orchestratorWithCancellation struct {
	captureOrchestrator
	pending bool
}

type messageOrchestratorWithCancellation struct {
	firstTurnCaptureOrchestrator
	pendingBySession map[string]bool
}

type cancellationListRepo struct {
	// Membership is not exercised by this fake; the embedded default
	// reports no membership, which is the narrower answer.
	repository.UnsupportedWorkspaceMembers
	mockRepository
	sessionsByTask []*models.TaskSession
	pendingActions map[string]models.TaskPendingAction
	pendingErr     error
}

func (r *cancellationListRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return r.sessionsByTask, nil
}

func (r *cancellationListRepo) CountToolCallMessagesBySession(context.Context, []string) (map[string]int, error) {
	return nil, nil
}

func (r *cancellationListRepo) GetPendingActionsBySessionIDs(
	context.Context,
	[]string,
) (map[string]models.TaskPendingAction, error) {
	return r.pendingActions, r.pendingErr
}

func (o *orchestratorWithCancellation) CancellationPending(string) bool {
	return o.pending
}

func (o messageOrchestratorWithCancellation) CancellationPending(sessionID string) bool {
	return o.pendingBySession[sessionID]
}

func TestNewTaskHandlers_DerivesCancellationPendingProvider(t *testing.T) {
	withCancellation := &orchestratorWithCancellation{pending: true}
	h := NewTaskHandlers(nil, withCancellation, nil, nil, newTestLogger(t))
	require.NotNil(t, h.cancellationPending)
	require.True(t, h.cancellationPending.CancellationPending("session-1"))

	plain := &captureOrchestrator{}
	h2 := NewTaskHandlers(nil, plain, nil, nil, newTestLogger(t))
	require.Nil(t, h2.cancellationPending)
}

func TestNewMessageHandlers_DerivesCancellationPendingProvider(t *testing.T) {
	withCancellation := &messageOrchestratorWithCancellation{
		pendingBySession: map[string]bool{"session-1": true},
	}
	h := NewMessageHandlers(nil, withCancellation, newTestLogger(t))
	require.NotNil(t, h.cancellationPending)
	require.True(t, h.cancellationPending.CancellationPending("session-1"))

	plain := &firstTurnCaptureOrchestrator{}
	h2 := NewMessageHandlers(nil, plain, newTestLogger(t))
	require.Nil(t, h2.cancellationPending)
}

func TestHTTPGetTaskSession_StampsCancellationPending(t *testing.T) {
	svc, _ := newSessionHandlerService(t, &models.TaskSession{
		ID:    "sess-cancel",
		State: models.TaskSessionStateRunning,
	})
	h := &TaskHandlers{
		service:             svc,
		cancellationPending: fakeCancellationPendingProvider{pending: true},
		logger:              newTestLogger(t),
	}

	resp := doGetTaskSession(t, h, "sess-cancel")
	require.True(t, resp.Session.CancellationPending)
	require.NotNil(t, resp.Session.PendingActionRevision)
	require.NotEmpty(t, resp.Session.PendingActionRevision.Epoch)
}

func TestHTTPListTaskSessions_StampsCancellationPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &models.TaskSession{ID: "sess-cancel", TaskID: "task-1", State: models.TaskSessionStateRunning}
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
		service:             svc,
		repo:                repo,
		cancellationPending: fakeCancellationPendingProvider{pending: true},
		logger:              newTestLogger(t),
	}

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/tasks/task-1/sessions", nil).WithContext(context.Background())
	c.Params = gin.Params{{Key: "id", Value: "task-1"}}
	h.httpListTaskSessions(c)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var response dto.ListTaskSessionSummariesResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Len(t, response.Sessions, 1)
	require.True(t, response.Sessions[0].CancellationPending)
	require.NotNil(t, response.Sessions[0].PendingActionRevision)
	require.NotEmpty(t, response.Sessions[0].PendingActionRevision.Epoch)
}

func TestListTaskSessionsFailsWhenPendingProjectionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	session := &models.TaskSession{
		ID: "sess-pending", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
	}
	repo := &cancellationListRepo{
		mockRepository: mockRepository{sessions: map[string]*models.TaskSession{session.ID: session}},
		sessionsByTask: []*models.TaskSession{session},
		pendingErr:     errors.New("pending projection unavailable"),
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, repo: repo, logger: newTestLogger(t)}

	t.Run("http", func(t *testing.T) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/tasks/task-1/sessions", nil)
		c.Params = gin.Params{{Key: "id", Value: "task-1"}}
		h.httpListTaskSessions(c)
		require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("websocket", func(t *testing.T) {
		request, err := ws.NewRequest("list", ws.ActionTaskSessionList, map[string]string{"task_id": "task-1"})
		require.NoError(t, err)
		response, err := h.doListTaskSessions(context.Background(), request, "task-1")
		require.NoError(t, err)
		var body ws.ErrorPayload
		require.NoError(t, response.ParsePayload(&body))
		require.Equal(t, string(ws.ErrorCodeInternalError), body.Code)
	})
}

func TestTaskSessionSummariesWithPendingActionsCentralizesInputGating(t *testing.T) {
	sessions := []*models.TaskSession{
		{ID: "waiting", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
		{ID: "running", TaskID: "task-1", State: models.TaskSessionStateRunning},
		{ID: "completed", TaskID: "task-1", State: models.TaskSessionStateCompleted},
	}
	repo := &cancellationListRepo{
		mockRepository: mockRepository{sessions: map[string]*models.TaskSession{}},
		sessionsByTask: sessions,
		pendingActions: map[string]models.TaskPendingAction{
			"waiting":   models.TaskPendingActionClarification,
			"running":   models.TaskPendingActionPermission,
			"completed": models.TaskPendingActionClarification,
		},
	}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo,
		Messages: repo, Turns: repo, Sessions: repo, GitSnapshots: repo,
		RepoEntities: repo, Executors: repo, Environments: repo,
		TaskEnvironments: repo, Reviews: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: newTestLogger(t)}

	summaries, err := h.taskSessionSummariesWithPendingActions(context.Background(), sessions)
	require.NoError(t, err)
	require.Len(t, summaries, 3)
	require.Equal(t, string(models.TaskPendingActionClarification), *summaries[0].PendingAction)
	require.Equal(t, string(models.TaskPendingActionPermission), *summaries[1].PendingAction)
	require.Nil(t, summaries[2].PendingAction)
	for _, summary := range summaries {
		require.NotNil(t, summary.PendingActionRevision)
		require.NotEmpty(t, summary.PendingActionRevision.Epoch)
	}
}

func TestWSListTaskSessions_StampsCancellationPending(t *testing.T) {
	session := &models.TaskSession{ID: "sess-cancel", TaskID: "task-1", State: models.TaskSessionStateRunning}
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
		service:             svc,
		cancellationPending: fakeCancellationPendingProvider{pending: true},
		logger:              newTestLogger(t),
	}

	request, err := ws.NewRequest("list", ws.ActionTaskSessionList, map[string]string{"task_id": "task-1"})
	require.NoError(t, err)
	response, err := h.doListTaskSessions(context.Background(), request, "task-1")
	require.NoError(t, err)
	var body dto.ListTaskSessionSummariesResponse
	require.NoError(t, response.ParsePayload(&body))
	require.Len(t, body.Sessions, 1)
	require.True(t, body.Sessions[0].CancellationPending)
	require.NotNil(t, body.Sessions[0].PendingActionRevision)
	require.NotEmpty(t, body.Sessions[0].PendingActionRevision.Epoch)
}

func newMessageCancellationService(t *testing.T, repo *messageAddSwitchRepo) *service.Service {
	t.Helper()
	return service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, newTestLogger(t), service.RepositoryDiscoveryConfig{})
}

func TestResolveSessionAfterTurnStart_EnrichesSubmittedSessionCancellation(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-1": {ID: "task-1"}},
		sessions: map[string]*models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID: "session-1",
	}
	h := NewMessageHandlers(newMessageCancellationService(t, repo), &messageOrchestratorWithCancellation{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{},
		pendingBySession:             map[string]bool{"session-1": true},
	}, newTestLogger(t))

	current := &dto.GetTaskSessionResponse{Session: dto.FromTaskSession(repo.sessions["session-1"])}
	response, err := h.resolveSessionAfterTurnStart(context.Background(), "task-1", "session-1", current)
	require.NoError(t, err)
	require.Equal(t, "session-1", response.Session.ID)
	require.True(t, response.Session.CancellationPending)
}

func TestResolveSessionAfterTurnStart_EnrichesReplacementSessionCancellation(t *testing.T) {
	now := time.Now().UTC()
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-1": {ID: "task-1"}},
		sessions: map[string]*models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCompleted, UpdatedAt: now},
			"session-2": {ID: "session-2", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput, UpdatedAt: now},
		},
		primaryID: "session-2",
	}
	h := NewMessageHandlers(newMessageCancellationService(t, repo), &messageOrchestratorWithCancellation{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{},
		pendingBySession:             map[string]bool{"session-1": true, "session-2": false},
	}, newTestLogger(t))

	current := &dto.GetTaskSessionResponse{Session: dto.FromTaskSession(repo.sessions["session-1"])}
	response, err := h.resolveSessionAfterTurnStart(context.Background(), "task-1", "session-1", current)
	require.NoError(t, err)
	require.Equal(t, "session-2", response.Session.ID)
	require.False(t, response.Session.CancellationPending)
}

func TestCheckSessionStateForMessage_EnrichesCancellation(t *testing.T) {
	repo := &messageAddSwitchRepo{
		tasks: map[string]*models.Task{"task-1": {ID: "task-1"}},
		sessions: map[string]*models.TaskSession{
			"session-1": {ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput},
		},
	}
	h := NewMessageHandlers(newMessageCancellationService(t, repo), &messageOrchestratorWithCancellation{
		firstTurnCaptureOrchestrator: firstTurnCaptureOrchestrator{},
		pendingBySession:             map[string]bool{"session-1": true},
	}, newTestLogger(t))
	request, err := ws.NewRequest("check", ws.ActionMessageAdd, nil)
	require.NoError(t, err)

	response, wsErr := h.checkSessionStateForMessage(context.Background(), request, "session-1")
	require.Nil(t, wsErr)
	require.True(t, response.Session.CancellationPending)
}

var _ dto.CancellationPendingProvider = fakeCancellationPendingProvider{}
