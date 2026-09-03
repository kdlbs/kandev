package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type dependencyHandlerRepo struct {
	mockRepository
	tasks    map[string]*models.Task
	blockers []*orchmodels.TaskBlocker
}

func (r *dependencyHandlerRepo) GetTask(_ context.Context, id string) (*models.Task, error) {
	return r.tasks[id], nil
}

func (r *dependencyHandlerRepo) GetTasksByIDs(_ context.Context, ids []string) ([]*models.Task, error) {
	result := make([]*models.Task, 0, len(ids))
	for _, id := range ids {
		if task := r.tasks[id]; task != nil {
			result = append(result, task)
		}
	}
	return result, nil
}

func (r *dependencyHandlerRepo) CreateTaskBlocker(_ context.Context, blocker *orchmodels.TaskBlocker) error {
	r.blockers = append(r.blockers, blocker)
	return nil
}

func (r *dependencyHandlerRepo) ListTaskBlockers(_ context.Context, taskID string) ([]*orchmodels.TaskBlocker, error) {
	result := make([]*orchmodels.TaskBlocker, 0)
	for _, blocker := range r.blockers {
		if blocker.TaskID == taskID {
			result = append(result, blocker)
		}
	}
	return result, nil
}

func (r *dependencyHandlerRepo) DeleteTaskBlocker(_ context.Context, taskID, blockerTaskID string) error {
	for index, blocker := range r.blockers {
		if blocker.TaskID == taskID && blocker.BlockerTaskID == blockerTaskID {
			r.blockers = append(r.blockers[:index], r.blockers[index+1:]...)
			break
		}
	}
	return nil
}

func (r *dependencyHandlerRepo) ListTasksBlockedBy(_ context.Context, blockerTaskID string) ([]string, error) {
	ids := make([]string, 0)
	for _, blocker := range r.blockers {
		if blocker.BlockerTaskID == blockerTaskID {
			ids = append(ids, blocker.TaskID)
		}
	}
	return ids, nil
}

func (r *dependencyHandlerRepo) ListBlockersForTasks(_ context.Context, taskIDs []string) (map[string][]string, error) {
	wanted := make(map[string]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		wanted[id] = struct{}{}
	}
	result := make(map[string][]string)
	for _, blocker := range r.blockers {
		if _, ok := wanted[blocker.TaskID]; ok {
			result[blocker.TaskID] = append(result[blocker.TaskID], blocker.BlockerTaskID)
		}
	}
	return result, nil
}

func (r *dependencyHandlerRepo) ListDependentsForTasks(_ context.Context, taskIDs []string) (map[string][]string, error) {
	wanted := make(map[string]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		wanted[id] = struct{}{}
	}
	result := make(map[string][]string)
	for _, blocker := range r.blockers {
		if _, ok := wanted[blocker.BlockerTaskID]; ok {
			result[blocker.BlockerTaskID] = append(result[blocker.BlockerTaskID], blocker.TaskID)
		}
	}
	return result, nil
}

func (r *dependencyHandlerRepo) ReplaceTaskBlockers(_ context.Context, taskID string, blockerTaskIDs []string) error {
	next := make([]*orchmodels.TaskBlocker, 0, len(r.blockers)+len(blockerTaskIDs))
	for _, blocker := range r.blockers {
		if blocker.TaskID != taskID {
			next = append(next, blocker)
		}
	}
	for _, blockerTaskID := range blockerTaskIDs {
		next = append(next, &orchmodels.TaskBlocker{TaskID: taskID, BlockerTaskID: blockerTaskID})
	}
	r.blockers = next
	return nil
}

func newDependencyHandlers(t *testing.T, repo *dependencyHandlerRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	svc.SetBlockerRepository(repo)
	return &TaskHandlers{service: svc, logger: log}
}

func dependencyRequest(t *testing.T, h *TaskHandlers, taskID, body string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPut,
		"/api/v1/tasks/"+taskID+"/dependencies",
		strings.NewReader(body),
	)
	context.Params = gin.Params{{Key: "id", Value: taskID}}
	h.httpReplaceTaskDependencies(context)
	return recorder
}

func dependencyTask(id, title string) *models.Task {
	return &models.Task{
		ID: id, WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Title: title, State: v1.TaskStateTODO,
	}
}

func TestHTTPReplaceTaskDependenciesReturnsProjection(t *testing.T) {
	repo := &dependencyHandlerRepo{tasks: map[string]*models.Task{
		"task-1": dependencyTask("task-1", "Dependent"),
		"task-2": dependencyTask("task-2", "Predecessor"),
	}}
	h := newDependencyHandlers(t, repo)

	recorder := dependencyRequest(t, h, "task-1", `{"depends_on_task_ids":["task-2"]}`)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "task-1", response["task_id"])
	require.Len(t, response["depends_on"], 1)
	require.Len(t, repo.blockers, 1)
	require.Equal(t, "task-2", repo.blockers[0].BlockerTaskID)
}

func TestHTTPReplaceTaskDependenciesRejectsCycleAndPreservesSet(t *testing.T) {
	repo := &dependencyHandlerRepo{tasks: map[string]*models.Task{
		"task-a": dependencyTask("task-a", "A"),
		"task-b": dependencyTask("task-b", "B"),
		"task-c": dependencyTask("task-c", "C"),
	}, blockers: []*orchmodels.TaskBlocker{
		{TaskID: "task-a", BlockerTaskID: "task-b"},
		{TaskID: "task-b", BlockerTaskID: "task-c"},
	}}
	h := newDependencyHandlers(t, repo)

	recorder := dependencyRequest(t, h, "task-c", `{"depends_on_task_ids":["task-a"]}`)

	require.Equal(t, http.StatusConflict, recorder.Code)
	require.Contains(t, recorder.Body.String(), "cycle")
	require.Len(t, repo.blockers, 2)
}

func TestHTTPReplaceTaskDependenciesRequiresCompleteList(t *testing.T) {
	repo := &dependencyHandlerRepo{tasks: map[string]*models.Task{
		"task-1": dependencyTask("task-1", "Dependent"),
	}}
	h := newDependencyHandlers(t, repo)

	recorder := dependencyRequest(t, h, "task-1", `{}`)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "depends_on_task_ids")
}
