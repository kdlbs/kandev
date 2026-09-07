package handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// registeredRoutes returns the router's method+path set.
func registeredRoutes(router *gin.Engine) map[string]bool {
	routes := make(map[string]bool)
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	return routes
}

func requireRoutes(t *testing.T, router *gin.Engine, want ...string) {
	t.Helper()
	routes := registeredRoutes(router)
	for _, route := range want {
		require.Truef(t, routes[route], "route %q is not registered", route)
	}
}

func requireActions(t *testing.T, dispatcher *ws.Dispatcher, want ...string) {
	t.Helper()
	for _, action := range want {
		require.Truef(t, dispatcher.HasHandler(action), "WS action %q has no handler", action)
	}
}

// TestRegisterWorkflowRoutesWiresHTTPAndWS pins the full public surface of the
// workflow handlers. A route silently dropped during a refactor is invisible to
// every other test in this file, which calls the methods directly.
func TestRegisterWorkflowRoutesWiresHTTPAndWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := workflowFixture()
	svc, log := newWorkflowService(t, repo)

	handlers := RegisterWorkflowRoutes(router, dispatcher, svc, &stubStepLister{}, log)

	require.NotNil(t, handlers)
	requireRoutes(t, router,
		"GET /api/v1/workflows",
		"GET /api/v1/workspaces/:id/workflows",
		"GET /api/v1/workspaces/:id/snapshot",
		"GET /api/v1/workflows/:id",
		"GET /api/v1/workflows/:id/snapshot",
		"POST /api/v1/workflows",
		"PATCH /api/v1/workflows/:id",
		"DELETE /api/v1/workflows/:id",
		"PUT /api/v1/workspaces/:id/workflows/reorder",
	)
	requireActions(t, dispatcher,
		ws.ActionWorkflowList, ws.ActionWorkflowCreate, ws.ActionWorkflowGet,
		ws.ActionWorkflowUpdate, ws.ActionWorkflowDelete, ws.ActionWorkflowReorder,
	)
}

func TestRegisterExecutorRoutesWiresHTTPAndWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := executorFixture()

	RegisterExecutorRoutes(router, dispatcher, newExecutorService(t, repo), newTestLogger(t))

	requireRoutes(t, router,
		"GET /api/v1/executors",
		"POST /api/v1/executors",
		"GET /api/v1/executors/:id",
		"PATCH /api/v1/executors/:id",
		"DELETE /api/v1/executors/:id",
	)
	requireActions(t, dispatcher,
		ws.ActionExecutorList, ws.ActionExecutorCreate, ws.ActionExecutorGet,
		ws.ActionExecutorUpdate, ws.ActionExecutorDelete,
	)
}

// TestRegisterExecutorProfileRoutesWiresBothDeleteRoutes covers the two DELETE
// registrations in particular: profile IDs are globally unique, so callers
// holding only the profile ID use the top-level route.
func TestRegisterExecutorProfileRoutesWiresBothDeleteRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := executorFixture()

	RegisterExecutorProfileRoutes(router, dispatcher, newExecutorService(t, repo), nil, newTestLogger(t))

	requireRoutes(t, router,
		"GET /api/v1/executor-profiles",
		"GET /api/v1/executor-profiles/default-script",
		"GET /api/v1/remote-credentials",
		"GET /api/v1/agent-config-bundles",
		"GET /api/v1/git/identity",
		"GET /api/v1/script-placeholders",
		"GET /api/v1/executors/:id/profiles",
		"POST /api/v1/executors/:id/profiles",
		"GET /api/v1/executors/:id/profiles/:profileId",
		"PATCH /api/v1/executors/:id/profiles/:profileId",
		"DELETE /api/v1/executors/:id/profiles/:profileId",
		"DELETE /api/v1/executor-profiles/:profileId",
	)
	requireActions(t, dispatcher,
		ws.ActionExecutorProfileList, ws.ActionExecutorProfileListAll,
		ws.ActionExecutorProfileCreate, ws.ActionExecutorProfileGet,
		ws.ActionExecutorProfileUpdate, ws.ActionExecutorProfileDelete,
	)
}

func TestRegisterWorkspaceRoutesWiresHTTPAndWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := workflowFixture()
	svc, log := newWorkflowService(t, repo)

	RegisterWorkspaceRoutes(router, dispatcher, svc, log)

	requireRoutes(t, router,
		"GET /api/v1/workspaces",
		"POST /api/v1/workspaces",
		"GET /api/v1/workspaces/:id",
		"PATCH /api/v1/workspaces/:id",
		"DELETE /api/v1/workspaces/:id",
	)
	requireActions(t, dispatcher,
		ws.ActionWorkspaceList, ws.ActionWorkspaceCreate, ws.ActionWorkspaceGet,
		ws.ActionWorkspaceUpdate, ws.ActionWorkspaceDelete,
	)
}

func TestRegisterEnvironmentRoutesWiresHTTPAndWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := workflowFixture()
	svc, log := newWorkflowService(t, repo)

	RegisterEnvironmentRoutes(router, dispatcher, svc, log)

	requireRoutes(t, router,
		"GET /api/v1/environments",
		"POST /api/v1/environments",
		"GET /api/v1/environments/:id",
		"PATCH /api/v1/environments/:id",
		"DELETE /api/v1/environments/:id",
	)
	requireActions(t, dispatcher,
		ws.ActionEnvironmentList, ws.ActionEnvironmentCreate, ws.ActionEnvironmentGet,
		ws.ActionEnvironmentUpdate, ws.ActionEnvironmentDelete,
	)
}

func TestRegisterRepositoryRoutesWiresHTTPAndWS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	dispatcher := ws.NewDispatcher()
	repo := workflowFixture()
	svc, log := newWorkflowService(t, repo)

	RegisterRepositoryRoutes(router, dispatcher, svc, log)

	requireRoutes(t, router,
		"GET /api/v1/workspaces/:id/repositories",
		"POST /api/v1/workspaces/:id/repositories",
		"POST /api/v1/workspaces/:id/repositories/initialize-local",
		"GET /api/v1/workspaces/:id/repositories/discover",
		"GET /api/v1/workspaces/:id/repositories/discovery",
		"POST /api/v1/workspaces/:id/repositories/discovery/refresh",
		"GET /api/v1/repositories/discovery/roots",
		"POST /api/v1/repositories/discovery/roots",
		"POST /api/v1/repositories/discovery/roots/reconnect",
		"DELETE /api/v1/repositories/discovery/roots",
		"GET /api/v1/workspaces/:id/branches",
		"GET /api/v1/workspaces/:id/repositories/local-status",
		"GET /api/v1/workspaces/:id/repositories/validate",
		"GET /api/v1/fs/list-dir",
		"POST /api/v1/fs/create-dir",
		"GET /api/v1/repositories/:id",
		"GET /api/v1/repositories/:id/branches",
		"GET /api/v1/repositories/:id/active-session-count",
		"PATCH /api/v1/repositories/:id",
		"DELETE /api/v1/repositories/:id",
		"GET /api/v1/repositories/:id/scripts",
		"POST /api/v1/repositories/:id/scripts",
		"GET /api/v1/scripts/:id",
		"PUT /api/v1/scripts/:id",
		"DELETE /api/v1/scripts/:id",
	)
	requireActions(t, dispatcher,
		ws.ActionRepositoryList, ws.ActionRepositoryCreate, ws.ActionRepositoryGet,
		ws.ActionRepositoryUpdate, ws.ActionRepositoryDelete,
		ws.ActionRepositoryScriptList, ws.ActionRepositoryScriptCreate,
		ws.ActionRepositoryScriptGet, ws.ActionRepositoryScriptUpdate,
		ws.ActionRepositoryScriptDelete,
	)
}

// TestRegisterProcessRoutesWiresSessionScopedRoutes pins the process group,
// including the session-level ACP actions that share the :id parameter.
func TestRegisterProcessRoutesWiresSessionScopedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	log := newTestLogger(t)
	repo := workflowFixture()
	svc, _ := newWorkflowService(t, repo)

	RegisterProcessRoutes(router, svc, newLifecycleManager(t, log), log)

	requireRoutes(t, router,
		"POST /api/v1/task-sessions/:id/processes/start",
		"POST /api/v1/task-sessions/:id/processes/:processId/stop",
		"GET /api/v1/task-sessions/:id/processes",
		"GET /api/v1/task-sessions/:id/processes/:processId",
		"GET /api/v1/task-sessions/:id/agentctl/metrics",
		"POST /api/v1/task-sessions/:id/set-mode",
		"POST /api/v1/task-sessions/:id/set-model",
		"POST /api/v1/task-sessions/:id/set-config-option",
		"POST /api/v1/task-sessions/:id/authenticate",
	)
}
