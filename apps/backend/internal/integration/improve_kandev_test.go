package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/improvekandev"
	"github.com/kandev/kandev/internal/task/models"
)

type improveKandevTestCloner struct {
	path string
}

// newImproveKandevTestRouter registers the improve-kandev routes with a
// synthetic identity, mirroring the production auth middleware (bootstrap
// requires an authenticated identity).
func newImproveKandevTestRouter(handler *improvekandev.Handler) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "test-user"})
		c.Next()
	})
	improvekandev.RegisterRoutes(router, handler)
	return router
}

func (c improveKandevTestCloner) EnsureWorkspaceCloned(
	_ context.Context, _, _, _, _, _ string,
) (string, error) {
	return c.path, nil
}

func TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently(t *testing.T) {
	ts := NewOrchestratorTestServer(t)

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	bootstrap := func() improvekandev.BootstrapResponse {
		t.Helper()
		body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var response improvekandev.BootstrapResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
		return response
	}

	first := bootstrap()
	second := bootstrap()
	require.NotEmpty(t, first.WorkspaceID, "bootstrap must return the dedicated workspace id")
	require.Equal(t, first.WorkspaceID, second.WorkspaceID)
	require.NotEmpty(t, first.WorkflowID)
	require.NotEmpty(t, first.IssueWorkflowID)
	require.NotEqual(t, first.WorkflowID, first.IssueWorkflowID)
	require.Equal(t, first.WorkflowID, second.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, second.IssueWorkflowID)

	// The bootstrap auto-created a workspace named "Improve Kandev".
	workspace, err := ts.TaskRepo.GetWorkspace(context.Background(), first.WorkspaceID)
	require.NoError(t, err)
	require.Equal(t, "Improve Kandev", workspace.Name)

	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), first.WorkspaceID, true)
	require.NoError(t, err)
	byTemplate := make(map[string]*models.Workflow, len(workflows))
	for _, workflow := range workflows {
		if workflow.WorkflowTemplateID != nil {
			byTemplate[*workflow.WorkflowTemplateID] = workflow
		}
	}
	for _, templateID := range []string{"improve-kandev", "report-kandev-issue"} {
		workflow := byTemplate[templateID]
		require.NotNil(t, workflow, "workflow template %s", templateID)
		require.True(t, workflow.Hidden, "workflow template %s should stay hidden", templateID)
	}

	issueSteps, err := ts.WorkflowSvc.ListStepsByWorkflow(context.Background(), first.IssueWorkflowID)
	require.NoError(t, err)
	require.Len(t, issueSteps, 1)
	require.Equal(t, "Open issue", issueSteps[0].Name)
	require.True(t, issueSteps[0].IsStartStep)
}

func TestImproveKandevBootstrapReusesExistingImproveWorkspace(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	workspaceID := "123e4567-e89b-12d3-a456-426614174000"
	require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
		ID:   workspaceID,
		Name: "Improve Kandev",
	}))

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	// The request may still carry a workspace_id from an older client; it must
	// be ignored in favor of the dedicated Improve Kandev workspace.
	body, err := json.Marshal(improvekandev.BootstrapRequest{WorkspaceID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))

	require.Equal(t, workspaceID, response.WorkspaceID)
	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), workspaceID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows, "hidden workflows must live in the pre-existing Improve Kandev workspace")
}

func TestImproveKandevBootstrapFallsBackToRequestedWorkspaceWhenCreationDeclined(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	fallbackID := "123e4567-e89b-12d3-a456-426614174000"
	require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
		ID:   fallbackID,
		Name: "Active Workspace",
	}))

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	// No dedicated workspace exists and the caller declines creation
	// (create_workspace=false): bootstrap falls back to the requested
	// workspace and scopes the hidden workflows there.
	body, err := json.Marshal(improvekandev.BootstrapRequest{WorkspaceID: fallbackID})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
	require.Equal(t, fallbackID, response.WorkspaceID)

	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), fallbackID, true)
	require.NoError(t, err)
	require.NotEmpty(t, workflows, "hidden workflows must live in the fallback workspace")
}

type fakeGitHubWorkspaceCopier struct {
	calls []struct{ src, dst string }
}

func (f *fakeGitHubWorkspaceCopier) CopyWorkspaceConnectionToWorkspace(_ context.Context, src, dst string) error {
	f.calls = append(f.calls, struct{ src, dst string }{src, dst})
	return nil
}

func TestImproveKandevBootstrapCopiesGitHubConnectionOnWorkspaceCreation(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	copier := &fakeGitHubWorkspaceCopier{}

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	resolveDefault := func(context.Context) (string, error) {
		return "default-workspace-id", nil
	}
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, copier, resolveDefault, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	var response improvekandev.BootstrapResponse
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))

	require.Len(t, copier.calls, 1, "connection must be copied exactly once on workspace creation")
	require.Equal(t, "default-workspace-id", copier.calls[0].src)
	require.Equal(t, response.WorkspaceID, copier.calls[0].dst)

	// A second bootstrap (workspace now exists) must not copy again.
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	res2 := httptest.NewRecorder()
	router.ServeHTTP(res2, req2)
	require.Equal(t, http.StatusOK, res2.Code, res2.Body.String())
	require.Len(t, copier.calls, 1, "existing workspace must not re-copy the connection")
}

func TestImproveKandevBootstrapRequiresWorkspaceIDWhenCreationDeclined(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	body, err := json.Marshal(improvekandev.BootstrapRequest{})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	require.Equal(t, http.StatusInternalServerError, res.Code, res.Body.String())
}

func TestImproveKandevBootstrapConvergesOnSingleWorkspace(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	// Simulate the outcome of two concurrent first-time bootstraps: two rows
	// with the same name (workspace names are not unique). Bootstrap must
	// converge on a single, deterministic workspace id across calls.
	for _, id := range []string{
		"aaaaaaaa-0000-0000-0000-000000000001",
		"aaaaaaaa-0000-0000-0000-000000000002",
	} {
		require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
			ID:   id,
			Name: "Improve Kandev",
		}))
	}

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, nil, nil, "test", ts.Logger)
	router := newImproveKandevTestRouter(handler)

	bootstrap := func() improvekandev.BootstrapResponse {
		t.Helper()
		body, err := json.Marshal(improvekandev.BootstrapRequest{CreateWorkspace: true})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/system/improve-kandev/bootstrap", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code, res.Body.String())
		var response improvekandev.BootstrapResponse
		require.NoError(t, json.Unmarshal(res.Body.Bytes(), &response))
		return response
	}

	first := bootstrap()
	second := bootstrap()
	require.Equal(t, first.WorkspaceID, second.WorkspaceID, "both bootstraps must agree on one workspace")
}
