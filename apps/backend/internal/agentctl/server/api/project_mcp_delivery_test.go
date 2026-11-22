package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

func projectDeliveryAPIEnv(t *testing.T) (*Server, *config.InstanceConfig, *types.ProjectMCPDelivery) {
	t.Helper()
	workspace := t.TempDir()
	manifest := &p.BuiltinManifest{
		Version: 1, CatalogVersion: 1, WorkspaceID: "workspace-1", TaskID: "task-1",
		Surface:  p.SurfaceKanbanTask,
		Packages: []p.PackageID{p.PackageCore, p.PackageDiagnostics},
		Origins:  map[p.PackageID]string{p.PackageCore: "required", p.PackageDiagnostics: "task"},
	}
	profile := p.Normalize(p.Context{Surface: p.SurfaceKanbanTask, BuiltinPackages: manifest})
	digest, err := p.BuiltinBindingDigest(profile)
	require.NoError(t, err)
	cfg := &config.InstanceConfig{
		InstanceID: "instance-1", Port: 43210, MCPHost: "localhost", WorkDir: workspace,
		TaskID: "task-1", McpProfile: &profile, AuthToken: "test-control",
		AgentType: types.ProjectMCPDeliveryAgentPi,
	}
	log := newTestLogger()
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	delivery := &types.ProjectMCPDelivery{
		Version: types.ProjectMCPDeliveryVersion, Strategy: types.ProjectMCPDeliveryStrategyPi,
		Workspace: filepath.Clean(workspace), TaskID: "task-1", InstanceID: "instance-1",
		BindingDigest: digest, Packages: []string{"core", "diagnostics"},
	}
	return server, cfg, delivery
}

func postProjectDeliveryConfigure(t *testing.T, server *Server, authenticated bool, delivery *types.ProjectMCPDelivery) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{"command": "pi-acp", "agent_args": []string{"pi-acp"}}
	if delivery != nil {
		payload["project_mcp_delivery"] = delivery
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authenticated {
		req.Header.Set("Authorization", "Bearer test-control")
	}
	response := httptest.NewRecorder()
	server.Router().ServeHTTP(response, req)
	return response
}

// @covers AC-AGENTS-BUILTIN-MCP-003.5, AC-AGENTS-BUILTIN-MCP-003.6
func TestHandleAgentConfigureProjectMCPDelivery(t *testing.T) {
	_, unauthenticatedCfg, delivery := projectDeliveryAPIEnv(t)
	unauthenticatedCfg.AuthToken = ""
	unauthenticated := NewServer(unauthenticatedCfg, process.NewManager(unauthenticatedCfg, newTestLogger()), nil, nil, newTestLogger())
	response := postProjectDeliveryConfigure(t, unauthenticated, false, delivery)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Nil(t, unauthenticatedCfg.ProjectMCPDelivery, "an unauthenticated caller cannot supply delivery evidence")

	server, cfg, delivery := projectDeliveryAPIEnv(t)
	response = postProjectDeliveryConfigure(t, server, true, delivery)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, delivery, cfg.ProjectMCPDelivery)

	invalid := delivery.Clone()
	invalid.InstanceID = "instance-2"
	response = postProjectDeliveryConfigure(t, server, true, invalid)
	require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
	require.Equal(t, delivery, cfg.ProjectMCPDelivery, "rejected evidence must not replace accepted state")

	response = postProjectDeliveryConfigure(t, server, true, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Nil(t, cfg.ProjectMCPDelivery, "a configuration without evidence clears the prior record")
}
