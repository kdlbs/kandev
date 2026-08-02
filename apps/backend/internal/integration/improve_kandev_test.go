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

func (c improveKandevTestCloner) EnsureWorkspaceCloned(
	_ context.Context, _, _, _, _, _ string,
) (string, error) {
	return c.path, nil
}

func TestImproveKandevBootstrapCreatesBothHiddenWorkflowsIdempotently(t *testing.T) {
	ts := NewOrchestratorTestServer(t)
	workspaceID := "123e4567-e89b-12d3-a456-426614174000"
	require.NoError(t, ts.TaskRepo.CreateWorkspace(context.Background(), &models.Workspace{
		ID:   workspaceID,
		Name: "Improve Kandev",
	}))

	repoPath := t.TempDir()
	require.NoError(t, exec.Command("git", "init", repoPath).Run())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "test-user"})
		c.Next()
	})
	handler := improvekandev.NewHandler(ts.TaskSvc, improveKandevTestCloner{path: repoPath}, "test", ts.Logger)
	improvekandev.RegisterRoutes(router, handler)

	bootstrap := func() improvekandev.BootstrapResponse {
		t.Helper()
		body, err := json.Marshal(improvekandev.BootstrapRequest{WorkspaceID: workspaceID})
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
	require.NotEmpty(t, first.WorkflowID)
	require.NotEmpty(t, first.IssueWorkflowID)
	require.NotEqual(t, first.WorkflowID, first.IssueWorkflowID)
	require.Equal(t, first.WorkflowID, second.WorkflowID)
	require.Equal(t, first.IssueWorkflowID, second.IssueWorkflowID)

	workflows, err := ts.TaskRepo.ListWorkflows(context.Background(), workspaceID, true)
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
