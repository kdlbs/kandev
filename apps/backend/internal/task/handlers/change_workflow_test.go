package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestHTTPWorkflowChangeRejectsStaleSourceBeforeMoving(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	updatedAt := time.Date(2026, time.September, 23, 10, 0, 0, 0, time.UTC)
	task := &models.Task{
		ID:             "task-workflow-change",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-source",
		UpdatedAt:      updatedAt,
	}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: task.ID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks/"+task.ID+"/move", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"workflow_change": {
			"expected_workflow_id": "wf-stale",
			"expected_step_id": "step-source",
			"expected_updated_at": "2026-09-23T10:00:00Z",
			"agent_overrides": {}
		}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpMoveTask(c)

	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, "wf-source", repo.task.WorkflowID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestHTTPWorkflowChangeRequiresAnAgentOverridesObject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	updatedAt := time.Date(2026, time.September, 23, 10, 0, 0, 0, time.UTC)
	task := &models.Task{
		ID:             "task-workflow-change-missing-map",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-source",
		UpdatedAt:      updatedAt,
	}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: task.ID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks/"+task.ID+"/move", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"workflow_change": {
			"expected_workflow_id": "wf-source",
			"expected_step_id": "step-source",
			"expected_updated_at": "2026-09-23T10:00:00Z"
		}
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpMoveTask(c)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	require.Equal(t, "wf-source", repo.task.WorkflowID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestWSWorkflowChangeReturnsStableConflictCode(t *testing.T) {
	log := newTestLogger(t)
	updatedAt := time.Date(2026, time.September, 23, 10, 0, 0, 0, time.UTC)
	task := &models.Task{
		ID: "task-ws-workflow-change", WorkspaceID: "workspace-1",
		WorkflowID: "wf-source", WorkflowStepID: "step-source", UpdatedAt: updatedAt,
	}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	h := &TaskHandlers{service: svc, logger: log}
	request, err := ws.NewRequest("move-1", ws.ActionTaskMove, map[string]any{
		"id": task.ID, "workflow_id": "wf-target", "workflow_step_id": "step-target",
		"workflow_change": map[string]any{
			"expected_workflow_id": "wf-outdated", "expected_step_id": "step-source",
			"expected_updated_at": updatedAt, "agent_overrides": map[string]string{},
		},
	})
	require.NoError(t, err)

	response, err := h.wsMoveTask(context.Background(), request)
	require.NoError(t, err)
	payload := wsWorkflowError(t, response)
	require.Equal(t, string(ws.ErrorCodeConflict), payload.Code)
	require.Equal(t, "workflow_change_conflict", payload.Message)
	require.Equal(t, "wf-source", repo.task.WorkflowID)
}
