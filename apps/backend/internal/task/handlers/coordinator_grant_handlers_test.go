package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

func TestParseCapabilitiesAllowsHostExecutionLease(t *testing.T) {
	t.Parallel()

	got := parseCapabilities("orchestrate, execute, inspect, execute")
	want := []string{"orchestrate", "execute", "inspect"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseCapabilities() = %v, want %v", got, want)
	}
}

func TestCreateCoordinatorGrantBindsTheTaskActivePrincipal(t *testing.T) {
	_, repo, svc := newRepositoryHTTPTestRouterWithService(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "coordinator", WorkspaceID: "ws-1", Title: "Coordinator"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator", BackingTaskID: "coordinator", BackingSessionID: "session-1",
	}); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
		c.Next()
	})
	RegisterCoordinatorGrantRoutes(router, repo, svc, log)
	body, err := json.Marshal(createGrantRequest{CoordinatorTaskID: "coordinator", ScopeKind: "workspace", Capabilities: "inspect"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/coordinator-grants", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusCreated, response.Body.String())
	}
	grants, err := repo.ListActiveWorkspaceAgentPrincipalGrants(ctx, "principal-1", "ws-1")
	if err != nil || len(grants) != 1 || grants[0].PrincipalID != "principal-1" {
		t.Fatalf("principal grants = %#v, err = %v; want one principal-bound grant", grants, err)
	}
	created, err := repo.ListCoordinatorGrants(ctx, "ws-1", "coordinator", false)
	if err != nil || len(created) != 1 || created[0].GrantedByUserID != "admin" {
		t.Fatalf("created grants = %#v, err = %v; want audit actor admin", created, err)
	}
}

// Reviewer-requested contract coverage: grant and audit routes must never be
// reachable by an anonymous or non-admin caller.
func TestCoordinatorGrantRoutesRequireAdmin(t *testing.T) {
	_, repo, svc := newRepositoryHTTPTestRouterWithService(t)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	router := gin.New()
	RegisterCoordinatorGrantRoutes(router, repo, svc, log)

	for name, identity := range map[string]*authn.Identity{
		"anonymous": nil,
		"member":    {UserID: "member", Role: authn.RoleMember},
	} {
		t.Run(name, func(t *testing.T) {
			handler := router
			if identity != nil {
				handler = gin.New()
				handler.Use(func(c *gin.Context) {
					authn.SetOnGin(c, *identity)
					c.Next()
				})
				RegisterCoordinatorGrantRoutes(handler, repo, svc, log)
			}
			want := http.StatusUnauthorized
			if identity != nil {
				want = http.StatusForbidden
			}
			for _, route := range []struct {
				method string
				path   string
			}{
				{method: http.MethodGet, path: "/api/v1/workspaces/ws-1/coordinator-grants"},
				{method: http.MethodGet, path: "/api/v1/tasks/task-1/coordinator-grants"},
				{method: http.MethodGet, path: "/api/v1/workspaces/ws-1/coordinator-audit"},
				{method: http.MethodPost, path: "/api/v1/workspaces/ws-1/coordinator-grants"},
				{method: http.MethodDelete, path: "/api/v1/coordinator-grants/grant-1"},
			} {
				t.Run(route.method+" "+route.path, func(t *testing.T) {
					request := httptest.NewRequest(route.method, route.path, nil)
					response := httptest.NewRecorder()

					handler.ServeHTTP(response, request)

					if response.Code != want {
						t.Fatalf("status = %d, want %d; body = %s", response.Code, want, response.Body.String())
					}
				})
			}
		})
	}
}
