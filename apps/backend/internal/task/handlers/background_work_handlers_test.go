package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type bgWorkMockRepo struct {
	mockRepository
	workloads map[string]*models.BackgroundWorkload
}

func (r *bgWorkMockRepo) ListBackgroundWorkloadsBySession(ctx context.Context, sessionID string) ([]*models.BackgroundWorkload, error) {
	var list []*models.BackgroundWorkload
	for _, w := range r.workloads {
		if w.SessionID == sessionID {
			list = append(list, w)
		}
	}
	return list, nil
}

func (r *bgWorkMockRepo) GetBackgroundWorkload(ctx context.Context, sessionID, id string) (*models.BackgroundWorkload, error) {
	w := r.workloads[id]
	if w != nil && w.SessionID == sessionID {
		return w, nil
	}
	return nil, nil
}

func (r *bgWorkMockRepo) UpsertBackgroundWorkload(ctx context.Context, workload *models.BackgroundWorkload) error {
	r.workloads[workload.ID] = workload
	return nil
}

func (r *bgWorkMockRepo) DeleteBackgroundWorkloadsBySession(ctx context.Context, sessionID string) error {
	return nil
}

func (r *bgWorkMockRepo) UpsertBackgroundRun(ctx context.Context, run *models.BackgroundRun) error {
	return nil
}

func (r *bgWorkMockRepo) GetBackgroundRun(ctx context.Context, sessionID, id string) (*models.BackgroundRun, error) {
	return nil, nil
}

func (r *bgWorkMockRepo) ListBackgroundRunsByWorkload(ctx context.Context, sessionID, workloadID string) ([]*models.BackgroundRun, error) {
	return nil, nil
}

func (r *bgWorkMockRepo) ReserveBackgroundActionReceipt(ctx context.Context, receipt *models.BackgroundActionReceipt) error {
	return nil
}

func (r *bgWorkMockRepo) RecordBackgroundActionReceipt(ctx context.Context, receipt *models.BackgroundActionReceipt) error {
	return nil
}

func (r *bgWorkMockRepo) GetBackgroundActionReceipt(ctx context.Context, sessionID, operationID string) (*models.BackgroundActionReceipt, error) {
	return nil, nil
}

func (r *bgWorkMockRepo) GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error) {
	return &models.TaskSession{ID: id, TaskID: "task-1"}, nil
}

type mockDispatcher struct{}

func (d *mockDispatcher) ExecuteBackgroundWorkAction(ctx context.Context, executionID string, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error) {
	return streams.BackgroundWorkActionResponse{
		Success: true,
		WorkID:  req.WorkID,
		Action:  req.Action,
	}, nil
}

func newTestBackgroundWorkHandlers(t *testing.T) (*TaskHandlers, *bgWorkMockRepo) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	repo := &bgWorkMockRepo{
		workloads: map[string]*models.BackgroundWorkload{
			"work-1": {
				ID:        "work-1",
				TaskID:    "task-1",
				SessionID: "session-1",
				Kind:      "shell",
				Title:     "Build",
				State:     "running",
			},
		},
	}
	svc := service.NewService(service.Repos{
		Workspaces:     repo,
		Tasks:          repo,
		Sessions:       repo,
		BackgroundWork: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetBackgroundWorkActionDispatcher(&mockDispatcher{})
	h := &TaskHandlers{service: svc, logger: log}
	h.SetBackgroundWorkEnabled(true)
	return h, repo
}

func TestHTTPListBackgroundWorkloads(t *testing.T) {
	h, _ := newTestBackgroundWorkHandlers(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/session-1/background-work", nil)
	c.Params = gin.Params{{Key: "id", Value: "session-1"}}

	h.httpListBackgroundWorkloads(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "work-1")
}

func TestHTTPGetBackgroundWorkload(t *testing.T) {
	h, _ := newTestBackgroundWorkHandlers(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/task-sessions/session-1/background-work/work-1", nil)
	c.Params = gin.Params{{Key: "id", Value: "session-1"}, {Key: "workId", Value: "work-1"}}

	h.httpGetBackgroundWorkload(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Build")
}

func TestHTTPExecuteBackgroundAction(t *testing.T) {
	h, _ := newTestBackgroundWorkHandlers(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/task-sessions/session-1/background-work/work-1/action", strings.NewReader(`{"action":"stop"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "session-1"}, {Key: "workId", Value: "work-1"}}

	h.httpExecuteBackgroundAction(c)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"success":true`)
}

func TestWSBackgroundWorkHandlers(t *testing.T) {
	h, _ := newTestBackgroundWorkHandlers(t)
	ctx := context.Background()

	t.Run("wsListBackgroundWorkloads", func(t *testing.T) {
		msg, err := ws.NewRequest("req-1", ws.ActionSessionBackgroundWorkList, map[string]interface{}{"session_id": "session-1"})
		require.NoError(t, err)
		resp, err := h.wsListBackgroundWorkloads(ctx, msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)
	})

	t.Run("wsGetBackgroundWorkload", func(t *testing.T) {
		msg, err := ws.NewRequest("req-2", ws.ActionSessionBackgroundWorkGet, map[string]interface{}{"session_id": "session-1", "work_id": "work-1"})
		require.NoError(t, err)
		resp, err := h.wsGetBackgroundWorkload(ctx, msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)
	})

	t.Run("wsExecuteBackgroundAction", func(t *testing.T) {
		msg, err := ws.NewRequest("req-3", ws.ActionSessionBackgroundWorkAction, map[string]interface{}{
			"session_id": "session-1",
			"work_id":    "work-1",
			"action":     "stop",
		})
		require.NoError(t, err)
		resp, err := h.wsExecuteBackgroundAction(ctx, msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, resp.Type)
	})
}
