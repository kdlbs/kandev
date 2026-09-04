package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

type reorderHandlerStepGetter struct {
	steps map[string]*wfmodels.WorkflowStep
}

func (f *reorderHandlerStepGetter) GetStep(_ context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	if step, ok := f.steps[stepID]; ok {
		return step, nil
	}
	return nil, taskrepo.ErrTaskNotFound
}

func (f *reorderHandlerStepGetter) GetNextStepByPosition(context.Context, string, int) (*wfmodels.WorkflowStep, error) {
	return nil, nil
}

// newReorderHandlerTest builds a real sqlite-backed service (the reorder
// repository method needs actual workflow_steps/tasks rows, not a mock) and
// seeds one step with the given tasks, each getting its own arrival
// position in creation order.
func newReorderHandlerTest(t *testing.T, stepID string, taskIDs ...string) (*TaskHandlers, *taskrepo.Repository) {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	repo, cleanup, err := repository.Provide(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cleanup() })

	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-reorder-http", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-reorder-http", WorkspaceID: "ws-reorder-http", Name: "Workflow"}))

	now := time.Now().UTC()
	_, err = repo.DB().Exec(`INSERT INTO workflow_steps
		(id, workflow_id, name, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		stepID, "wf-reorder-http", stepID, 0, now, now)
	require.NoError(t, err)

	for _, id := range taskIDs {
		require.NoError(t, repo.CreateTask(ctx, &models.Task{
			ID: id, WorkspaceID: "ws-reorder-http", WorkflowID: "wf-reorder-http", WorkflowStepID: stepID, Title: id,
		}))
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	require.NoError(t, err)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, bus.NewMemoryEventBus(log), log, service.RepositoryDiscoveryConfig{})
	svc.SetWorkflowStepGetter(&reorderHandlerStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		stepID: {ID: stepID, WorkflowID: "wf-reorder-http", Name: stepID, Position: 0},
	}})
	return &TaskHandlers{service: svc, logger: log}, repo
}

func reorderRequestContext(stepID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Params = gin.Params{{Key: "id", Value: stepID}}
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/workflow-steps/"+stepID+"/tasks/reorder", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func TestHTTPReorderStepTasksSuccess(t *testing.T) {
	h, _ := newReorderHandlerTest(t, "step-http-ok", "task-1", "task-2")
	c, rec := reorderRequestContext("step-http-ok", `{"band":"admitted","ordered_task_ids":["task-2","task-1"]}`)

	h.httpReorderStepTasks(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	if body["workflow_step_id"] != "step-http-ok" {
		t.Fatalf("workflow_step_id = %v, want step-http-ok", body["workflow_step_id"])
	}
	if body["revision"].(float64) != 1 {
		t.Fatalf("revision = %v, want 1", body["revision"])
	}
	tasks, ok := body["tasks"].([]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("tasks = %v, want 2 entries", body["tasks"])
	}
	first := tasks[0].(map[string]interface{})
	if first["id"] != "task-2" || first["position"].(float64) != 0 {
		t.Fatalf("tasks[0] = %v, want {id: task-2, position: 0}", first)
	}
}

func TestHTTPReorderStepTasksStepChangedConflict(t *testing.T) {
	h, _ := newReorderHandlerTest(t, "step-http-conflict", "task-1", "task-2")
	// Submission omits task-2: the band gained a member the caller doesn't
	// know about (REQ-TASKS-KANBAN-TASK-REORDERING-001.19/.26).
	c, rec := reorderRequestContext("step-http-conflict", `{"band":"admitted","ordered_task_ids":["task-1"]}`)

	h.httpReorderStepTasks(c)

	require.Equal(t, http.StatusConflict, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	if body["code"] != "step_changed" {
		t.Fatalf("code = %v, want step_changed", body["code"])
	}
	tasks, ok := body["tasks"].([]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("tasks = %v, want the authoritative 2-task order", body["tasks"])
	}
}

func TestHTTPReorderStepTasksInvalidRequest(t *testing.T) {
	h, _ := newReorderHandlerTest(t, "step-http-invalid", "task-1")
	c, rec := reorderRequestContext("step-http-invalid", `{"band":"admitted","ordered_task_ids":[]}`)

	h.httpReorderStepTasks(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	if body["code"] != "invalid_reorder" {
		t.Fatalf("code = %v, want invalid_reorder", body["code"])
	}
	if _, hasTasks := body["tasks"]; hasTasks {
		t.Fatalf("invalid_reorder body must carry no task list, got %v", body)
	}
}
