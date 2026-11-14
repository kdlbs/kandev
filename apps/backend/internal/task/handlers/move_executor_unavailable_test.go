package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

const (
	testUnavailableExecutorProfileID = "deleted-profile-secret-id"
	testUnavailableExecutorMessage   = "The task's executor is no longer available for this step."
)

type unavailableExecutorMovePreflight struct{}

func (unavailableExecutorMovePreflight) PreflightWorkflowStepMove(
	context.Context,
	string,
	*models.TaskSession,
	*wfmodels.WorkflowStep,
) error {
	return fmt.Errorf("executor profile %s: %w", testUnavailableExecutorProfileID, workflowmove.ErrDestinationExecutorUnavailable)
}

func newUnavailableExecutorMoveHandler(t *testing.T) (*TaskHandlers, *moveTaskConflictRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	task := &models.Task{
		ID:             "task-unavailable-executor",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-source",
		State:          v1.TaskStateTODO,
	}
	repo := &moveTaskConflictRepo{task: task}
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Name: "Review"},
	}})
	svc.SetWorkflowMovePreflight(unavailableExecutorMovePreflight{})
	return &TaskHandlers{service: svc, logger: log}, repo
}

func (m *moveTaskConflictRepo) ListTasksByWorkflowStep(_ context.Context, stepID string) ([]*models.Task, error) {
	if m.task != nil && m.task.WorkflowStepID == stepID {
		return []*models.Task{m.task}, nil
	}
	return nil, nil
}

func TestHTTPMoveTaskExecutorUnavailableUsesSafeConflict(t *testing.T) {
	h, repo := newUnavailableExecutorMoveHandler(t)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: repo.task.ID}}
	c.Request = httptest.NewRequest(http.MethodPost, "/tasks/"+repo.task.ID+"/move", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"position": 0
	}`))
	c.Request.Header.Set("Content-Type", "application/json")

	h.httpMoveTask(c)

	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, moveConflictCodeExecutorUnavailable, body["code"])
	require.Equal(t, testUnavailableExecutorMessage, body["error"])
	require.NotContains(t, rec.Body.String(), testUnavailableExecutorProfileID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestWSMoveTaskExecutorUnavailableUsesSafeConflict(t *testing.T) {
	h, repo := newUnavailableExecutorMoveHandler(t)
	request, err := ws.NewRequest("move-unavailable-executor", ws.ActionTaskMove, map[string]any{
		"id": repo.task.ID, "workflow_id": "wf-target", "workflow_step_id": "step-target",
	})
	require.NoError(t, err)
	response, err := h.wsMoveTask(context.Background(), request)
	require.NoError(t, err)

	var payload ws.ErrorPayload
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, ws.ErrorCodeConflict, payload.Code)
	require.Equal(t, testUnavailableExecutorMessage, payload.Message)
	require.Equal(t, moveConflictCodeExecutorUnavailable, payload.Details[errorDetailKeyErrorCode])
	require.NotContains(t, string(response.Payload), testUnavailableExecutorProfileID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestHTTPBulkMoveSelectedExecutorUnavailableUsesSafeConflict(t *testing.T) {
	h, repo := newUnavailableExecutorMoveHandler(t)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/tasks/bulk-move", fmt.Sprintf(`{
		"target_workflow_id": "wf-target",
		"target_step_id": "step-target",
		"task_ids": [%q]
	}`, repo.task.ID))

	h.httpBulkMoveTasks(c)

	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, moveConflictCodeExecutorUnavailable, body["code"])
	require.Equal(t, testUnavailableExecutorMessage, body["error"])
	require.NotContains(t, rec.Body.String(), testUnavailableExecutorProfileID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestHTTPBulkMoveWholeStepExecutorUnavailableUsesSafeConflict(t *testing.T) {
	h, repo := newUnavailableExecutorMoveHandler(t)
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/tasks/bulk-move", `{
		"source_workflow_id": "wf-source",
		"source_step_id": "step-source",
		"target_workflow_id": "wf-target",
		"target_step_id": "step-target"
	}`)

	h.httpBulkMoveTasks(c)

	require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, moveConflictCodeExecutorUnavailable, body["code"])
	require.Equal(t, testUnavailableExecutorMessage, body["error"])
	require.NotContains(t, rec.Body.String(), testUnavailableExecutorProfileID)
	require.Equal(t, "step-source", repo.task.WorkflowStepID)
}

func TestMoveConflictCodeRecognizesDestinationExecutorUnavailable(t *testing.T) {
	err := fmt.Errorf("executor profile %s: %w", testUnavailableExecutorProfileID, workflowmove.ErrDestinationExecutorUnavailable)
	require.True(t, errors.Is(err, workflowmove.ErrDestinationExecutorUnavailable))
	require.Equal(t, "task_move_executor_unavailable", moveConflictCode(err))
}
