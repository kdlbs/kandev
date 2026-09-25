package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type captureWorkflowMovePreviewer struct {
	request orchestrator.WorkflowMovePreviewRequest
	calls   int
	result  *orchestrator.WorkflowMovePreview
}

func (p *captureWorkflowMovePreviewer) PreviewWorkflowMove(
	_ context.Context,
	request orchestrator.WorkflowMovePreviewRequest,
) (*orchestrator.WorkflowMovePreview, error) {
	p.request = request
	p.calls++
	return p.result, nil
}

func newMovePreviewHandler(t *testing.T, repo *moveTaskConflictRepo, previewer WorkflowMovePreviewer, log *logger.Logger) *TaskHandlers {
	t.Helper()
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, movePreviewer: previewer, logger: log}
}

func TestHTTPMoveTaskPreviewNormalizesOptionsAndReturnsNoStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	task := &models.Task{
		ID:             "task-preview-handler",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-source",
		WorkflowStepID: "step-source",
	}
	repo := &moveTaskConflictRepo{
		task: task,
		workflows: map[string]*models.Workflow{
			"wf-target": {ID: "wf-target", WorkspaceID: task.WorkspaceID},
		},
	}
	previewer := &captureWorkflowMovePreviewer{
		result: &orchestrator.WorkflowMovePreview{
			TaskID:         task.ID,
			WorkflowStepID: "step-target",
			Outcome:        orchestrator.WorkflowMovePreviewOutcomeCreateNew,
		},
	}
	h := newMovePreviewHandler(t, repo, previewer, log)
	router := gin.New()
	router.POST("/api/v1/tasks/:id/move-preview", h.httpMoveTaskPreview)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.ID+"/move-preview", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"entry_options": {
			"instructions": "  inspect the implementation  ",
			"reset_context": true
		}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	assert.Equal(t, 1, previewer.calls)
	assert.Equal(t, task.ID, previewer.request.TaskID)
	assert.Equal(t, "wf-target", previewer.request.WorkflowID)
	assert.Equal(t, "step-target", previewer.request.WorkflowStepID)
	require.NotNil(t, previewer.request.EntryOptions)
	assert.Equal(t, "inspect the implementation", previewer.request.EntryOptions.Instructions)
	assert.True(t, previewer.request.EntryOptions.ResetContext)
	assert.Equal(t, "wf-source", repo.task.WorkflowID)
	assert.Equal(t, "step-source", repo.task.WorkflowStepID)

	var body orchestrator.WorkflowMovePreview
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, orchestrator.WorkflowMovePreviewOutcomeCreateNew, body.Outcome)
}

func TestHTTPMoveTaskPreviewRejectsOptionsForPositionOnlyMove(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	task := &models.Task{
		ID:             "task-position-preview",
		WorkspaceID:    "workspace-1",
		WorkflowID:     "wf-target",
		WorkflowStepID: "step-target",
	}
	repo := &moveTaskConflictRepo{
		task: task,
		workflows: map[string]*models.Workflow{
			"wf-target": {ID: "wf-target", WorkspaceID: task.WorkspaceID},
		},
	}
	previewer := &captureWorkflowMovePreviewer{}
	h := newMovePreviewHandler(t, repo, previewer, log)
	router := gin.New()
	router.POST("/api/v1/tasks/:id/move-preview", h.httpMoveTaskPreview)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tasks/"+task.ID+"/move-preview", strings.NewReader(`{
		"workflow_id": "wf-target",
		"workflow_step_id": "step-target",
		"entry_options": {"reset_context": true}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	assert.Zero(t, previewer.calls)
	assert.Contains(t, rec.Body.String(), "entry options")
}
