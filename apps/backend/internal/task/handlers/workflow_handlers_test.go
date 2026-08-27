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
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// workflowRepo is the shared fixture for the workflow handler tests. It records
// every mutation the handlers make so a test can assert that a rejected request
// wrote nothing, which is the whole point of the read-only guards.
type workflowRepo struct {
	mockRepository

	workspaces  map[string]*models.Workspace
	workflows   map[string]*models.Workflow
	byWorkspace map[string][]*models.Workflow

	listErr    error
	createErr  error
	updateErr  error
	deleteErr  error
	reorderErr error

	created            []*models.Workflow
	updated            []*models.Workflow
	deleted            []string
	reordered          []string
	reorderedWorkspace string

	listedWorkspaceID string
	listedHidden      bool
}

func (r *workflowRepo) GetWorkspace(_ context.Context, id string) (*models.Workspace, error) {
	if workspace, ok := r.workspaces[id]; ok {
		return workspace, nil
	}
	return nil, fmt.Errorf("workspace not found: %s", id)
}

func (r *workflowRepo) ListWorkspaces(context.Context) ([]*models.Workspace, error) {
	all := make([]*models.Workspace, 0, len(r.workspaces))
	for _, workspace := range r.workspaces {
		all = append(all, workspace)
	}
	return all, nil
}

func (r *workflowRepo) GetWorkflow(_ context.Context, id string) (*models.Workflow, error) {
	if workflow, ok := r.workflows[id]; ok {
		return workflow, nil
	}
	return nil, fmt.Errorf("workflow not found: %s", id)
}

func (r *workflowRepo) ListWorkflows(_ context.Context, workspaceID string, includeHidden bool) ([]*models.Workflow, error) {
	r.listedWorkspaceID = workspaceID
	r.listedHidden = includeHidden
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.byWorkspace[workspaceID], nil
}

func (r *workflowRepo) CreateWorkflow(_ context.Context, workflow *models.Workflow) error {
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, workflow)
	return nil
}

func (r *workflowRepo) UpdateWorkflow(_ context.Context, workflow *models.Workflow) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.updated = append(r.updated, workflow)
	return nil
}

func (r *workflowRepo) DeleteWorkflow(_ context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *workflowRepo) ReorderWorkflows(_ context.Context, workspaceID string, ids []string) error {
	if r.reorderErr != nil {
		return r.reorderErr
	}
	r.reorderedWorkspace = workspaceID
	r.reordered = ids
	return nil
}

func (r *workflowRepo) ListTasks(context.Context, string) ([]*models.Task, error) {
	return nil, nil
}

// stubStepLister satisfies WorkflowStepLister without a workflow repository.
type stubStepLister struct {
	steps []*workflowmodels.WorkflowStep
	err   error
	calls []string
}

func (s *stubStepLister) ListStepsByWorkflow(_ context.Context, workflowID string) ([]*workflowmodels.WorkflowStep, error) {
	s.calls = append(s.calls, workflowID)
	if s.err != nil {
		return nil, s.err
	}
	return s.steps, nil
}

func newWorkflowService(t *testing.T, repo *workflowRepo) (*service.Service, *logger.Logger) {
	t.Helper()
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: repo, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return svc, log
}

func newWorkflowHandlers(t *testing.T, repo *workflowRepo, steps WorkflowStepLister) *WorkflowHandlers {
	t.Helper()
	svc, log := newWorkflowService(t, repo)
	return NewWorkflowHandlers(svc, steps, log)
}

// workflowFixture builds a repo with one ordinary workspace holding two
// workflows: a normal one and the workspace's office workflow.
func workflowFixture() *workflowRepo {
	normal := &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Kanban"}
	office := &models.Workflow{ID: "wf-office", WorkspaceID: "ws-1", Name: "Office"}
	return &workflowRepo{
		workspaces: map[string]*models.Workspace{
			"ws-1":       {ID: "ws-1", Name: "Team", OfficeWorkflowID: "wf-office"},
			"ws-improve": {ID: "ws-improve", Name: models.WorkspaceNameImproveKandev},
		},
		workflows: map[string]*models.Workflow{
			"wf-1":       normal,
			"wf-office":  office,
			"wf-synced":  {ID: "wf-synced", WorkspaceID: "ws-1", Name: "Synced", Source: models.WorkflowSourceGitHub},
			"wf-improve": {ID: "wf-improve", WorkspaceID: "ws-improve", Name: "Improve"},
		},
		byWorkspace: map[string][]*models.Workflow{
			"ws-1": {normal, office},
		},
	}
}

func newWorkflowRequest(t *testing.T, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	c.Request = httptest.NewRequest(method, target, reader)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, rec
}

func decodeWorkflowList(t *testing.T, body []byte) dto.ListWorkflowsResponse {
	t.Helper()
	var resp dto.ListWorkflowsResponse
	require.NoError(t, json.Unmarshal(body, &resp))
	return resp
}

func workflowIDsOf(workflows []dto.WorkflowDTO) []string {
	ids := make([]string, 0, len(workflows))
	for _, w := range workflows {
		ids = append(ids, w.ID)
	}
	return ids
}

func TestHTTPListWorkflowsExcludesOfficeWorkflowByDefault(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows?workspace_id=ws-1", "")

	h.httpListWorkflows(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeWorkflowList(t, rec.Body.Bytes())
	require.Equal(t, []string{"wf-1"}, workflowIDsOf(resp.Workflows))
	require.Equal(t, 1, resp.Total, "Total must count the filtered list, not the raw one")
	require.Equal(t, "ws-1", repo.listedWorkspaceID)
	require.False(t, repo.listedHidden, "include_hidden defaults to false")
}

func TestHTTPListWorkflowsIncludesOfficeWhenExplicitlyRequested(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/workflows?workspace_id=ws-1&exclude_office=false&include_hidden=true", "")

	h.httpListWorkflows(c)

	require.Equal(t, http.StatusOK, rec.Code)
	resp := decodeWorkflowList(t, rec.Body.Bytes())
	require.Equal(t, []string{"wf-1", "wf-office"}, workflowIDsOf(resp.Workflows))
	require.Equal(t, 2, resp.Total)
	require.True(t, repo.listedHidden, "include_hidden=true must reach the repository")
}

func TestHTTPListWorkflowsReturnsInternalErrorWhenListFails(t *testing.T) {
	repo := workflowFixture()
	repo.listErr = errors.New("database unavailable")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows?workspace_id=ws-1", "")

	h.httpListWorkflows(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to list workflows"}`, rec.Body.String())
}

func TestHTTPListWorkflowsByWorkspaceReadsThePathParam(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workspaces/ws-1/workflows", "")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpListWorkflowsByWorkspace(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ws-1", repo.listedWorkspaceID)
	require.Equal(t, []string{"wf-1"}, workflowIDsOf(decodeWorkflowList(t, rec.Body.Bytes()).Workflows))
}

func TestParseIncludeHidden(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"yes", false},
	} {
		require.Equalf(t, tc.want, parseIncludeHidden(tc.in), "parseIncludeHidden(%q)", tc.in)
	}
}

func TestHTTPGetWorkflowReturnsWorkflowOrNotFound(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/wf-1", "")
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}
	h.httpGetWorkflow(c)
	require.Equal(t, http.StatusOK, rec.Code)
	var got dto.WorkflowDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "wf-1", got.ID)
	require.Equal(t, "Kanban", got.Name)

	missingCtx, missingRec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/nope", "")
	missingCtx.Params = gin.Params{{Key: "id", Value: "nope"}}
	h.httpGetWorkflow(missingCtx)
	require.Equal(t, http.StatusNotFound, missingRec.Code)
	require.JSONEq(t, `{"error":"workflow not found"}`, missingRec.Body.String())
}

func TestHTTPCreateWorkflowRejectsInvalidRequests(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	for name, tc := range map[string]struct {
		body     string
		wantBody string
	}{
		"malformed json": {body: `{`, wantBody: `{"error":"invalid request body"}`},
		"missing name": {
			body:     `{"workspace_id":"ws-1"}`,
			wantBody: `{"error":"workspace_id and name are required"}`,
		},
		"missing workspace": {
			body:     `{"name":"Flow"}`,
			wantBody: `{"error":"workspace_id and name are required"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/workflows", tc.body)
			h.httpCreateWorkflow(c)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
		})
	}
	require.Empty(t, repo.created, "rejected requests must not create a workflow")
}

func TestHTTPCreateWorkflowCreatesWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/workflows",
		`{"workspace_id":"ws-1","name":"Flow","description":"d","prompt":"p"}`)

	h.httpCreateWorkflow(c)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, repo.created, 1)
	require.Equal(t, "Flow", repo.created[0].Name)
	require.Equal(t, "ws-1", repo.created[0].WorkspaceID)
	require.Equal(t, "d", repo.created[0].Description)
	require.Equal(t, "p", repo.created[0].Prompt)

	var got dto.WorkflowDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, repo.created[0].ID, got.ID)
	require.Equal(t, "Flow", got.Name)
}

func TestHTTPCreateWorkflowRejectsImproveKandevWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/workflows",
		`{"workspace_id":"ws-improve","name":"Flow"}`)

	h.httpCreateWorkflow(c)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"`+workspaceReadOnlyMsg+`"}`, rec.Body.String())
	require.Empty(t, repo.created, "the read-only workspace must not gain a workflow")
}

func TestHTTPCreateWorkflowReturnsNotFoundForUnknownWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/workflows",
		`{"workspace_id":"ws-missing","name":"Flow"}`)

	h.httpCreateWorkflow(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"workspace not found"}`, rec.Body.String())
	require.Empty(t, repo.created)
}

func TestHTTPCreateWorkflowSurfacesRepositoryFailure(t *testing.T) {
	repo := workflowFixture()
	repo.createErr = errors.New("disk full")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPost, "/api/v1/workflows",
		`{"workspace_id":"ws-1","name":"Flow"}`)

	h.httpCreateWorkflow(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"failed to create workflow"}`, rec.Body.String())
}

func TestHTTPUpdateWorkflowUpdatesFields(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/workflows/wf-1",
		`{"name":"Renamed","agent_profile_id":"  ap-1  "}`)
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}

	h.httpUpdateWorkflow(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, repo.updated, 1)
	require.Equal(t, "Renamed", repo.updated[0].Name)
	require.Equal(t, "ap-1", repo.updated[0].AgentProfileID, "agent_profile_id is trimmed")

	var got dto.WorkflowDTO
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "Renamed", got.Name)
}

func TestHTTPUpdateWorkflowRejectsInvalidBody(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPatch, "/api/v1/workflows/wf-1", `{`)
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}

	h.httpUpdateWorkflow(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"invalid request body"}`, rec.Body.String())
	require.Empty(t, repo.updated)
}

// TestHTTPWorkflowMutationsRejectReadOnlyWorkflows covers both read-only
// reasons on both mutating routes: a GitHub-synced workflow and any workflow
// living in the Improve Kandev workspace. The recorded-mutation assertions are
// the point — a guard that answered 409 after writing would still be a bug.
func TestHTTPWorkflowMutationsRejectReadOnlyWorkflows(t *testing.T) {
	for name, tc := range map[string]struct {
		workflowID string
		wantMsg    string
	}{
		"github synced":     {workflowID: "wf-synced", wantMsg: workflowReadOnlyMsg},
		"improve workspace": {workflowID: "wf-improve", wantMsg: workspaceReadOnlyMsg},
	} {
		t.Run(name, func(t *testing.T) {
			repo := workflowFixture()
			h := newWorkflowHandlers(t, repo, &stubStepLister{})

			updateCtx, updateRec := newWorkflowRequest(t, http.MethodPatch,
				"/api/v1/workflows/"+tc.workflowID, `{"name":"Renamed"}`)
			updateCtx.Params = gin.Params{{Key: "id", Value: tc.workflowID}}
			h.httpUpdateWorkflow(updateCtx)
			require.Equal(t, http.StatusConflict, updateRec.Code)
			require.JSONEq(t, `{"error":"`+tc.wantMsg+`"}`, updateRec.Body.String())
			require.Empty(t, repo.updated, "read-only workflow must not be updated")

			deleteCtx, deleteRec := newWorkflowRequest(t, http.MethodDelete,
				"/api/v1/workflows/"+tc.workflowID, "")
			deleteCtx.Params = gin.Params{{Key: "id", Value: tc.workflowID}}
			h.httpDeleteWorkflow(deleteCtx)
			require.Equal(t, http.StatusConflict, deleteRec.Code)
			require.JSONEq(t, `{"error":"`+tc.wantMsg+`"}`, deleteRec.Body.String())
			require.Empty(t, repo.deleted, "read-only workflow must not be deleted")
		})
	}
}

func TestHTTPDeleteWorkflowDeletesEditableWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/workflows/wf-1", "")
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}

	h.httpDeleteWorkflow(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"success":true}`, rec.Body.String())
	require.Equal(t, []string{"wf-1"}, repo.deleted)
}

func TestHTTPDeleteWorkflowReturnsNotFoundForUnknownWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodDelete, "/api/v1/workflows/nope", "")
	c.Params = gin.Params{{Key: "id", Value: "nope"}}

	h.httpDeleteWorkflow(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"workflow not found"}`, rec.Body.String())
	require.Empty(t, repo.deleted)
}

func TestHTTPReorderWorkflowsRejectsInvalidRequests(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})

	badCtx, badRec := newWorkflowRequest(t, http.MethodPut,
		"/api/v1/workspaces/ws-1/workflows/reorder", `{`)
	badCtx.Params = gin.Params{{Key: "id", Value: "ws-1"}}
	h.httpReorderWorkflows(badCtx)
	require.Equal(t, http.StatusBadRequest, badRec.Code)
	require.Contains(t, badRec.Body.String(), "Invalid request:")

	emptyCtx, emptyRec := newWorkflowRequest(t, http.MethodPut,
		"/api/v1/workspaces/ws-1/workflows/reorder", `{"workflow_ids":[]}`)
	emptyCtx.Params = gin.Params{{Key: "id", Value: "ws-1"}}
	h.httpReorderWorkflows(emptyCtx)
	require.Equal(t, http.StatusBadRequest, emptyRec.Code)
	require.JSONEq(t, `{"error":"workflow_ids is required"}`, emptyRec.Body.String())

	require.Nil(t, repo.reordered, "rejected requests must not reorder")
}

func TestHTTPReorderWorkflowsRejectsImproveKandevWorkspace(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPut,
		"/api/v1/workspaces/ws-improve/workflows/reorder", `{"workflow_ids":["wf-improve"]}`)
	c.Params = gin.Params{{Key: "id", Value: "ws-improve"}}

	h.httpReorderWorkflows(c)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.JSONEq(t, `{"error":"`+workspaceReadOnlyMsg+`"}`, rec.Body.String())
	require.Nil(t, repo.reordered)
}

func TestHTTPReorderWorkflowsPersistsOrder(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPut,
		"/api/v1/workspaces/ws-1/workflows/reorder", `{"workflow_ids":["wf-office","wf-1"]}`)
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpReorderWorkflows(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"success":true}`, rec.Body.String())
	require.Equal(t, "ws-1", repo.reorderedWorkspace)
	require.Equal(t, []string{"wf-office", "wf-1"}, repo.reordered)
}

func TestHTTPReorderWorkflowsSurfacesRepositoryFailure(t *testing.T) {
	repo := workflowFixture()
	repo.reorderErr = errors.New("constraint violation")
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodPut,
		"/api/v1/workspaces/ws-1/workflows/reorder", `{"workflow_ids":["wf-1"]}`)
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpReorderWorkflows(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"Failed to reorder workflows"}`, rec.Body.String())
}

func decodeWorkflowSnapshot(t *testing.T, body []byte) dto.WorkflowSnapshotDTO {
	t.Helper()
	var snapshot dto.WorkflowSnapshotDTO
	require.NoError(t, json.Unmarshal(body, &snapshot))
	return snapshot
}

func TestHTTPGetWorkflowSnapshotReturnsWorkflowStepsAndTasks(t *testing.T) {
	repo := workflowFixture()
	steps := &stubStepLister{steps: []*workflowmodels.WorkflowStep{
		{ID: "step-1", WorkflowID: "wf-1", Name: "Todo"},
		{ID: "step-2", WorkflowID: "wf-1", Name: "Done"},
	}}
	h := newWorkflowHandlers(t, repo, steps)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/wf-1/snapshot", "")
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}

	h.httpGetWorkflowSnapshot(c)

	require.Equal(t, http.StatusOK, rec.Code)
	snapshot := decodeWorkflowSnapshot(t, rec.Body.Bytes())
	require.Equal(t, "wf-1", snapshot.Workflow.ID)
	require.Len(t, snapshot.Steps, 2)
	require.Equal(t, "step-1", snapshot.Steps[0].ID)
	require.Equal(t, []string{"wf-1"}, steps.calls)
	require.Empty(t, snapshot.Tasks)
}

func TestHTTPGetWorkflowSnapshotReturnsNotFoundForUnknownWorkflow(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/nope/snapshot", "")
	c.Params = gin.Params{{Key: "id", Value: "nope"}}

	h.httpGetWorkflowSnapshot(c)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.JSONEq(t, `{"error":"workflow not found"}`, rec.Body.String())
}

func TestHTTPGetWorkflowSnapshotFailsWithoutAStepLister(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, nil)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/wf-1/snapshot", "")
	c.Params = gin.Params{{Key: "id", Value: "wf-1"}}

	h.httpGetWorkflowSnapshot(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"request failed"}`, rec.Body.String())
}

func TestHTTPGetWorkspaceSnapshotFallsBackToFirstWorkflow(t *testing.T) {
	repo := workflowFixture()
	steps := &stubStepLister{steps: []*workflowmodels.WorkflowStep{{ID: "step-1", WorkflowID: "wf-1"}}}
	h := newWorkflowHandlers(t, repo, steps)
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workspaces/ws-1/snapshot", "")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpGetWorkspaceSnapshot(c)

	require.Equal(t, http.StatusOK, rec.Code)
	snapshot := decodeWorkflowSnapshot(t, rec.Body.Bytes())
	require.Equal(t, "wf-1", snapshot.Workflow.ID, "no workflow_id means the workspace's first workflow")
	require.Equal(t, []string{"wf-1"}, steps.calls)
}

func TestHTTPGetWorkspaceSnapshotUsesExplicitWorkflowID(t *testing.T) {
	repo := workflowFixture()
	steps := &stubStepLister{}
	h := newWorkflowHandlers(t, repo, steps)
	c, rec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/workspaces/ws-1/snapshot?workflow_id=wf-office", "")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpGetWorkspaceSnapshot(c)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "wf-office", decodeWorkflowSnapshot(t, rec.Body.Bytes()).Workflow.ID)
	require.Equal(t, []string{"wf-office"}, steps.calls)
}

func TestHTTPGetWorkspaceSnapshotRejectsMissingWorkspaceID(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workspaces//snapshot", "")

	h.httpGetWorkspaceSnapshot(c)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":"workspace id is required"}`, rec.Body.String())
}

// TestHTTPGetWorkspaceSnapshotWhenWorkspaceHasNoWorkflows pins the status this
// path actually returns. The handler builds its own error and passes the
// "workflow not found" fallback to handleNotFound, clearly intending a 404 —
// but handleNotFound classifies by message, and "no workflows found for
// workspace: X" contains neither "not found" nor a validation keyword, so the
// request lands on the generic 500. Asserted as-is: this is existing behavior,
// and pinning it means a future fix has to come with a deliberate test change.
func TestHTTPGetWorkspaceSnapshotWhenWorkspaceHasNoWorkflows(t *testing.T) {
	repo := workflowFixture()
	repo.workspaces["ws-empty"] = &models.Workspace{ID: "ws-empty", Name: "Empty"}
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	c, rec := newWorkflowRequest(t, http.MethodGet, "/api/v1/workspaces/ws-empty/snapshot", "")
	c.Params = gin.Params{{Key: "id", Value: "ws-empty"}}

	h.httpGetWorkspaceSnapshot(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.JSONEq(t, `{"error":"request failed"}`, rec.Body.String())
}

// TestHTTPGetWorkspaceSnapshotRejectsWorkflowFromAnotherWorkspace pins the
// cross-workspace check: asking workspace A for workspace B's workflow must not
// serve B's board. The refusal currently arrives as a 500 for the same
// message-classification reason as the test above; what matters for safety is
// that no foreign data is returned and B's steps are never read.
func TestHTTPGetWorkspaceSnapshotRejectsWorkflowFromAnotherWorkspace(t *testing.T) {
	repo := workflowFixture()
	steps := &stubStepLister{}
	h := newWorkflowHandlers(t, repo, steps)
	c, rec := newWorkflowRequest(t, http.MethodGet,
		"/api/v1/workspaces/ws-1/snapshot?workflow_id=wf-improve", "")
	c.Params = gin.Params{{Key: "id", Value: "ws-1"}}

	h.httpGetWorkspaceSnapshot(c)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "wf-improve", "the foreign workflow must not be serialized")
	require.Empty(t, steps.calls, "the foreign workflow's steps must never be read")
}

func TestApplyTaskLimitTruncatesOnlyForPositiveSmallerLimits(t *testing.T) {
	tasks := []*models.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}}
	for name, tc := range map[string]struct {
		query string
		want  int
	}{
		"no limit":       {query: "", want: 3},
		"limit two":      {query: "?task_limit=2", want: 2},
		"limit exceeds":  {query: "?task_limit=99", want: 3},
		"limit zero":     {query: "?task_limit=0", want: 3},
		"limit negative": {query: "?task_limit=-1", want: 3},
		"limit garbage":  {query: "?task_limit=abc", want: 3},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newWorkflowRequest(t, http.MethodGet, "/api/v1/workflows/wf-1/snapshot"+tc.query, "")
			require.Len(t, applyTaskLimit(c, tasks), tc.want)
		})
	}
}

func TestNewWorkflowHandlersStoresForegroundActivityProvider(t *testing.T) {
	repo := workflowFixture()
	h := newWorkflowHandlers(t, repo, &stubStepLister{})
	require.Nil(t, h.foregroundActivity)

	h.SetForegroundActivityProvider(fakeForegroundActivityProvider{})
	require.NotNil(t, h.foregroundActivity)
}

// wsWorkflowError decodes an error frame so tests can assert on the code and
// message separately instead of substring-matching the whole payload.
func wsWorkflowError(t *testing.T, msg *ws.Message) ws.ErrorPayload {
	t.Helper()
	require.Equal(t, ws.MessageTypeError, msg.Type, "expected an error frame, got %s", msg.Payload)
	var payload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &payload))
	return payload
}
