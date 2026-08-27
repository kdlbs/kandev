package backendapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
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

// scopedRequest builds a GET request on the given ported Host with the given
// cookies set (browser-jar style: several cookies may coexist).
func scopedRequest(host string, cookies ...http.Cookie) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = host
	for _, cookie := range cookies {
		req.AddCookie(&cookie)
	}
	return req
}

func bootStateActiveID(t *testing.T, req *http.Request, params routeParams, route webapp.RouteName) string {
	t.Helper()
	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: route})
	block, ok := state["workspaces"].(map[string]any)
	if !ok {
		t.Fatalf("route %s: no workspaces block in state %v", route, state)
	}
	activeID, _ := block["activeId"].(string)
	return activeID
}

// TestReadActiveWorkspaceCookiePrefersScopedName pins the reader contract:
// the port-scoped general name wins over the legacy unprefixed one on a
// ported Host.
func TestReadActiveWorkspaceCookiePrefersScopedName(t *testing.T) {
	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "kandev-active-workspace", Value: "legacy-1"},
		http.Cookie{Name: "kandev-active-workspace_8443", Value: "scoped-1"},
	)
	if got := readActiveWorkspaceCookie(req); got != "scoped-1" {
		t.Fatalf("readActiveWorkspaceCookie = %q, want scoped-1", got)
	}
}

// TestReadActiveWorkspaceCookieFallsBackToLegacy pins the read-only legacy
// fallback: a pre-upgrade selection survives when no scoped cookie exists.
func TestReadActiveWorkspaceCookieFallsBackToLegacy(t *testing.T) {
	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "kandev-active-workspace", Value: "legacy-1"},
	)
	if got := readActiveWorkspaceCookie(req); got != "legacy-1" {
		t.Fatalf("readActiveWorkspaceCookie = %q, want legacy-1", got)
	}
}

// TestReadActiveWorkspaceCookieIgnoresOfficeFamily pins family separation:
// the general reader never consults the office-family cookies (pre-change it
// read the legacy office name — fails pre-change).
func TestReadActiveWorkspaceCookieIgnoresOfficeFamily(t *testing.T) {
	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "office-active-workspace_8443", Value: "office-scoped"},
		http.Cookie{Name: "office-active-workspace", Value: "office-legacy"},
	)
	if got := readActiveWorkspaceCookie(req); got != "" {
		t.Fatalf("readActiveWorkspaceCookie = %q, want empty (office family must be ignored)", got)
	}
}

// TestReadOfficeWorkspaceCookieScopedFirstLegacyFallback pins the office
// reader: scoped office name first, legacy office name as fallback.
func TestReadOfficeWorkspaceCookieScopedFirstLegacyFallback(t *testing.T) {
	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "office-active-workspace", Value: "legacy"},
		http.Cookie{Name: "office-active-workspace_8443", Value: "scoped"},
	)
	if got := readOfficeWorkspaceCookie(req); got != "scoped" {
		t.Fatalf("readOfficeWorkspaceCookie = %q, want scoped", got)
	}
	req = scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "office-active-workspace", Value: "legacy"},
	)
	if got := readOfficeWorkspaceCookie(req); got != "legacy" {
		t.Fatalf("readOfficeWorkspaceCookie = %q, want legacy", got)
	}
}

// TestReadOfficeWorkspaceCookieIgnoresGeneralFamily pins the office reader's
// family boundary: the general cookie is never an office candidate.
func TestReadOfficeWorkspaceCookieIgnoresGeneralFamily(t *testing.T) {
	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "kandev-active-workspace_8443", Value: "general-scoped"},
		http.Cookie{Name: "kandev-active-workspace", Value: "general-legacy"},
	)
	if got := readOfficeWorkspaceCookie(req); got != "" {
		t.Fatalf("readOfficeWorkspaceCookie = %q, want empty (general family must be ignored)", got)
	}
}

// TestBootActiveWorkspaceCookieIsolatedPerHost models the browser jar: BOTH
// ports' scoped cookies are present in one request, and each Host resolves
// its own port's selection. Failing-before: the reader ignored all scoped
// names, so both requests resolved the first workspace (non-first fixtures).
func TestBootActiveWorkspaceCookieIsolatedPerHost(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	repo := harness.taskRepo
	for _, workspace := range []*models.Workspace{
		{ID: "ws-first", Name: "First"},
		{ID: "ws-a", Name: "A"},
		{ID: "ws-b", Name: "B"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	params := routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}

	jar := []http.Cookie{
		{Name: "kandev-active-workspace_8443", Value: "ws-a"},
		{Name: "kandev-active-workspace_9443", Value: "ws-b"},
	}
	if got := bootStateActiveID(t, scopedRequest("127.0.0.1:8443", jar...), params, webapp.RouteHome); got != "ws-a" {
		t.Fatalf("Host 8443 activeId = %q, want ws-a", got)
	}
	if got := bootStateActiveID(t, scopedRequest("127.0.0.1:9443", jar...), params, webapp.RouteHome); got != "ws-b" {
		t.Fatalf("Host 9443 activeId = %q, want ws-b", got)
	}
}

// TestBootActiveWorkspaceLegacyFallbackAndScopedPrecedence covers the
// downstream boot-state regression (scenario 3): with no scoped cookie a
// VALID unprefixed legacy id is selected (invariant), a foreign/invalid
// legacy id falls through to the instance-owned fallback (invariant), and
// when both exist the scoped value wins (failing-before).
func TestBootActiveWorkspaceLegacyFallbackAndScopedPrecedence(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	repo := harness.taskRepo
	// ListWorkspaces orders by created_at DESC, so ws-first (created last) is
	// the fallback "first workspace" — distinct from the legacy selection.
	for _, workspace := range []*models.Workspace{
		{ID: "ws-legacy", Name: "Legacy"},
		{ID: "ws-first", Name: "First"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	params := routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}

	// Invariant: valid unprefixed legacy id selected when no scoped cookie.
	req := scopedRequest("127.0.0.1:8443", http.Cookie{Name: "kandev-active-workspace", Value: "ws-legacy"})
	if got := bootStateActiveID(t, req, params, webapp.RouteHome); got != "ws-legacy" {
		t.Fatalf("valid legacy id: activeId = %q, want ws-legacy", got)
	}

	// Invariant: foreign/invalid legacy id falls to the first workspace.
	req = scopedRequest("127.0.0.1:8443", http.Cookie{Name: "kandev-active-workspace", Value: "ws-foreign"})
	if got := bootStateActiveID(t, req, params, webapp.RouteHome); got != "ws-first" {
		t.Fatalf("foreign legacy id: activeId = %q, want ws-first", got)
	}

	// Failing-before: scoped + legacy both present — the scoped value wins.
	req = scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "kandev-active-workspace", Value: "ws-legacy"},
		http.Cookie{Name: "kandev-active-workspace_8443", Value: "ws-first"},
	)
	if got := bootStateActiveID(t, req, params, webapp.RouteHome); got != "ws-first" {
		t.Fatalf("scoped over legacy: activeId = %q, want ws-first", got)
	}
}

// TestBootGenericRoutesIgnoreOfficeCookies drives the downstream generic-route
// family separation: every generic route that resolves an active workspace id
// resolves settings/first — never the office id — when only office-family
// cookies are present. The legacy-office row fails before the change (the old
// generic reader consulted the legacy office name); the scoped-office row is
// the paired invariant guard.
func TestBootGenericRoutesIgnoreOfficeCookies(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	repo := harness.taskRepo
	for _, workspace := range []*models.Workspace{
		{ID: "ws-o1", Name: "Office 1", OfficeWorkflowID: "wf-1"},
		{ID: "ws-o2", Name: "Office 2", OfficeWorkflowID: "wf-2"},
		{ID: "ws-kanban", Name: "Kanban"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	settingsID := "ws-kanban"
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID: &settingsID,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	params := routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}

	routes := []webapp.RouteName{
		webapp.RouteUnknown,
		webapp.RouteHome,
		webapp.RouteTasks,
		webapp.RouteSettings,
		webapp.RouteGitHub,
		webapp.RouteGitLab,
		webapp.RouteJira,
		webapp.RouteLinear,
		webapp.RouteStats,
	}
	for _, route := range routes {
		t.Run(string(route)+"/legacy-office-cookie", func(t *testing.T) {
			req := scopedRequest("127.0.0.1:8443",
				http.Cookie{Name: "office-active-workspace", Value: "ws-o1"},
			)
			if got := bootStateActiveID(t, req, params, route); got != "ws-kanban" {
				t.Fatalf("activeId = %q, want settings workspace ws-kanban (office cookie must be ignored)", got)
			}
		})
		t.Run(string(route)+"/scoped-office-cookie", func(t *testing.T) {
			req := scopedRequest("127.0.0.1:8443",
				http.Cookie{Name: "office-active-workspace_8443", Value: "ws-o1"},
			)
			if got := bootStateActiveID(t, req, params, route); got != "ws-kanban" {
				t.Fatalf("activeId = %q, want settings workspace ws-kanban (scoped office cookie must be ignored)", got)
			}
		})
	}

	// Tasks routeData resolves the same settings/first workspace.
	t.Run("tasks-route-data", func(t *testing.T) {
		req := scopedRequest("127.0.0.1:8443",
			http.Cookie{Name: "office-active-workspace_8443", Value: "ws-o1"},
		)
		routeData := bootRouteData(ctx, req, params, webapp.RouteClassification{Route: webapp.RouteTasks})
		tasksPage, ok := routeData["tasksPage"].(map[string]any)
		if !ok {
			t.Fatalf("tasks routeData missing tasksPage: %v", routeData)
		}
		if got := tasksPage["activeWorkspaceId"]; got != "ws-kanban" {
			t.Fatalf("tasks routeData activeWorkspaceId = %v, want ws-kanban", got)
		}
	})
}

// TestBootQuickChatFallsBackToSettingsWithOnlyOfficeCookies covers the
// quick-chat boot fallback separately: on a non-task route with only an
// office cookie present, quick chat follows settings/first — never the office
// id.
func TestBootQuickChatFallsBackToSettingsWithOnlyOfficeCookies(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	repo := harness.taskRepo
	for _, workspace := range []*models.Workspace{
		{ID: "ws-o1", Name: "Office 1", OfficeWorkflowID: "wf-1"},
		{ID: "ws-kanban", Name: "Kanban"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	settingsID := "ws-kanban"
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID: &settingsID,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	seedBootQuickChatTask(t, repo, ctx, bootQuickChatFixture{
		id: "chat-kanban", workspaceID: "ws-kanban", title: "Settings Chat",
		updatedAt: base, sessionUpdatedAt: base, agentProfileID: "agent-1",
	})
	seedBootQuickChatTask(t, repo, ctx, bootQuickChatFixture{
		id: "chat-office", workspaceID: "ws-o1", title: "Office Chat",
		updatedAt: base.Add(time.Minute), sessionUpdatedAt: base.Add(time.Minute), agentProfileID: "agent-2",
	})

	req := scopedRequest("127.0.0.1:8443",
		http.Cookie{Name: "office-active-workspace_8443", Value: "ws-o1"},
	)
	params := routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}
	state := bootInitialState(t.Context(), req, params, webapp.RouteClassification{Route: webapp.RouteHome})
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Marshal state: %v", err)
	}
	var decoded struct {
		QuickChat struct {
			Sessions []struct {
				SessionID   string `json:"sessionId"`
				WorkspaceID string `json:"workspaceId"`
			} `json:"sessions"`
		} `json:"quickChat"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal state: %v", err)
	}
	sessions := decoded.QuickChat.Sessions
	if len(sessions) != 1 {
		t.Fatalf("quickChat sessions = %#v, want exactly the settings workspace's session", sessions)
	}
	if got := sessions[0].WorkspaceID; got != "ws-kanban" {
		t.Fatalf("quickChat session workspaceId = %v, want ws-kanban (settings), not the office cookie's", got)
	}
}

// officeWorkspacesPrecedenceHarness builds three office workspaces and a
// kanban workspace with REAL user settings pointing at O2. ListWorkspaces
// orders by created_at DESC, so ws-o3 (created last) is the first office
// fallback — distinct from O1/O2, which the non-first fixtures need.
func officeWorkspacesPrecedenceHarness(t *testing.T) (bootStateBuilder, context.Context) {
	t.Helper()
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	repo := harness.taskRepo
	for _, workspace := range []*models.Workspace{
		{ID: "ws-o1", Name: "Office 1", OfficeWorkflowID: "wf-1"},
		{ID: "ws-o2", Name: "Office 2", OfficeWorkflowID: "wf-2"},
		{ID: "ws-o3", Name: "Office 3 (first fallback)", OfficeWorkflowID: "wf-3"},
		{ID: "ws-kanban", Name: "Kanban"},
	} {
		if err := repo.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", workspace.ID, err)
		}
	}
	settingsID := "ws-o2"
	if _, err := harness.userSvc.UpdateUserSettings(ctx, &userservice.UpdateUserSettingsRequest{
		WorkspaceID: &settingsID,
	}); err != nil {
		t.Fatalf("UpdateUserSettings: %v", err)
	}
	return bootStateBuilder{p: routeParams{taskSvc: harness.taskSvc, userCtrl: harness.userCtrl}}, ctx
}

// TestOfficeWorkspacesScopedCookiePrecedence drives the office boot
// resolution with SCOPED cookie names: general → office → settings → first.
// Every row is failing-before — the scoped names did not exist pre-change, so
// the reader returned empty and the resolver fell back to the first office
// workspace (never the asserted non-first winner).
func TestOfficeWorkspacesScopedCookiePrecedence(t *testing.T) {
	builder, ctx := officeWorkspacesPrecedenceHarness(t)

	cases := []struct {
		name    string
		cookies []http.Cookie
		want    string
	}{
		{"scoped general kanban + scoped office O2 → O2", []http.Cookie{
			{Name: "kandev-active-workspace_8443", Value: "ws-kanban"},
			{Name: "office-active-workspace_8443", Value: "ws-o2"},
		}, "ws-o2"},
		{"scoped general O1 + scoped office O2 → O1", []http.Cookie{
			{Name: "kandev-active-workspace_8443", Value: "ws-o1"},
			{Name: "office-active-workspace_8443", Value: "ws-o2"},
		}, "ws-o1"},
		{"scoped office O1 + settings O2 → O1", []http.Cookie{
			{Name: "office-active-workspace_8443", Value: "ws-o1"},
		}, "ws-o1"},
		{"scoped general O1 only → O1", []http.Cookie{
			{Name: "kandev-active-workspace_8443", Value: "ws-o1"},
		}, "ws-o1"},
		{"no cookies → settings O2 (new settings candidate)", nil, "ws-o2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, activeID, err := builder.officeWorkspaces(ctx, scopedRequest("127.0.0.1:8443", tc.cookies...))
			if err != nil {
				t.Fatalf("officeWorkspaces: %v", err)
			}
			if activeID != tc.want {
				t.Fatalf("activeID = %q, want %q", activeID, tc.want)
			}
		})
	}
}

// TestOfficeWorkspacesLegacyCookiePrecedence drives the office boot
// resolution with LEGACY (unprefixed) names. The kanban-general row is
// failing-before (pre-change the office cookie was never consulted once the
// general cookie named a kanban workspace); the office-wins rows are
// invariants (office beat general-miss and settings both before and after).
func TestOfficeWorkspacesLegacyCookiePrecedence(t *testing.T) {
	builder, ctx := officeWorkspacesPrecedenceHarness(t)

	cases := []struct {
		name    string
		cookies []http.Cookie
		want    string
	}{
		{"legacy general kanban + legacy office O2 → O2", []http.Cookie{
			{Name: "kandev-active-workspace", Value: "ws-kanban"},
			{Name: "office-active-workspace", Value: "ws-o2"},
		}, "ws-o2"},
		{"legacy general O1 + legacy office O2 → O1", []http.Cookie{
			{Name: "kandev-active-workspace", Value: "ws-o1"},
			{Name: "office-active-workspace", Value: "ws-o2"},
		}, "ws-o1"},
		{"legacy office O1 + settings O2 → O1", []http.Cookie{
			{Name: "office-active-workspace", Value: "ws-o1"},
		}, "ws-o1"},
		{"legacy general O1 only → O1", []http.Cookie{
			{Name: "kandev-active-workspace", Value: "ws-o1"},
		}, "ws-o1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, activeID, err := builder.officeWorkspaces(ctx, scopedRequest("127.0.0.1:8443", tc.cookies...))
			if err != nil {
				t.Fatalf("officeWorkspaces: %v", err)
			}
			if activeID != tc.want {
				t.Fatalf("activeID = %q, want %q", activeID, tc.want)
			}
		})
	}
}
