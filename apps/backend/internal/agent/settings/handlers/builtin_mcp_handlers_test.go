package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

type builtinHandlerStore struct{ writes int }

func (r *builtinHandlerStore) GetBuiltinPolicySnapshot(_ context.Context, workspace, task string) (*mcpconfig.BuiltinPolicySnapshot, error) {
	snapshot := &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy(workspace)}
	if task != "" {
		snapshot.Preferences = &mcpconfig.BuiltinTaskPreferences{Version: 1, WorkspaceID: workspace, TaskID: task, Packages: mcpconfig.BuiltinPackageChoices{}}
	}
	return snapshot, nil
}
func (r *builtinHandlerStore) PutBuiltinWorkspacePolicy(_ context.Context, policy *mcpconfig.BuiltinWorkspacePolicy, revision int64) (*mcpconfig.BuiltinWorkspacePolicy, error) {
	if revision != 0 {
		return nil, &mcpconfig.BuiltinPolicyConflictError{Current: &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy(policy.WorkspaceID)}}
	}
	r.writes++
	return policy, nil
}
func (r *builtinHandlerStore) PutBuiltinTaskPreferences(_ context.Context, preferences *mcpconfig.BuiltinTaskPreferences, _, _ int64, _ p.Surface) (*mcpconfig.BuiltinPolicySnapshot, error) {
	r.writes++
	return &mcpconfig.BuiltinPolicySnapshot{Policy: mcpconfig.DefaultBuiltinWorkspacePolicy(preferences.WorkspaceID), Preferences: preferences}, nil
}

func TestBuiltinManagementHTTPValidatesCompleteVersionedWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &builtinHandlerStore{}
	service := mcpconfig.NewBuiltinPolicyService(repo, func(_ context.Context, workspace string, _ authz.Scope) error {
		if workspace != "workspace" {
			return mcpconfig.ErrMCPWorkspaceAccess
		}
		return nil
	}, func(_ context.Context, task string) (string, p.Surface, error) {
		return "workspace", p.SurfaceKanbanTask, nil
	})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: "owner"}))
		c.Next()
	})
	RegisterBuiltinMCPRoutes(router, service, nil, nil)
	request := func(method, path, body string, want int) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		require.Equal(t, want, response.Code, response.Body.String())
	}
	request(http.MethodGet, "/api/v1/workspaces/workspace/mcp-built-in", "", 200)
	request(http.MethodGet, "/api/v1/tasks/task/mcp-built-in", "", 200)
	request(http.MethodGet, "/api/v1/workspaces/foreign/mcp-built-in", "", 403)
	valid := `{"version":1,"expected_revision":0,"expected_workspace_policy_revision":0,"packages":{"configuration":"on"}}`
	request(http.MethodPut, "/api/v1/tasks/task/mcp-built-in/preferences", valid, 200)
	require.Equal(t, 1, repo.writes)
	for _, invalid := range []string{
		`{}`, `{"version":1,"expected_revision":0,"packages":{}}`,
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"on"`, `"on","configuration":"off"`, 1),
		strings.Replace(valid, `"configuration"`, `"core"`, 1),
		strings.Replace(valid, `"configuration":"on"`, `"show_walkthrough_kandev":"on"`, 1),
		strings.Replace(valid, `"packages":`, `"unknown":true,"packages":`, 1),
		strings.Replace(valid, `{"configuration":"on"}`, `null`, 1), valid + `{}`,
	} {
		request(http.MethodPut, "/api/v1/tasks/task/mcp-built-in/preferences", invalid, 400)
	}
	require.Equal(t, 1, repo.writes)
	policy := `{"version":1,"expected_revision":2,"packages":{"configuration":"available","rich-output":"default","walkthroughs":"default","diagnostics":"default"}}`
	request(http.MethodPut, "/api/v1/workspaces/workspace/mcp-built-in/policy", policy, 409)
	require.Equal(t, 1, repo.writes)
}
