package agents

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
)

// newMemoryRouter returns a gin engine with the agent auth middleware
// installed, so requests carrying a Bearer JWT route through
// checkAgentMemoryAccess. The middleware is a no-op for unauthenticated
// requests (matches the production wiring).
func newMemoryRouter(svc *AgentService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AgentAuthMiddleware(svc))
	group := r.Group("/api/v1")
	RegisterRoutes(group, svc, logger.Default())
	return r
}

// newMemoryReq builds a GET /agents/:id/memory request with the given
// agentID and an optional Bearer token (empty token = no Authorization
// header, the path UI callers take).
func newMemoryReq(agentID, token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/"+agentID+"/memory", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// seedAgentForMemoryTest persists an AgentInstance row and returns it.
// The role is supplied by the caller so admin / worker / cross-agent
// scenarios share the same scaffolding.
func seedAgentForMemoryTest(t *testing.T, svc *AgentService, id string, role models.AgentRole) *models.AgentInstance {
	t.Helper()
	a := &models.AgentInstance{
		ID:          id,
		WorkspaceID: "ws-1",
		Name:        id,
		Role:        role,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(context.Background(), a); err != nil {
		t.Fatalf("create agent %q: %v", id, err)
	}
	return a
}

// TestMemoryHandlers_AgentCanAccessOwnMemory pins the happy-path branch
// of checkAgentMemoryAccess: a JWT-authenticated agent reading its own
// /memory endpoint must succeed.
func TestMemoryHandlers_AgentCanAccessOwnMemory(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetAuth(NewAgentAuth("test-key"))
	agent := seedAgentForMemoryTest(t, svc, "agent-1", models.AgentRoleWorker)

	token, err := svc.auth.MintAgentJWT(agent.ID, "task-1", agent.WorkspaceID, "sess-1")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	newMemoryRouter(svc).ServeHTTP(rec, newMemoryReq(agent.ID, token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestMemoryHandlers_AgentForbiddenFromOtherAgentMemory pins the deny
// branch: an agent JWT reading another agent's memory must get 403.
// Without checkAgentMemoryAccess, any compromised worker could harvest
// memory from every agent in the workspace.
func TestMemoryHandlers_AgentForbiddenFromOtherAgentMemory(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetAuth(NewAgentAuth("test-key"))
	caller := seedAgentForMemoryTest(t, svc, "agent-1", models.AgentRoleWorker)
	other := seedAgentForMemoryTest(t, svc, "agent-2", models.AgentRoleWorker)

	token, err := svc.auth.MintAgentJWT(caller.ID, "task-1", caller.WorkspaceID, "sess-1")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	newMemoryRouter(svc).ServeHTTP(rec, newMemoryReq(other.ID, token))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}

// TestMemoryHandlers_AdminCanAccessAnyAgentMemory documents the CEO
// escape hatch: the admin role is exempt from the same-agent guard so
// the CEO can audit any agent's memory.
func TestMemoryHandlers_AdminCanAccessAnyAgentMemory(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetAuth(NewAgentAuth("test-key"))
	ceo := seedAgentForMemoryTest(t, svc, "ceo-1", models.AgentRoleCEO)
	other := seedAgentForMemoryTest(t, svc, "agent-2", models.AgentRoleWorker)

	token, err := svc.auth.MintAgentJWT(ceo.ID, "task-1", ceo.WorkspaceID, "sess-1")
	if err != nil {
		t.Fatalf("mint jwt: %v", err)
	}

	rec := httptest.NewRecorder()
	newMemoryRouter(svc).ServeHTTP(rec, newMemoryReq(other.ID, token))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestMemoryHandlers_UIRequestPassesThrough confirms unauthenticated UI
// callers (no Authorization header) skip the agent-memory guard. The
// existing UI memory tab in the office app relies on this — break it
// and the dashboard can no longer view memory.
func TestMemoryHandlers_UIRequestPassesThrough(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetAuth(NewAgentAuth("test-key"))
	agent := seedAgentForMemoryTest(t, svc, "agent-1", models.AgentRoleWorker)

	rec := httptest.NewRecorder()
	newMemoryRouter(svc).ServeHTTP(rec, newMemoryReq(agent.ID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}
