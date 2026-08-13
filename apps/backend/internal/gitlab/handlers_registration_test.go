package gitlab

import (
	"context"
	"testing"

	"github.com/gin-gonic/gin"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// gitLabWSActions is every action registerWSHandlers wires. Kept explicit so a
// handler added to handlers.go without a corresponding entry here fails
// TestRegisterWSHandlersCoversEveryGitLabAction rather than going untested.
var gitLabWSActions = []string{
	ws.ActionGitLabStatus,
	ws.ActionGitLabTaskMRsList,
	ws.ActionGitLabTaskMRGet,
	ws.ActionGitLabMRFeedbackGet,
	ws.ActionGitLabReviewWatchesList,
	ws.ActionGitLabReviewWatchCreate,
	ws.ActionGitLabReviewWatchUpdate,
	ws.ActionGitLabReviewWatchDelete,
	ws.ActionGitLabReviewTrigger,
	ws.ActionGitLabReviewTriggerAll,
	ws.ActionGitLabMRWatchesList,
	ws.ActionGitLabMRWatchDelete,
	ws.ActionGitLabMRFilesGet,
	ws.ActionGitLabMRCommitsGet,
	ws.ActionGitLabTaskMRSync,
	ws.ActionGitLabStats,
	ws.ActionGitLabMRMerge,
	ws.ActionGitLabMRApprove,
	ws.ActionGitLabMRUnapprove,
	ws.ActionGitLabMRSetLabels,
	ws.ActionGitLabMRSetAssignees,
	ws.ActionGitLabMRDiscussionNew,
	ws.ActionGitLabMRDiscussionResolve,
	ws.ActionGitLabProjectMergeMethodsGet,
	ws.ActionGitLabIssueWatchesList,
	ws.ActionGitLabIssueWatchCreate,
	ws.ActionGitLabIssueWatchUpdate,
	ws.ActionGitLabIssueWatchDelete,
	ws.ActionGitLabIssueTrigger,
	ws.ActionGitLabIssueTriggerAll,
	ws.ActionGitLabActionPresetsList,
	ws.ActionGitLabActionPresetsUpdate,
	ws.ActionGitLabActionPresetsReset,
	ws.ActionGitLabListUserProjects,
	ws.ActionGitLabSearchProjects,
	ws.ActionGitLabProjectBranches,
	ws.ActionGitLabCleanupReviewTasks,
	ws.ActionGitLabCleanupIssueTasks,
}

func TestRegisterWSHandlersCoversEveryGitLabAction(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))
	dispatcher := ws.NewDispatcher()

	registerWSHandlers(dispatcher, svc, log)

	for _, action := range gitLabWSActions {
		if !dispatcher.HasHandler(action) {
			t.Errorf("action %q has no handler registered", action)
		}
	}
	if got := dispatcher.HandlerCount(); got != len(gitLabWSActions) {
		t.Errorf("handler count = %d, want %d; a handler was added or removed "+
			"without updating gitLabWSActions", got, len(gitLabWSActions))
	}
}

// TestRegisterRoutesWithDispatcherLeavesWSSurfaceUnmounted pins the
// authorization boundary documented in handlers.go: the legacy GitLab WS
// handlers are workspace-unscoped, so RegisterRoutesWithDispatcher must mount
// the HTTP surface only and leave the dispatcher untouched. Wiring
// registerWSHandlers back in here would silently expose every workspace's
// watches over a connection that carries no workspace scope.
func TestRegisterRoutesWithDispatcherLeavesWSSurfaceUnmounted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))
	dispatcher := ws.NewDispatcher()
	router := gin.New()

	RegisterRoutesWithDispatcher(router, dispatcher, svc, log)

	if got := dispatcher.HandlerCount(); got != 0 {
		t.Fatalf("dispatcher has %d handlers, want 0 — the GitLab WS surface is "+
			"deliberately unmounted because it cannot carry the workspace "+
			"authorization boundary the HTTP routes enforce", got)
	}
	for _, action := range gitLabWSActions {
		if dispatcher.HasHandler(action) {
			t.Errorf("action %q was mounted on the dispatcher", action)
		}
	}
}

func TestRegisterRoutesWithDispatcherMountsHTTPRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))
	router := gin.New()

	RegisterRoutesWithDispatcher(router, ws.NewDispatcher(), svc, log)

	want := map[string]string{
		"GET":    "/api/v1/gitlab/status",
		"POST":   "/api/v1/gitlab/watches/review",
		"DELETE": "/api/v1/gitlab/watches/review/:id",
		"PUT":    "/api/v1/gitlab/action-presets",
	}
	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for method, path := range want {
		if !routes[method+" "+path] {
			t.Errorf("route %s %s is not registered", method, path)
		}
	}
}

// TestRegisterRoutesMatchesDispatcherVariant guards the package-level
// RegisterRoutes entrypoint against drifting from the dispatcher-aware sibling:
// both must mount exactly the same HTTP surface.
func TestRegisterRoutesMatchesDispatcherVariant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	plain := gin.New()
	RegisterRoutes(plain, svc, log)
	withDispatcher := gin.New()
	RegisterRoutesWithDispatcher(withDispatcher, ws.NewDispatcher(), svc, log)

	collect := func(router *gin.Engine) map[string]bool {
		out := map[string]bool{}
		for _, route := range router.Routes() {
			out[route.Method+" "+route.Path] = true
		}
		return out
	}
	plainRoutes, dispatcherRoutes := collect(plain), collect(withDispatcher)
	if len(plainRoutes) == 0 {
		t.Fatal("RegisterRoutes mounted no routes at all")
	}
	for route := range plainRoutes {
		if !dispatcherRoutes[route] {
			t.Errorf("route %q is missing from RegisterRoutesWithDispatcher", route)
		}
	}
	for route := range dispatcherRoutes {
		if !plainRoutes[route] {
			t.Errorf("route %q is missing from RegisterRoutes", route)
		}
	}
}

func TestRegisterMockRoutesIsNoOpForNonMockClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, log := newWSFixture(t, NewNoopClient(DefaultHost))
	router := gin.New()

	RegisterMockRoutes(router, svc, log)

	if got := len(router.Routes()); got != 0 {
		t.Errorf("mock routes = %d, want 0 for a non-mock client (%v)", got, router.Routes())
	}
}

func TestRegisterMockRoutesMountsControlEndpointsForMockClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))
	router := gin.New()

	RegisterMockRoutes(router, svc, log)

	if len(router.Routes()) == 0 {
		t.Fatal("no mock control endpoints were registered for a MockClient")
	}
}

// TestWSHandlersDispatchThroughDispatcher proves the registered closures are
// reachable by action name and return their own action in the reply, which is
// what lets the frontend correlate a response with the request that caused it.
func TestWSHandlersDispatchThroughDispatcher(t *testing.T) {
	client := NewMockClient(DefaultHost)
	client.SetUser("alice")
	svc, _, log := newWSFixture(t, client)
	dispatcher := ws.NewDispatcher()
	registerWSHandlers(dispatcher, svc, log)

	resp, err := dispatcher.Dispatch(context.Background(),
		wsRequest(t, ws.ActionGitLabStatus, nil))

	var status Status
	wsOK(t, resp, err, ws.ActionGitLabStatus, &status)
	if status.Username != "alice" {
		t.Errorf("username = %q, want alice", status.Username)
	}
}
