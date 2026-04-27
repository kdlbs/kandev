package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/controller"
	"github.com/kandev/kandev/internal/orchestrate/handlers"
	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func setupRouter(t *testing.T, svc *service.Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ctrl := controller.NewController(svc)
	handlers.RegisterRoutes(r, ctrl, testLogger())
	return r
}

func TestAuthMiddleware_NoJWT(t *testing.T) {
	svc := newTestService(t)
	r := setupRouter(t, svc)

	// Request without Authorization header should pass through (admin).
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/orchestrate/meta", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for no-JWT request, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ValidJWT(t *testing.T) {
	svc := newTestService(t)
	auth := service.NewAgentAuth("test-key")
	svc.SetAgentAuth(auth)

	// Create an agent so the middleware can find it.
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "test-agent",
		Role:        models.AgentRoleCEO,
	}
	if err := svc.CreateAgentInstance(t.Context(), agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	token, err := auth.MintAgentJWT(agent.ID, "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint JWT: %v", err)
	}

	r := setupRouter(t, svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/orchestrate/agents/"+agent.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid JWT, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddleware_ExpiredJWT(t *testing.T) {
	svc := newTestService(t)
	auth := service.NewAgentAuth("test-key")
	svc.SetAgentAuth(auth)

	// Use a very short expiry to create an expired token.
	token, err := auth.MintExpiredJWT("agent-1", "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint JWT: %v", err)
	}

	r := setupRouter(t, svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/orchestrate/meta", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired JWT, got %d", w.Code)
	}
}

func TestCEOCanCreateAgent(t *testing.T) {
	svc := newTestService(t)
	auth := service.NewAgentAuth("test-key")
	svc.SetAgentAuth(auth)

	ceo := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "ceo-agent",
		Role:        models.AgentRoleCEO,
	}
	if err := svc.CreateAgentInstance(t.Context(), ceo); err != nil {
		t.Fatalf("create CEO: %v", err)
	}

	token, err := auth.MintAgentJWT(ceo.ID, "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint JWT: %v", err)
	}

	r := setupRouter(t, svc)
	body := `{"name":"new-worker","role":"worker"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/orchestrate/workspaces/ws-1/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("CEO should be able to create agents, got %d: %s", w.Code, w.Body.String())
	}
}

func TestWorkerCannotCreateAgent(t *testing.T) {
	svc := newTestService(t)
	auth := service.NewAgentAuth("test-key")
	svc.SetAgentAuth(auth)

	worker := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "worker-agent",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(t.Context(), worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}

	token, err := auth.MintAgentJWT(worker.ID, "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint JWT: %v", err)
	}

	r := setupRouter(t, svc)
	body := `{"name":"new-worker-2","role":"worker"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/orchestrate/workspaces/ws-1/agents", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Worker should not be able to create agents, got %d: %s", w.Code, w.Body.String())
	}
}

func TestMetaIncludesPermissions(t *testing.T) {
	svc := newTestService(t)
	r := setupRouter(t, svc)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/orchestrate/meta", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Permissions        []struct{ Key string } `json:"permissions"`
		PermissionDefaults map[string]interface{} `json:"permissionDefaults"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Permissions) != 6 {
		t.Errorf("expected 6 permissions, got %d", len(resp.Permissions))
	}
	if _, ok := resp.PermissionDefaults["ceo"]; !ok {
		t.Error("expected permissionDefaults to include ceo")
	}
	if _, ok := resp.PermissionDefaults["worker"]; !ok {
		t.Error("expected permissionDefaults to include worker")
	}
}
