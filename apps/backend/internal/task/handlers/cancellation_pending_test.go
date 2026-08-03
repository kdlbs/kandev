package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
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
	pending bool
}

type cancellationListRepo struct {
	mockRepository
	sessionsByTask []*models.TaskSession
}

func (r *cancellationListRepo) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return r.sessionsByTask, nil
}

func (r *cancellationListRepo) CountToolCallMessagesBySession(context.Context, []string) (map[string]int, error) {
	return nil, nil
}

func (o *orchestratorWithCancellation) CancellationPending(string) bool {
	return o.pending
}

func (o messageOrchestratorWithCancellation) CancellationPending(string) bool {
	return o.pending
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
	withCancellation := &messageOrchestratorWithCancellation{pending: true}
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
}

var _ dto.CancellationPendingProvider = fakeCancellationPendingProvider{}
