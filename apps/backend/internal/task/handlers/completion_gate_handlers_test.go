package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

type completionGateResponseRepository struct {
	*moveTaskConflictRepo
	snapshot *models.TaskCompletionGateSnapshot
}

func (r *completionGateResponseRepository) SetTaskCompletionCriteria(
	context.Context,
	models.TaskCompletionCriteriaChange,
) (*models.TaskCompletionGateSnapshot, error) {
	return nil, errors.New("unexpected criteria update")
}

func (r *completionGateResponseRepository) VerifyTaskCompletionCriterion(
	context.Context,
	models.TaskCompletionEvidenceChange,
) (*models.TaskCompletionGateSnapshot, error) {
	return nil, errors.New("unexpected evidence update")
}

func (r *completionGateResponseRepository) GetTaskCompletionGate(
	context.Context,
	string,
) (*models.TaskCompletionGateSnapshot, error) {
	return r.snapshot, nil
}

func (r *completionGateResponseRepository) ListTaskCompletionGateHistory(
	context.Context,
	string,
) ([]*models.TaskCompletionGateHistory, error) {
	return nil, nil
}

func TestHTTPCompletionGateBlockedErrorIncludesCurrentSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	snapshot := &models.TaskCompletionGateSnapshot{
		TaskID:   "task-1",
		Revision: 3,
		Criteria: []models.TaskCompletionCriterion{},
		Blockers: []models.TaskCompletionBlocker{{CriterionID: "review", Reason: "evidence_stale"}},
		Blocked:  true,
	}
	repo := &completionGateResponseRepository{
		moveTaskConflictRepo: &moveTaskConflictRepo{task: &models.Task{ID: "task-1"}},
		snapshot:             snapshot,
	}
	log := newTestLogger(t)
	taskService := service.NewService(service.Repos{Tasks: repo}, nil, log, service.RepositoryDiscoveryConfig{})
	handlers := &TaskHandlers{service: taskService, logger: log}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/tasks/task-1/move", nil)

	handlers.handleCompletionGateError(ctx, repository.ErrTaskCompletionGateBlocked)

	require.Equal(t, http.StatusConflict, response.Code)
	var body struct {
		Code string                            `json:"code"`
		Gate models.TaskCompletionGateSnapshot `json:"gate"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, "task_completion_gate_blocked", body.Code)
	require.Equal(t, *snapshot, body.Gate)
}
