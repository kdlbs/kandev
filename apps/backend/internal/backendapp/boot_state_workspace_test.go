package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/webapp"
)

// authenticatedSPARoutes are the route classifications that render the app
// shell. Every one of them must boot with a workspaces block naming an active
// workspace: the SPA derives Office-vs-kanban chrome from the active workspace
// record, so a payload that omits it (or names none) leaves the sidebar unable
// to tell which mode it is in until a client fetch lands.
//
// Pre-auth routes are absent by design — they render the login/setup screens
// and deliberately carry no data.
//
// RouteOffice is also absent, deliberately: its payload comes from
// addOfficeRouteState, which emits nothing before onboarding completes (the
// setup wizard boots bare), so the invariant does not hold unconditionally
// there. The completed-onboarding path needs the office services this harness
// does not construct; the office e2e suite covers that boot end to end.
var authenticatedSPARoutes = []webapp.RouteName{
	webapp.RouteHome,
	webapp.RouteUnknown,
	webapp.RouteTasks,
	webapp.RouteSettings,
	webapp.RouteGitHub,
	webapp.RouteGitLab,
	webapp.RouteJira,
	webapp.RouteLinear,
	webapp.RouteStats,
}

func bootStateParamsForTest(t *testing.T) routeParams {
	t.Helper()
	harness := newBootStateTestHarness(t)
	return routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}
}

func TestBootInitialStateNamesAnActiveWorkspaceOnEverySPARoute(t *testing.T) {
	params := bootStateParamsForTest(t)

	for _, route := range authenticatedSPARoutes {
		t.Run(string(route), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: route})

			workspaces, ok := state["workspaces"].(map[string]any)
			if !ok {
				t.Fatalf("route %s: boot state has no workspaces block", route)
			}
			activeID, _ := workspaces["activeId"].(string)
			if activeID == "" {
				t.Fatalf("route %s: boot state names no active workspace (activeId=%v)", route, workspaces["activeId"])
			}
		})
	}
}

func TestBootInitialStateSettingsPrefersTheActiveWorkspaceCookie(t *testing.T) {
	params := bootStateParamsForTest(t)
	workspaces, err := params.taskSvc.ListWorkspaces(t.Context())
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("list workspaces: %v", err)
	}
	// A second workspace so "the cookie won" is distinguishable from "the first
	// workspace won".
	created, err := params.taskSvc.CreateWorkspace(t.Context(), &taskservice.CreateWorkspaceRequest{
		Name: "Second workspace",
	})
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	req.AddCookie(&http.Cookie{Name: activeWorkspaceCookie, Value: created.ID})

	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{
		Route: webapp.RouteSettings,
		Path:  "/settings",
	})

	block, ok := state["workspaces"].(map[string]any)
	if !ok {
		t.Fatal("settings boot state has no workspaces block")
	}
	if got := block["activeId"]; got != created.ID {
		t.Fatalf("activeId = %v, want %s", got, created.ID)
	}
}
