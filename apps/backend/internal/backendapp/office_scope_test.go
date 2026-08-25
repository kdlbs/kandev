package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// officeScopeHarness is one SQLite database carrying two workspaces owned by
// two different users, plus one row of every Office resource kind in each, so
// a request for workspace A's resource can be driven with workspace B's
// owner's identity.
type officeScopeHarness struct {
	taskSvc    *taskservice.Service
	officeRepo *officesqlite.Repository
	authSvc    *auth.Service
	workspaces map[string]string // owner user id -> workspace id
}

const (
	officeScopeUserA = "user-a"
	officeScopeUserB = "user-b"
)

func newOfficeScopeHarness(t *testing.T) *officeScopeHarness {
	t.Helper()
	taskSvc, _, officeRepo := newRunSubscriptionCheckHarness(t)
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720

	h := &officeScopeHarness{
		taskSvc:    taskSvc,
		officeRepo: officeRepo,
		authSvc:    newEnabledAuthService(t, cfg),
		workspaces: map[string]string{},
	}
	for _, owner := range []string{officeScopeUserA, officeScopeUserB} {
		workspaceID := seedOwnedWorkspace(t, taskSvc, owner)
		h.workspaces[owner] = workspaceID
		seedOfficeResources(t, officeRepo, workspaceID, owner)
	}
	return h
}

// seedOfficeResources inserts one row of every Office resource kind owned by
// workspaceID, with ids suffixed by suffix. Raw INSERTs keep the fixture to
// the columns the resolvers read.
func seedOfficeResources(t *testing.T, repo *officesqlite.Repository, workspaceID, suffix string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := repo.ExecRaw(ctx, query, args...); err != nil {
			t.Fatalf("seed %q: %v", query, err)
		}
	}
	exec(`INSERT INTO agents (id, name, workspace_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"agent-"+suffix, "agent-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO agent_profiles (id, agent_id, name, agent_display_name, workspace_id, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, '', ?, ?)`,
		"agent-"+suffix, "agent-"+suffix, "n-"+suffix, "d", workspaceID, now, now)
	exec(`INSERT INTO tasks (id, workspace_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"task-"+suffix, workspaceID, "t", now, now)
	exec(`INSERT INTO office_routines (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"routine-"+suffix, workspaceID, "r", now, now)
	exec(`INSERT INTO office_routine_triggers (id, routine_id, kind, public_id, created_at, updated_at)
		VALUES (?, ?, 'webhook', ?, ?, ?)`,
		"trigger-"+suffix, "routine-"+suffix, "public-"+suffix, now, now)
	exec(`INSERT INTO office_projects (id, workspace_id, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"project-"+suffix, workspaceID, "p", now, now)
	exec(`INSERT INTO office_skills (id, workspace_id, name, slug, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"skill-"+suffix, workspaceID, "s", "s-"+suffix, now, now)
	exec(`INSERT INTO office_budget_policies
		(id, workspace_id, scope_type, scope_id, limit_subcents, period, created_at, updated_at)
		VALUES (?, ?, 'workspace', '', 100, 'monthly', ?, ?)`,
		"budget-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO office_approvals (id, workspace_id, type, created_at, updated_at)
		VALUES (?, ?, 'hire_agent', ?, ?)`,
		"approval-"+suffix, workspaceID, now, now)
	exec(`INSERT INTO office_channels (id, workspace_id, agent_profile_id, platform, created_at, updated_at)
		VALUES (?, ?, ?, 'telegram', ?, ?)`,
		"channel-"+suffix, workspaceID, "agent-"+suffix, now, now)
	if err := repo.CreateRun(ctx, &models.Run{
		ID:             "run-" + suffix,
		AgentProfileID: "agent-" + suffix,
		Reason:         "task_assigned",
		Status:         models.RunStatus("queued"),
		CoalescedCount: 1,
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}
}

// officeScopeCase is one by-ID Office route driven end to end through the
// guard. `path` is a concrete URL for the `-user-a`-suffixed resources.
type officeScopeCase struct {
	name    string
	method  string
	pattern string
	path    string
	body    string
}

// officeScopeCases covers every resource kind the guard resolves. The
// patterns are cross-checked against the REAL Office route table by
// TestOfficeScopeCasesMatchRegisteredRoutes, so a renamed route breaks this
// table instead of silently unhooking it.
func officeScopeCases() []officeScopeCase {
	return []officeScopeCase{
		{"agent", http.MethodGet, "/api/v1/office/agents/:id/memory", "/api/v1/office/agents/agent-user-a/memory", ""},
		{"task", http.MethodGet, "/api/v1/office/tasks/:id/documents", "/api/v1/office/tasks/task-user-a/documents", ""},
		{"task-document", http.MethodGet, "/api/v1/office/tasks/:id/documents/:key",
			"/api/v1/office/tasks/task-user-a/documents/spec", ""},
		{"task-tree", http.MethodPost, "/api/v1/office/tasks/:id/tree/cancel",
			"/api/v1/office/tasks/task-user-a/tree/cancel", ""},
		{"run", http.MethodGet, "/api/v1/office/runs/:id/attempts", "/api/v1/office/runs/run-user-a/attempts", ""},
		{"routine", http.MethodGet, "/api/v1/office/routines/:id", "/api/v1/office/routines/routine-user-a", ""},
		{"routine-trigger", http.MethodDelete, "/api/v1/office/routine-triggers/:triggerId",
			"/api/v1/office/routine-triggers/trigger-user-a", ""},
		{"routine-trigger-public", http.MethodPost, "/api/v1/office/routine-triggers/:publicId/fire",
			"/api/v1/office/routine-triggers/public-user-a/fire", ""},
		{"project", http.MethodGet, "/api/v1/office/projects/:id", "/api/v1/office/projects/project-user-a", ""},
		{"skill", http.MethodGet, "/api/v1/office/skills/:id", "/api/v1/office/skills/skill-user-a", ""},
		{"budget", http.MethodPatch, "/api/v1/office/budgets/:id", "/api/v1/office/budgets/budget-user-a", ""},
		{"approval", http.MethodPost, "/api/v1/office/approvals/:id/decide",
			"/api/v1/office/approvals/approval-user-a/decide", ""},
		{"channel", http.MethodPost, "/api/v1/office/channels/:channelId/inbound",
			"/api/v1/office/channels/channel-user-a/inbound", "hello"},
		{"agent-channel", http.MethodDelete, "/api/v1/office/agents/:id/channels/:channelId",
			"/api/v1/office/agents/agent-user-a/channels/channel-user-a", ""},
		{"workspace", http.MethodGet, "/api/v1/office/workspaces/:wsId/agents", "", ""},
		{"inbox-dismiss", http.MethodPost, "/api/v1/office/inbox/dismiss", "/api/v1/office/inbox/dismiss",
			`{"kind":"agent_run_failed","item_id":"run-user-a"}`},
	}
}

// officeScopeEngine mounts the real guard in front of a recording terminal
// handler on every case's route pattern. The recorder is what proves a denial
// happened BEFORE dispatch rather than inside a handler.
func officeScopeEngine(h *officeScopeHarness, userID string, reached *string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if userID != "" {
		engine.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(authn.WithIdentity(
				c.Request.Context(), authn.Identity{UserID: userID, Role: authn.RoleMember}))
			c.Next()
		})
	}
	group := engine.Group("/api/v1/office")
	group.Use(officeWorkspaceScopeMiddleware(h.authSvc, h.taskSvc, h.officeRepo))
	for _, tc := range officeScopeCases() {
		pattern := strings.TrimPrefix(tc.pattern, "/api/v1/office")
		group.Handle(tc.method, pattern, func(c *gin.Context) {
			*reached = c.FullPath()
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	}
	return engine
}

func officeScopeRequest(t *testing.T, engine *gin.Engine, tc officeScopeCase, path string) *httptest.ResponseRecorder {
	t.Helper()
	var body *strings.Reader
	if tc.body != "" {
		body = strings.NewReader(tc.body)
	} else {
		body = strings.NewReader("")
	}
	req := httptest.NewRequest(tc.method, path, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

// officeScopeCasePath resolves the concrete URL for a case under the given
// owner's workspace (the `:wsId` case is the only one that needs the
// workspace id substituted).
func officeScopeCasePath(h *officeScopeHarness, tc officeScopeCase, owner string) string {
	if tc.path != "" {
		return tc.path
	}
	return strings.Replace(tc.pattern, ":wsId", h.workspaces[owner], 1)
}

// TestOfficeScopeDeniesForeignResourceByID is the defect this guard exists
// for: with auth enabled, user B naming user A's Office resource by id must
// be refused, and the handler must never run. Before the fix every one of
// these returned user A's data.
func TestOfficeScopeDeniesForeignResourceByID(t *testing.T) {
	h := newOfficeScopeHarness(t)

	for _, tc := range officeScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var reached string
			engine := officeScopeEngine(h, officeScopeUserB, &reached)
			rec := officeScopeRequest(t, engine, tc, officeScopeCasePath(h, tc, officeScopeUserA))

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
			}
			if !strings.Contains(rec.Body.String(), "workspace not found") {
				t.Errorf("body = %q, want the workspace-not-found denial", rec.Body.String())
			}
			if reached != "" {
				t.Errorf("handler %q ran; the guard must deny before dispatch", reached)
			}
		})
	}
}

// TestOfficeScopeAllowsOwner is the other half: the guard must not lock the
// owner out of any of the same routes.
func TestOfficeScopeAllowsOwner(t *testing.T) {
	h := newOfficeScopeHarness(t)

	for _, tc := range officeScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var reached string
			engine := officeScopeEngine(h, officeScopeUserA, &reached)
			rec := officeScopeRequest(t, engine, tc, officeScopeCasePath(h, tc, officeScopeUserA))

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d (%s), want %d", rec.Code, rec.Body.String(), http.StatusOK)
			}
			if reached != tc.pattern {
				t.Errorf("reached handler %q, want %q", reached, tc.pattern)
			}
		})
	}
}

// TestOfficeScopeAuthDisabledPassesEverythingThrough pins that the guard is
// inert without auth: single-user installs, dev, and e2e must behave exactly
// as they did before.
func TestOfficeScopeAuthDisabledPassesEverythingThrough(t *testing.T) {
	h := newOfficeScopeHarness(t)
	if h.authSvc.Mode() == auth.ModeDisabled {
		t.Fatal("harness auth service is disabled; the enabled-mode tests would be vacuous")
	}

	for _, tc := range officeScopeCases() {
		t.Run(tc.name, func(t *testing.T) {
			var reached string
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(authn.WithIdentity(
					c.Request.Context(), authn.Identity{UserID: officeScopeUserB, Role: authn.RoleMember}))
				c.Next()
			})
			group := engine.Group("/api/v1/office")
			// nil auth service is the disabled path production takes when
			// features.auth is off.
			group.Use(officeWorkspaceScopeMiddleware(nil, h.taskSvc, h.officeRepo))
			pattern := strings.TrimPrefix(tc.pattern, "/api/v1/office")
			group.Handle(tc.method, pattern, func(c *gin.Context) {
				reached = c.FullPath()
				c.JSON(http.StatusOK, gin.H{"ok": true})
			})

			rec := officeScopeRequest(t, engine, tc, officeScopeCasePath(h, tc, officeScopeUserA))
			if rec.Code != http.StatusOK || reached != tc.pattern {
				t.Errorf("status = %d, reached = %q; want 200 and %q", rec.Code, reached, tc.pattern)
			}
		})
	}
}

// TestOfficeScopeDeniesUnresolvableID pins the fail-closed rule that makes
// this guard worth having: an id that resolves to no workspace must be
// refused. AuthorizeWorkspaceAccess reads workspaceID == "" as "no scoping
// applies" and allows everything, so falling through here would leave the
// original hole open for every guessed id.
func TestOfficeScopeDeniesUnresolvableID(t *testing.T) {
	h := newOfficeScopeHarness(t)
	var reached string
	engine := officeScopeEngine(h, officeScopeUserB, &reached)

	rec := officeScopeRequest(t, engine,
		officeScopeCase{method: http.MethodGet, pattern: "/api/v1/office/agents/:id/memory"},
		"/api/v1/office/agents/does-not-exist/memory")
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if reached != "" {
		t.Errorf("handler %q ran for an unresolvable id", reached)
	}
}

// TestOfficeScopeDeniesEmptyResolvedWorkspace covers the same trap one level
// down: a row that EXISTS but carries workspace_id = "" (the schema default
// for an ordinary non-Office agent profile) must be denied, not handed to
// AuthorizeWorkspaceAccess's allow-everything branch.
func TestOfficeScopeDeniesEmptyResolvedWorkspace(t *testing.T) {
	h := newOfficeScopeHarness(t)
	seedOfficeResources(t, h.officeRepo, "", "unscoped")

	var reached string
	engine := officeScopeEngine(h, officeScopeUserB, &reached)
	rec := officeScopeRequest(t, engine,
		officeScopeCase{method: http.MethodGet, pattern: "/api/v1/office/agents/:id/memory"},
		"/api/v1/office/agents/agent-unscoped/memory")

	if rec.Code != http.StatusNotFound {
		t.Errorf("empty-workspace status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if reached != "" {
		t.Errorf("handler %q ran for an empty-workspace resource", reached)
	}
}

// TestOfficeScopeDeniesWhenOfficeRepoNil pins the nil-repository default:
// unreachable in the shipped binary, but the wrong default for a security
// check.
func TestOfficeScopeDeniesWhenOfficeRepoNil(t *testing.T) {
	h := newOfficeScopeHarness(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authn.WithIdentity(
			c.Request.Context(), authn.Identity{UserID: officeScopeUserA, Role: authn.RoleMember}))
		c.Next()
	})
	group := engine.Group("/api/v1/office")
	group.Use(officeWorkspaceScopeMiddleware(h.authSvc, h.taskSvc, nil))
	reached := false
	group.GET("/agents/:id/memory", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/office/agents/agent-user-a/memory", nil))
	if rec.Code != http.StatusNotFound || reached {
		t.Errorf("nil office repo: status = %d, reached = %v; want 404 and false", rec.Code, reached)
	}
}

// TestOfficeScopeAgentJWTCallerUnaffected drives the REAL AgentAuthMiddleware
// with a REAL minted runtime token in front of the guard: an agent caller is
// already constrained to its workspace claim and must keep reaching by-ID
// routes it could reach before, including ones in another user's workspace
// that this guard would otherwise refuse.
func TestOfficeScopeAgentJWTCallerUnaffected(t *testing.T) {
	h := newOfficeScopeHarness(t)
	agentSvc := officeagents.NewAgentService(h.officeRepo, testLogger(t), nil)
	agentSvc.SetAuth(officeagents.NewAgentAuth("test-signing-key"))
	token, err := agentSvc.MintRuntimeJWT("agent-user-a", "task-user-a", h.workspaces[officeScopeUserA], "run-user-a", "", "")
	if err != nil {
		t.Fatalf("mint runtime jwt: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		// A different user's browser identity, to prove the agent token (not
		// the identity) is what carries the request through.
		c.Request = c.Request.WithContext(authn.WithIdentity(
			c.Request.Context(), authn.Identity{UserID: officeScopeUserB, Role: authn.RoleMember}))
		c.Next()
	})
	group := engine.Group("/api/v1/office")
	group.Use(officeagents.AgentAuthMiddleware(agentSvc))
	group.Use(officeWorkspaceScopeMiddleware(h.authSvc, h.taskSvc, h.officeRepo))
	reached := false
	group.GET("/agents/:id/memory", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/agents/agent-user-a/memory", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || !reached {
		t.Errorf("agent JWT caller: status = %d (%s), reached = %v; want 200 and true",
			rec.Code, rec.Body.String(), reached)
	}
}

// TestOfficeScopeIdentitylessWebhookUnaffected pins that the two external
// webhook routes still work. They arrive with no session identity and
// authenticate by their own signature/public id, and AuthorizeWorkspaceAccess
// is a no-op for an unscoped caller — so resolving their workspace must not
// have turned them into denials.
func TestOfficeScopeIdentitylessWebhookUnaffected(t *testing.T) {
	h := newOfficeScopeHarness(t)

	for _, name := range []string{"routine-trigger-public", "channel"} {
		t.Run(name, func(t *testing.T) {
			var tc officeScopeCase
			for _, candidate := range officeScopeCases() {
				if candidate.name == name {
					tc = candidate
				}
			}
			var reached string
			engine := officeScopeEngine(h, "", &reached)
			rec := officeScopeRequest(t, engine, tc, tc.path)
			if rec.Code != http.StatusOK || reached != tc.pattern {
				t.Errorf("status = %d, reached = %q; want 200 and %q", rec.Code, reached, tc.pattern)
			}
		})
	}
}

// TestOfficeScopeDeniesRoutesWithNoRegisteredResolver drives the guard's
// fail-closed default on route shapes no Office route has TODAY — a resource
// kind nobody registered a resolver for, an id param that is only ever a
// child of a checked parent, and a route with no id at all. This is what a
// newly added route looks like before its author wires it up, and the whole
// point of the backstop is that such a route is denied rather than exempt.
//
// TestOfficeRouteScopeCompleteness turns each of these into a build failure;
// this pins the runtime behaviour if one ever ships anyway.
func TestOfficeScopeDeniesRoutesWithNoRegisteredResolver(t *testing.T) {
	h := newOfficeScopeHarness(t)
	routes := map[string]string{
		"unregistered resource kind":        "/widgets/:widgetId",
		"sub-resource param with no parent": "/documents/:key",
		"no id param at all":                "/newly-added-report",
		// The subtle one: a checked parent does NOT license an unregistered
		// sibling id. Authorizing only the agent here would let a caller pair
		// their OWN agent with another workspace's widget — the same "invent
		// a fifth name for the resource" escape hatch the WS gateway's
		// dispatch backstop documents.
		"unregistered id beside a checked parent": "/agents/:id/widgets/:widgetId",
	}

	for name, pattern := range routes {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				c.Request = c.Request.WithContext(authn.WithIdentity(
					c.Request.Context(), authn.Identity{UserID: officeScopeUserA, Role: authn.RoleMember}))
				c.Next()
			})
			group := engine.Group("/api/v1/office")
			group.Use(officeWorkspaceScopeMiddleware(h.authSvc, h.taskSvc, h.officeRepo))
			reached := false
			group.GET(pattern, func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, httptest.NewRequest(
				http.MethodGet, "/api/v1/office"+strings.NewReplacer(
					":widgetId", "w1", ":key", "k1", ":id", "agent-"+officeScopeUserA).Replace(pattern), nil))

			if rec.Code != http.StatusNotFound || reached {
				t.Errorf("%s: status = %d, reached = %v; want 404 and false", pattern, rec.Code, reached)
			}
		})
	}
}
