package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type workflowMutationExecutorRepo struct {
	*workflowRepo
}

func (r *workflowMutationExecutorRepo) GetExecutor(context.Context, string) (*models.Executor, error) {
	return &models.Executor{ID: "private-executor", OwnerUserID: "alice"}, nil
}

func newExecutorMutationWorkflowHandlers(t *testing.T) (*WorkflowHandlers, *workflowRepo) {
	t.Helper()
	repo := workflowFixture()
	executorID := "private-executor"
	repo.workspaces["ws-1"].DefaultExecutorID = &executorID
	repo.workflows["wf-1"].CreatedByUserID = "alice"
	log := newTestLogger(t)
	svc := service.NewService(service.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo,
		Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, GitSnapshots: repo, RepoEntities: repo,
		Executors: &workflowMutationExecutorRepo{repo}, Environments: repo, TaskEnvironments: repo,
		Reviews: repo,
	}, nil, log, service.RepositoryDiscoveryConfig{})
	return NewWorkflowHandlers(svc, &stubStepLister{}, log), repo
}

func TestHTTPWorkflowExecutorMutationDenial(t *testing.T) {
	for _, user := range []string{"bob", "alice"} {
		t.Run(user, func(t *testing.T) {
			h, repo := newExecutorMutationWorkflowHandlers(t)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			h.registerHTTP(router)
			request := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/wf-1", strings.NewReader(`{"name":"Renamed"}`))
			request.Header.Set("Content-Type", "application/json")
			request = request.WithContext(authn.WithIdentity(t.Context(), authn.Identity{UserID: user, Role: authn.RoleMember}))
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			if user == "alice" {
				require.Equal(t, http.StatusOK, response.Code)
				require.Len(t, repo.updated, 1)
				return
			}
			require.Equal(t, http.StatusForbidden, response.Code)
			require.JSONEq(t, `{"error":"executor mutation denied"}`, response.Body.String())
			require.Empty(t, repo.updated)
			require.Equal(t, "Kanban", repo.workflows["wf-1"].Name)
		})
	}
}

func TestWSWorkflowExecutorMutationDenial(t *testing.T) {
	for _, user := range []string{"bob", "alice"} {
		t.Run(user, func(t *testing.T) {
			h, repo := newExecutorMutationWorkflowHandlers(t)
			ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: user, Role: authn.RoleMember})
			request, err := ws.NewRequest("edit", ws.ActionWorkflowUpdate, map[string]string{"id": "wf-1", "name": "Renamed"})
			require.NoError(t, err)

			response, err := h.wsUpdateWorkflow(ctx, request)

			require.NoError(t, err)
			if user == "alice" {
				require.Equal(t, ws.MessageTypeResponse, response.Type)
				require.Len(t, repo.updated, 1)
				return
			}
			payload := wsWorkflowError(t, response)
			require.Equal(t, ws.ErrorCodeForbidden, payload.Code)
			require.Equal(t, "executor mutation denied", payload.Message)
			require.Empty(t, payload.Details)
			require.Empty(t, repo.updated)
			require.Equal(t, "Kanban", repo.workflows["wf-1"].Name)
		})
	}
}
