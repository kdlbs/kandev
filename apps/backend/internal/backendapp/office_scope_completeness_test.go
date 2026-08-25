package backendapp

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"

	"github.com/kandev/kandev/internal/office"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/approvals"
	"github.com/kandev/kandev/internal/office/channels"
	officeconfig "github.com/kandev/kandev/internal/office/config"
	"github.com/kandev/kandev/internal/office/costs"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/labels"
	"github.com/kandev/kandev/internal/office/onboarding"
	"github.com/kandev/kandev/internal/office/projects"
	"github.com/kandev/kandev/internal/office/routines"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/skills"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// registeredOfficeRoutes returns every route office.RegisterAllRoutes mounts,
// as gin reports them. The services are zero values on purpose: only route
// REGISTRATION is exercised here, never a handler, and a zero value is enough
// to get past the `if h == nil || h.svc == nil { return }` guards that would
// otherwise silently drop whole route groups from the table this test walks.
func registeredOfficeRoutes(t *testing.T) gin.RoutesInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	office.RegisterAllRoutes(engine.Group(officeRoutePrefix), officeTestServices(), testLogger(t))

	routes := engine.Routes()
	if len(routes) < 100 {
		t.Fatalf("only %d Office routes registered; the table this test walks is not the real one", len(routes))
	}
	return routes
}

// officeRouteCoverage explains how one route is scoped, or why it is not.
type officeRouteCoverage struct {
	how       string
	uncovered string
}

// classifyOfficeRoute mirrors authorizeOfficeRequest's decision, on a route
// PATTERN rather than a live request.
//
// A `:wsId` param covers the route, but it does NOT excuse the route's other
// id params. That exemption is what let the mixed-parameter label routes ship
// unscoped: `/workspaces/:wsId/labels/:id` looked "covered" while its handler
// mutated by label id alone. Every non-workspace param must still resolve.
func classifyOfficeRoute(route string, resolvers map[string]officeWorkspaceResolver) officeRouteCoverage {
	if _, ok := officeWorkspacelessRoute(route); ok {
		return officeRouteCoverage{how: "workspace-less allowlist"}
	}
	var (
		hasWorkspaceParam bool
		resolved          []string
		unknown           []string
	)
	for _, key := range officeRouteParamKeys(route) {
		switch {
		case paramOfScopeKey(key) == officeWorkspaceParam:
			hasWorkspaceParam = true
		case resolvers[key] != nil:
			resolved = append(resolved, key)
		case officeScopedSubResourceParams[key] != "":
			// A child of a resource checked on the same route.
		default:
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		return officeRouteCoverage{uncovered: "unresolvable id param(s) " + strings.Join(unknown, ", ")}
	}
	switch {
	case hasWorkspaceParam && len(resolved) > 0:
		return officeRouteCoverage{how: "wsId param + resolver " + strings.Join(resolved, ", ")}
	case hasWorkspaceParam:
		return officeRouteCoverage{how: "wsId param"}
	case len(resolved) > 0:
		return officeRouteCoverage{how: "resolver " + strings.Join(resolved, ", ")}
	case officeBodyScopeResolvers[route] != nil:
		return officeRouteCoverage{how: "body resolver"}
	}
	return officeRouteCoverage{uncovered: "no :wsId, no id param, no body resolver, not on the allowlist"}
}

// TestOfficeRouteScopeCompleteness is the point of this whole guard: it walks
// the REGISTERED Office route table and fails if any route can reach a
// handler without a workspace check.
//
// The original hole was not that one handler forgot a check — it was that
// ~50 by-ID routes were never in scope for the middleware at all, and nothing
// noticed. Without this test the fix rots on the next PR that adds a route.
//
// If this fails on a route you just added, pick one:
//   - key it by :wsId, or
//   - register a resolver in officeParamScopeResolvers for its resource kind
//     (add a WorkspaceIDFor... lookup to the office repository), or
//   - list its param in officeScopedSubResourceParams if it is a child of a
//     resource already checked on the same route, or
//   - add it to officeWorkspacelessRoutes/Prefixes WITH the reason it needs
//     no workspace.
func TestOfficeRouteScopeCompleteness(t *testing.T) {
	resolvers := officeParamScopeResolvers(nil)
	var uncovered []string
	for _, route := range registeredOfficeRoutes(t) {
		relative, ok := officeRelativeRoute(route.Path)
		if !ok {
			t.Errorf("route %s %s is not under %s", route.Method, route.Path, officeRoutePrefix)
			continue
		}
		if coverage := classifyOfficeRoute(relative, resolvers); coverage.uncovered != "" {
			uncovered = append(uncovered, route.Method+" "+route.Path+": "+coverage.uncovered)
		}
	}
	if len(uncovered) > 0 {
		sort.Strings(uncovered)
		t.Fatalf("%d Office route(s) reach a handler with no workspace check:\n  %s",
			len(uncovered), strings.Join(uncovered, "\n  "))
	}
}

// TestOfficeScopeTablesHaveNoDeadEntries guards the other direction. An
// allowlist entry for a route that no longer exists, or a sub-resource param
// nothing uses, is a standing invitation to reintroduce it by name later.
func TestOfficeScopeTablesHaveNoDeadEntries(t *testing.T) {
	routes := registeredOfficeRoutes(t)
	relatives := make([]string, 0, len(routes))
	paramKeys := map[string]bool{}
	for _, route := range routes {
		relative, ok := officeRelativeRoute(route.Path)
		if !ok {
			continue
		}
		relatives = append(relatives, relative)
		for _, key := range officeRouteParamKeys(relative) {
			paramKeys[key] = true
		}
	}
	matches := func(pred func(string) bool) bool {
		for _, relative := range relatives {
			if pred(relative) {
				return true
			}
		}
		return false
	}

	for route := range officeWorkspacelessRoutes {
		if !matches(func(r string) bool { return r == route }) {
			t.Errorf("officeWorkspacelessRoutes has %q, which is not a registered Office route", route)
		}
	}
	for prefix := range officeWorkspacelessPrefixes {
		if !matches(func(r string) bool { return strings.HasPrefix(r, prefix) }) {
			t.Errorf("officeWorkspacelessPrefixes has %q, which no registered Office route starts with", prefix)
		}
	}
	for route := range officeBodyScopeResolvers {
		if !matches(func(r string) bool { return r == route }) {
			t.Errorf("officeBodyScopeResolvers has %q, which is not a registered Office route", route)
		}
	}
	for key := range officeScopedSubResourceParams {
		if !paramKeys[key] {
			t.Errorf("officeScopedSubResourceParams has %q, which no registered Office route uses", key)
		}
	}
	for key := range officeParamScopeResolvers(nil) {
		if !paramKeys[key] {
			t.Errorf("officeParamScopeResolvers has %q, which no registered Office route uses", key)
		}
	}
}

// TestOfficeWorkspacelessEntriesCarryAReason keeps the allowlist honest: an
// entry with no stated reason is how a route quietly opts out.
func TestOfficeWorkspacelessEntriesCarryAReason(t *testing.T) {
	for route, reason := range officeWorkspacelessRoutes {
		if len(strings.TrimSpace(reason)) < 20 {
			t.Errorf("officeWorkspacelessRoutes[%q] reason %q is too thin to review", route, reason)
		}
	}
	for prefix, reason := range officeWorkspacelessPrefixes {
		if len(strings.TrimSpace(reason)) < 20 {
			t.Errorf("officeWorkspacelessPrefixes[%q] reason %q is too thin to review", prefix, reason)
		}
	}
	for key, reason := range officeScopedSubResourceParams {
		if len(strings.TrimSpace(reason)) < 20 {
			t.Errorf("officeScopedSubResourceParams[%q] reason %q is too thin to review", key, reason)
		}
	}
}

// TestOfficeScopeCasesMatchRegisteredRoutes links the behavioural table in
// office_scope_test.go to the real route table, so renaming an Office route
// breaks the test that exercises it instead of silently unhooking it.
func TestOfficeScopeCasesMatchRegisteredRoutes(t *testing.T) {
	registered := map[string]bool{}
	for _, route := range registeredOfficeRoutes(t) {
		registered[route.Method+" "+route.Path] = true
	}
	for _, tc := range officeScopeCases() {
		if !registered[tc.method+" "+tc.pattern] {
			t.Errorf("officeScopeCases exercises %s %s, which is not a registered Office route",
				tc.method, tc.pattern)
		}
	}
}

// officeTestServices returns the zero-value service set registeredOfficeRoutes
// uses, so the production mount can be exercised without building real Office
// services.
func officeTestServices() *office.Services {
	return &office.Services{
		Agents:       &officeagents.AgentService{},
		Skills:       &skills.SkillService{},
		Projects:     &projects.ProjectService{},
		Costs:        &costs.CostService{},
		Routines:     &routines.RoutineService{},
		Approvals:    &approvals.ApprovalService{},
		Channels:     &channels.ChannelService{},
		Config:       &officeconfig.ConfigService{},
		Dashboard:    &dashboard.DashboardService{},
		Labels:       &labels.LabelService{},
		Onboarding:   &onboarding.OnboardingService{},
		TreeControls: &officeservice.Service{},
		Workspaces:   &officeservice.Service{},
		Documents:    &taskservice.DocumentService{},
	}
}

// officeScopeFixtureIDs maps each resolver key to the id prefix
// seedOfficeResources uses for that resource kind, so a route pattern can be
// turned into a concrete URL naming a chosen workspace's rows.
//
// TestOfficeMultiIDRoutesRejectMixedWorkspaces asserts every registered
// resolver has an entry, so adding a resource kind forces a fixture and
// cannot quietly drop out of the relationship sweep below.
var officeScopeFixtureIDs = map[string]string{
	"agents/:id":                  "agent",
	"tasks/:id":                   "task",
	"tasks/:taskId":               "task",
	"runs/:id":                    "run",
	"runs/:runId":                 "run",
	"routines/:id":                "routine",
	"routine-triggers/:triggerId": "trigger",
	"routine-triggers/:publicId":  "public",
	"projects/:id":                "project",
	"skills/:id":                  "skill",
	"budgets/:id":                 "budget",
	"approvals/:id":               "approval",
	"channels/:channelId":         "channel",
	"labels/:id":                  "label",
	"reviewers/:agentId":          "agent",
	"approvers/:agentId":          "agent",
}

// officeRouteURL renders a route pattern into a concrete URL. Every
// workspace-bearing param takes its id from workspaces[i] (clamped to the
// last entry), so passing one workspace builds a same-workspace request and
// passing two builds a mixed one. Sub-resource params get a literal.
func officeRouteURL(route string, suffixes []string, workspaceIDs []string) (string, int) {
	rendered := route
	bearing := 0
	pick := func(list []string) string {
		if bearing < len(list) {
			return list[bearing]
		}
		return list[len(list)-1]
	}
	for _, key := range officeRouteParamKeys(route) {
		param := paramOfScopeKey(key)
		var value string
		switch {
		case param == officeWorkspaceParam:
			value = pick(workspaceIDs)
			bearing++
		case officeScopeFixtureIDs[key] != "":
			value = officeScopeFixtureIDs[key] + "-" + pick(suffixes)
			bearing++
		default:
			value = "sub-resource"
		}
		rendered = strings.Replace(rendered, param, value, 1)
	}
	return rendered, bearing
}

// TestOfficeMultiIDRoutesRejectMixedWorkspaces is the structural form of the
// relationship rule, generated from the REAL route table rather than a
// hand-written case list.
//
// Five sibling audit fixes hit the same pattern: each individual id check was
// right, the RELATIONSHIP between the ids was unchecked, and every one of
// those PRs had passing tests. A completeness test that only asks "does this
// route have a check" cannot see that class. So for every registered route
// naming two or more workspace-bearing ids, this drives the guard twice:
//
//   - control — all ids from ONE workspace the caller owns, must reach the
//     handler. Without this the mismatch assertion below would pass on a
//     malformed URL and prove nothing.
//   - mixed — the first id from that workspace and the rest from a SECOND
//     workspace the same caller also owns, must be refused before dispatch.
//
// A newly added route naming two resources is covered the day it is added.
func TestOfficeMultiIDRoutesRejectMixedWorkspaces(t *testing.T) {
	for key := range officeParamScopeResolvers(nil) {
		if officeScopeFixtureIDs[key] == "" {
			t.Fatalf("resolver %q has no officeScopeFixtureIDs entry, so it is invisible "+
				"to the mixed-workspace sweep; add one (and seed the row in seedOfficeResources)", key)
		}
	}

	h := newOfficeScopeHarness(t)
	wsA1, wsA2 := h.workspaces[officeScopeUserA], h.workspaces[officeScopeUserASecond]
	checked := 0

	for _, registered := range registeredOfficeRoutes(t) {
		route, ok := officeRelativeRoute(registered.Path)
		if !ok {
			continue
		}
		if _, allowed := officeWorkspacelessRoute(route); allowed {
			continue
		}
		if _, bearing := officeRouteURL(route, []string{officeScopeUserA}, []string{wsA1}); bearing < 2 {
			continue
		}
		checked++
		t.Run(registered.Method+" "+route, func(t *testing.T) {
			same, _ := officeRouteURL(route,
				[]string{officeScopeUserA}, []string{wsA1})
			mixed, _ := officeRouteURL(route,
				[]string{officeScopeUserA, officeScopeUserASecond}, []string{wsA1, wsA2})

			if code, reached := officeProbeRequest(t, h, registered.Method, route, same); !reached {
				t.Fatalf("control %s returned %d without reaching the handler; "+
					"the mixed assertion below would be vacuous", same, code)
			}
			code, reached := officeProbeRequest(t, h, registered.Method, route, mixed)
			if code != http.StatusNotFound || reached {
				t.Errorf("mixed-workspace %s: status = %d, reached = %v; want 404 and false",
					mixed, code, reached)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no multi-id Office routes were exercised; the sweep is not running")
	}
	t.Logf("mixed-workspace sweep covered %d multi-id Office routes", checked)
}

// officeProbeRequest mounts the guard in front of a recording probe on one
// route pattern and drives target through it as user A.
func officeProbeRequest(t *testing.T, h *officeScopeHarness, method, route, target string) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authn.WithIdentity(
			c.Request.Context(), authn.Identity{UserID: officeScopeUserA, Role: authn.RoleMember}))
		c.Next()
	})
	group := engine.Group(officeRoutePrefix)
	group.Use(officeWorkspaceScopeMiddleware(h.authSvc, h.taskSvc, h.officeRepo))
	reached := false
	group.Handle(method, route, func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(method, officeRoutePrefix+target, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec.Code, reached
}
