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

type detachHandlerRepo struct {
	mockRepository
	task      *models.Task
	getErr    error
	updated   *models.Task
	updateErr error
}

func (r *detachHandlerRepo) GetTask(_ context.Context, _ string) (*models.Task, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	copy := *r.task
	return &copy, nil
}

func (r *detachHandlerRepo) UpdateTask(_ context.Context, task *models.Task) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updated = task
	return nil
}

func (r *detachHandlerRepo) DetachTask(_ context.Context, _ string) (bool, error) {
	if r.getErr != nil {
		return false, r.getErr
	}
	if r.updateErr != nil {
		return false, r.updateErr
	}
	if r.task.ParentID == "" {
		return false, nil
	}
	r.task.ParentID = ""
	if workspace, ok := r.task.Metadata["workspace"].(map[string]interface{}); ok && workspace["mode"] == "inherit_parent" {
		workspace["mode"] = "shared_group"
	}
	r.updated = r.task
	return true, nil
}

func (r *detachHandlerRepo) UpsertSessionFileReview(context.Context, *models.SessionFileReview) error {
	return nil
}

func (r *detachHandlerRepo) GetSessionFileReviews(context.Context, string) ([]*models.SessionFileReview, error) {
	return nil, nil
}

func (r *detachHandlerRepo) DeleteSessionFileReviews(context.Context, string) error { return nil }

func (r *detachHandlerRepo) ListTurnsBySession(context.Context, string) ([]*models.Turn, error) {
	return nil, nil
}

func (r *detachHandlerRepo) CountToolCallMessagesBySession(context.Context, []string) (map[string]int, error) {
	return nil, nil
}

func testDetachRouter(t *testing.T, repo *detachHandlerRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Tasks: repo, TaskRepos: repo, Sessions: repo, Messages: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	router := gin.New()
	NewTaskHandlers(svc, nil, repo, nil, log).registerHTTP(router)
	return router
}

func TestHTTPDetachTaskReturnsUpdatedTask(t *testing.T) {
	repo := &detachHandlerRepo{task: &models.Task{
		ID: "child", ParentID: "parent", Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "inherit_parent", "group_id": "group-1"},
		},
	}}
	router := testDetachRouter(t, repo)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/child/detach", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var response struct {
		ID       string                 `json:"id"`
		ParentID string                 `json:"parent_id"`
		Metadata map[string]interface{} `json:"metadata"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	require.Equal(t, "child", response.ID)
	require.Empty(t, response.ParentID)
	workspace := response.Metadata["workspace"].(map[string]interface{})
	require.Equal(t, "shared_group", workspace["mode"])
	require.NotNil(t, repo.updated)
}

func TestHTTPDetachTaskReturnsNotFound(t *testing.T) {
	repo := &detachHandlerRepo{getErr: repository.ErrTaskNotFound}
	router := testDetachRouter(t, repo)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/missing/detach", nil))

	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

func TestHTTPDetachTaskIsIdempotentForRoot(t *testing.T) {
	repo := &detachHandlerRepo{task: &models.Task{ID: "root"}}
	router := testDetachRouter(t, repo)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/root/detach", nil))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Nil(t, repo.updated, "root detach must not issue a task-row update")
}

func TestHTTPDetachTaskReturnsInternalErrorWhenPersistenceFails(t *testing.T) {
	repo := &detachHandlerRepo{
		task:      &models.Task{ID: "child", ParentID: "parent"},
		updateErr: errors.New("database write failed"),
	}
	router := testDetachRouter(t, repo)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/tasks/child/detach", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}
