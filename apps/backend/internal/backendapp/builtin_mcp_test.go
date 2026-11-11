package backendapp

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestBuiltinMCPManagementRoutesAreWired(t *testing.T) {
	h := newBootStateTestHarness(t)
	log := logger.Default()
	settings, cleanup, err := store.Provide(h.db, h.db, log)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	eventBus := bus.NewMemoryEventBus(log)
	svc := orchestrator.NewService(orchestrator.DefaultServiceConfig(), eventBus, nil, &taskRepositoryAdapter{repo: h.taskRepo, svc: h.taskSvc}, h.taskRepo, nil, nil, nil, log)
	router := gin.New()
	configureMCPServices(routeParams{router: router, taskSvc: h.taskSvc, taskRepo: h.taskRepo, agentSettingsRepo: settings, orchestratorSvc: svc, log: log})
	workspaces, err := h.taskSvc.ListWorkspaces(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, workspaces)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/workspaces/"+workspaces[0].ID+"/mcp-built-in", nil))
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"configuration"`)
	require.Contains(t, response.Body.String(), `"can_edit":false`)
	require.NoError(t, h.taskRepo.CreateTask(context.Background(), &models.Task{ID: "builtin-task", WorkspaceID: workspaces[0].ID, Title: "Package task"}))
	workspaceID, surface, err := svc.BuiltinTaskScope(t.Context(), "builtin-task")
	require.NoError(t, err, "initial package consent must work before the first session exists")
	require.Equal(t, workspaces[0].ID, workspaceID)
	require.Equal(t, p.SurfaceKanbanTask, surface)
	require.NoError(t, h.taskRepo.CreateTaskSession(context.Background(), &models.TaskSession{ID: "builtin-session", TaskID: "builtin-task", State: models.TaskSessionStateCreated, IsPrimary: true}))
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/task-sessions/builtin-session/mcp-built-in", nil))
	require.Equal(t, 200, response.Code, response.Body.String())
	var view mcpconfig.BuiltinSessionView
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &view))
	require.NotNil(t, view.Desired, "normal wiring must install the runtime policy reader")
	require.Contains(t, view.Desired.Packages, p.PackageWalkthroughs)
	require.NotContains(t, view.Desired.Packages, p.PackageConfiguration)
	require.Nil(t, view.Bound)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/tasks/builtin-task/mcp-built-in", nil))
	require.Equal(t, 200, response.Code, response.Body.String())
	routes := router.Routes()
	paths := map[string]bool{}
	for _, route := range routes {
		paths[route.Method+" "+route.Path] = true
	}
	require.True(t, paths["GET /api/v1/task-sessions/:id/mcp-built-in"])
	require.True(t, paths["PUT /api/v1/tasks/:id/mcp-built-in/preferences"])
	require.True(t, paths["PUT /api/v1/workspaces/:id/mcp-built-in/policy"])
}
