package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// httpTaskRepo extends the WS fixture with the paging and counting surfaces
// the HTTP list/count routes need, recording the arguments they receive.
type httpTaskRepo struct {
	wsTaskRepo

	listedWorkspaceID string
	listedPage        int
	listedPageSize    int
	listedSort        string
	listedArchived    bool
	listTotal         int

	workflowCount     int
	workflowCountErr  error
	stepCount         int
	stepCountErr      error
	countedWorkflowID string
	countedStepID     string
}

func (r *httpTaskRepo) ListTasksByWorkspace(
	_ context.Context,
	workspaceID, _, _, _ string,
	page, pageSize int,
	sort string,
	includeArchived, _, _, _ bool,
) ([]*models.Task, int, error) {
	r.listedWorkspaceID = workspaceID
	r.listedPage = page
	r.listedPageSize = pageSize
	r.listedSort = sort
	r.listedArchived = includeArchived
	return []*models.Task{{ID: "task-b", WorkspaceID: "ws-b", Title: "Mine"}}, r.listTotal, nil
}

func (r *httpTaskRepo) CountTasksByWorkflow(_ context.Context, workflowID string) (int, error) {
	r.countedWorkflowID = workflowID
	return r.workflowCount, r.workflowCountErr
}

func (r *httpTaskRepo) CountTasksByWorkflowStep(_ context.Context, stepID string) (int, error) {
	r.countedStepID = stepID
	return r.stepCount, r.stepCountErr
}

func newHTTPTaskHandlers(t *testing.T, repo *httpTaskRepo) *TaskHandlers {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return &TaskHandlers{service: svc, logger: log}
}

// taskRequestAs builds a request for a task route carrying userID's identity.
// An empty userID leaves the context identity-free (an internal caller).
func taskRequestAs(t *testing.T, userID, method, target, id string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, rec := newWorkflowRequest(t, method, target, "")
	if userID != "" {
		c.Request = c.Request.WithContext(authn.WithIdentity(
			c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
	}
	c.Params = gin.Params{{Key: "id", Value: id}}
	return c, rec
}

func TestHTTPGetTaskDeniesForeignTask(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})

	c, rec := taskRequestAs(t, "user-a", http.MethodGet, "/api/v1/tasks/task-b", "task-b")
	h.httpGetTask(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"task not found"}`, rec.Body.String())
	require.NotContains(t, rec.Body.String(), "Victim")

	ownerCtx, ownerRec := taskRequestAs(t, "user-b", http.MethodGet, "/api/v1/tasks/task-b", "task-b")
	h.httpGetTask(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code)
	require.Contains(t, ownerRec.Body.String(), "Victim", "the owner must still get their task")
}

func TestHTTPGetTaskReturnsNotFoundForUnknownTask(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/tasks/nope", "nope")

	h.httpGetTask(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"task not found"}`, rec.Body.String())
}

func TestHTTPListTasksFiltersForeignWorkspaceRows(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflows/wf-b/tasks", "wf-b")

	h.httpListTasks(c)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp dto.ListTasksResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, resp.Total, "the row from another workspace must be dropped")
	require.Equal(t, "task-b", resp.Tasks[0].ID)
}

func TestHTTPListTasksReturnsNotFoundForUnknownWorkflow(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflows/nope/tasks", "nope")

	h.httpListTasks(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"tasks not found"}`, rec.Body.String())
}

// TestHTTPListTasksByWorkspaceClampsPaging pins the query-parameter parsing:
// page and page_size fall back to their defaults for anything non-positive,
// out of range, or unparseable, and page_size is capped at 100.
func TestHTTPListTasksByWorkspaceClampsPaging(t *testing.T) {
	for name, tc := range map[string]struct {
		query        string
		wantPage     int
		wantPageSize int
	}{
		"defaults":           {query: "", wantPage: 1, wantPageSize: 50},
		"explicit":           {query: "?page=3&page_size=25", wantPage: 3, wantPageSize: 25},
		"page zero":          {query: "?page=0", wantPage: 1, wantPageSize: 50},
		"page negative":      {query: "?page=-2", wantPage: 1, wantPageSize: 50},
		"page garbage":       {query: "?page=abc", wantPage: 1, wantPageSize: 50},
		"page size over cap": {query: "?page_size=500", wantPage: 1, wantPageSize: 50},
		"page size at cap":   {query: "?page_size=100", wantPage: 1, wantPageSize: 100},
		"page size non int":  {query: "?page_size=many", wantPage: 1, wantPageSize: 50},
		"page size negative": {query: "?page_size=-1", wantPage: 1, wantPageSize: 50},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &httpTaskRepo{listTotal: 7}
			h := newHTTPTaskHandlers(t, repo)
			c, rec := taskRequestAs(t, "", http.MethodGet,
				"/api/v1/workspaces/ws-b/tasks"+tc.query, "ws-b")

			h.httpListTasksByWorkspace(c)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "ws-b", repo.listedWorkspaceID)
			require.Equal(t, tc.wantPage, repo.listedPage)
			require.Equal(t, tc.wantPageSize, repo.listedPageSize)

			var resp dto.ListTasksResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
			require.Equal(t, 7, resp.Total, "Total is the repository's grand total, not the page length")
			require.Len(t, resp.Tasks, 1)
		})
	}
}

func TestHTTPListTasksByWorkspacePassesThroughFilters(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "", http.MethodGet,
		"/api/v1/workspaces/ws-b/tasks?include_archived=true&sort=created_at", "ws-b")

	h.httpListTasksByWorkspace(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.listedArchived)
	require.NotEmpty(t, repo.listedSort, "sort is normalized rather than dropped")
}

func TestHTTPListTasksByWorkspaceDeniesForeignWorkspace(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "user-a", http.MethodGet, "/api/v1/workspaces/ws-b/tasks", "ws-b")

	h.httpListTasksByWorkspace(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, repo.listedWorkspaceID, "a denied list must not reach the repository")
}

func TestHTTPGetWorkflowTaskCount(t *testing.T) {
	repo := &httpTaskRepo{workflowCount: 4}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflows/wf-b/task-count", "wf-b")

	h.httpGetWorkflowTaskCount(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"task_count":4}`, rec.Body.String())
	require.Equal(t, "wf-b", repo.countedWorkflowID)
}

func TestHTTPGetWorkflowTaskCountSurfacesFailure(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{workflowCountErr: errors.New("database unavailable")})
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflows/wf-b/task-count", "wf-b")

	h.httpGetWorkflowTaskCount(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to count tasks"}`, rec.Body.String())
}

func TestHTTPGetStepTaskCount(t *testing.T) {
	repo := &httpTaskRepo{stepCount: 2}
	h := newHTTPTaskHandlers(t, repo)
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflow-steps/step-1/task-count", "step-1")

	h.httpGetStepTaskCount(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"task_count":2}`, rec.Body.String())
	require.Equal(t, "step-1", repo.countedStepID)
}

func TestHTTPGetStepTaskCountSurfacesFailure(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{stepCountErr: errors.New("database unavailable")})
	c, rec := taskRequestAs(t, "", http.MethodGet, "/api/v1/workflow-steps/step-1/task-count", "step-1")

	h.httpGetStepTaskCount(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to count tasks"}`, rec.Body.String())
}

// TestHTTPArchiveTaskDeniesForeignTask is the highest-value denial on this
// surface: archiving takes work away. The owner witness proves the 404 comes
// from scoping, and the write counter proves nothing was archived on the way
// out.
//
// The sibling delete route is deliberately not covered here: it derives its
// context from context.Background() rather than the request, which drops the
// caller identity before the service's check runs. That is reported separately
// rather than pinned here as if it were intended.
func TestHTTPArchiveTaskDeniesForeignTask(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)

	c, rec := taskRequestAs(t, "user-a", http.MethodDelete, "/api/v1/tasks/task-b", "task-b")
	h.httpArchiveTask(c)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"task not archived"}`, rec.Body.String())
	require.Empty(t, repo.archived, "a denied request must not archive the task")

	ownerCtx, ownerRec := taskRequestAs(t, "user-b", http.MethodDelete, "/api/v1/tasks/task-b", "task-b")
	h.httpArchiveTask(ownerCtx)
	require.Equal(t, http.StatusOK, ownerRec.Code,
		"the owner must succeed — otherwise the 404 above proves nothing (body: %s)", ownerRec.Body.String())
	require.Equal(t, []string{"task-b"}, repo.archived)
}

func TestHTTPDeleteTaskDeletesAndReportsMissingTasks(t *testing.T) {
	repo := &httpTaskRepo{}
	h := newHTTPTaskHandlers(t, repo)

	c, rec := taskRequestAs(t, "", http.MethodDelete, "/api/v1/tasks/task-b", "task-b")
	h.httpDeleteTask(c)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"success":true}`, rec.Body.String())
	require.Equal(t, []string{"task-b"}, repo.deleted)

	missingCtx, missingRec := taskRequestAs(t, "", http.MethodDelete, "/api/v1/tasks/nope", "nope")
	h.httpDeleteTask(missingCtx)
	require.Equal(t, http.StatusNotFound, missingRec.Code)
	require.JSONEq(t, `{"error":"task not deleted"}`, missingRec.Body.String())
	require.Len(t, repo.deleted, 1, "a missing task must not add a delete")
}

func TestCascadeQueryParam(t *testing.T) {
	for query, want := range map[string]bool{
		"":               false,
		"?cascade=true":  true,
		"?cascade=TRUE":  true,
		"?cascade=True":  true,
		"?cascade=false": false,
		"?cascade=1":     false,
		"?cascade=yes":   false,
		"?cascade":       false,
	} {
		c, _ := newWorkflowRequest(t, http.MethodDelete, "/api/v1/tasks/task-b"+query, "")
		require.Equalf(t, want, cascadeQueryParam(c), "cascade query %q", query)
	}
}

func TestHTTPBulkMoveTasksRejectsInvalidRequests(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})

	for name, tc := range map[string]struct {
		body     string
		wantBody string
	}{
		"malformed json": {
			body:     `{`,
			wantBody: `{"error":"invalid payload"}`,
		},
		"missing target workflow": {
			body:     `{"source_workflow_id":"wf-b","target_step_id":"step-1"}`,
			wantBody: `{"error":"target_workflow_id and target_step_id are required"}`,
		},
		"missing target step": {
			body:     `{"source_workflow_id":"wf-b","target_workflow_id":"wf-b"}`,
			wantBody: `{"error":"target_workflow_id and target_step_id are required"}`,
		},
		"missing source without explicit ids": {
			body:     `{"target_workflow_id":"wf-b","target_step_id":"step-1"}`,
			wantBody: `{"error":"source_workflow_id, target_workflow_id, and target_step_id are required"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/tasks/bulk-move", tc.body)

			h.httpBulkMoveTasks(c)

			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
}

// TestHTTPBulkMoveSelectedTasksWithEmptySelectionIsANoOp documents that an
// explicit but empty task list short-circuits instead of falling through to
// the whole-column move, which would move tasks nobody selected.
func TestHTTPBulkMoveSelectedTasksWithEmptySelectionIsANoOp(t *testing.T) {
	h := newHTTPTaskHandlers(t, &httpTaskRepo{})
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/tasks/bulk-move", "")

	h.httpBulkMoveSelectedTasks(c, httpBulkMoveTasksRequest{
		TaskIDs:          nil,
		TargetWorkflowID: "wf-b",
		TargetStepID:     "step-1",
	})

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"moved_count":0}`, rec.Body.String())
}
