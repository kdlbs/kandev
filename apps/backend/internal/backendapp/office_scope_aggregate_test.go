package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	officeagents "github.com/kandev/kandev/internal/office/agents"
)

// driveOfficeAggregate mounts the Office scope guard in front of a recording
// handler for GET /workspaces/aggregate and drives one request. identity is
// injected on the request context when non-nil; agentSvc non-nil mounts the
// agent-auth middleware so an Authorization header can mint an agent caller.
func driveOfficeAggregate(
	t *testing.T,
	h *officeScopeHarness,
	authSvc *auth.Service,
	identity *authn.Identity,
	agentSvc *officeagents.AgentService,
	token string,
) (int, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if identity != nil {
		engine.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(), *identity))
			c.Next()
		})
	}
	group := engine.Group("/api/v1/office")
	if agentSvc != nil {
		group.Use(officeagents.AgentAuthMiddleware(agentSvc))
	}
	group.Use(officeWorkspaceScopeMiddleware(authSvc, h.taskSvc, h.officeRepo))
	reached := false
	group.GET("/workspaces/aggregate", func(c *gin.Context) { reached = true; c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/aggregate", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec.Code, reached
}

// TestOfficeScopeAggregateAllowsRealIdentity pins that a real browser identity
// reaches the multi-workspace aggregate under enforced auth. The workspace
// visibility itself is decided by the handler's identity-scoped ListWorkspaces,
// not by this guard.
func TestOfficeScopeAggregateAllowsRealIdentity(t *testing.T) {
	h := newOfficeScopeHarness(t)
	code, reached := driveOfficeAggregate(t, h, h.authSvc,
		&authn.Identity{UserID: officeScopeUserA, Role: authn.RoleMember}, nil, "")
	if code != http.StatusOK || !reached {
		t.Fatalf("status = %d, reached = %v; want 200 and true", code, reached)
	}
}

// TestOfficeScopeAggregateDeniesSyntheticIdentity pins that a synthetic
// single-user identity (auth enabled, no real account) cannot read the
// aggregate: the route reads per-user data and must not fall back to an
// unscoped list under enforced auth.
func TestOfficeScopeAggregateDeniesSyntheticIdentity(t *testing.T) {
	h := newOfficeScopeHarness(t)
	code, reached := driveOfficeAggregate(t, h, h.authSvc,
		&authn.Identity{UserID: officeScopeUserA, Role: authn.RoleAdmin, Synthetic: true}, nil, "")
	if code != http.StatusNotFound || reached {
		t.Fatalf("status = %d, reached = %v; want 404 and false", code, reached)
	}
}

// TestOfficeScopeAggregateDeniesNoIdentity pins that a request with no
// identity at all is refused under enforced auth.
func TestOfficeScopeAggregateDeniesNoIdentity(t *testing.T) {
	h := newOfficeScopeHarness(t)
	code, reached := driveOfficeAggregate(t, h, h.authSvc, nil, nil, "")
	if code != http.StatusNotFound || reached {
		t.Fatalf("status = %d, reached = %v; want 404 and false", code, reached)
	}
}

// TestOfficeScopeAggregateDeniesAgentCaller pins that an agent JWT cannot
// reach the aggregate: an agent token is confined to one workspace and must
// not enumerate the owner's other workspaces.
func TestOfficeScopeAggregateDeniesAgentCaller(t *testing.T) {
	h := newOfficeScopeHarness(t)
	agentSvc := officeagents.NewAgentService(h.officeRepo, testLogger(t), nil)
	agentSvc.SetAuth(officeagents.NewAgentAuth("test-signing-key"))
	token, err := agentSvc.MintRuntimeJWT("agent-user-a", "task-user-a", h.workspaces[officeScopeUserA], "run-user-a", "", "")
	if err != nil {
		t.Fatalf("mint runtime jwt: %v", err)
	}
	code, reached := driveOfficeAggregate(t, h, h.authSvc, nil, agentSvc, token)
	if code != http.StatusNotFound || reached {
		t.Fatalf("status = %d, reached = %v; want 404 and false", code, reached)
	}
}

// TestOfficeScopeAggregateDeniesAgentCallerWhenAuthDisabled pins that the
// multi-workspace route still rejects agent tokens when browser auth is off.
func TestOfficeScopeAggregateDeniesAgentCallerWhenAuthDisabled(t *testing.T) {
	h := newOfficeScopeHarness(t)
	agentSvc := officeagents.NewAgentService(h.officeRepo, testLogger(t), nil)
	agentSvc.SetAuth(officeagents.NewAgentAuth("test-signing-key"))
	token, err := agentSvc.MintRuntimeJWT("agent-user-a", "task-user-a", h.workspaces[officeScopeUserA], "run-user-a", "", "")
	if err != nil {
		t.Fatalf("mint runtime jwt: %v", err)
	}

	code, reached := driveOfficeAggregate(t, h, nil, nil, agentSvc, token)
	if code != http.StatusNotFound || reached {
		t.Fatalf("status = %d, reached = %v; want 404 and false", code, reached)
	}
}

// TestOfficeScopeAggregateAuthDisabledPassthrough pins that the aggregate is
// reachable when auth is off, matching every other Office route's disabled
// behavior.
func TestOfficeScopeAggregateAuthDisabledPassthrough(t *testing.T) {
	h := newOfficeScopeHarness(t)
	code, reached := driveOfficeAggregate(t, h, nil, nil, nil, "")
	if code != http.StatusOK || !reached {
		t.Fatalf("status = %d, reached = %v; want 200 and true", code, reached)
	}
}
