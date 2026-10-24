package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPackagesReachEverySessionRequestFamily(t *testing.T) {
	for _, action := range []string{"new", "load", "resume", "reset"} {
		t.Run(action, func(t *testing.T) {
			s, capture := newBuiltinSessionServer(t)
			request, err := ws.NewRequest("request", "agent.session."+action, map[string]any{
				"session_id": "native-session", "mcp_servers": []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp"}},
			})
			require.NoError(t, err)
			response := s.handleAgentStreamRequest(context.Background(), request)
			require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
			servers := append([]types.McpServer(nil), capture.newSessionServers...)
			servers = append(servers, capture.loadSessionServers...)
			require.Len(t, servers, 5)
			require.Equal(t, "kandev-configuration", servers[2].Name)
			require.Equal(t, "http://127.0.0.1:12345/mcp/packages/configuration", servers[2].URL)
			require.Equal(t, "third-party", servers[4].Name)
		})
	}
}

func TestBuiltinSessionRequestRejectsReservedCollisions(t *testing.T) {
	s, capture := newBuiltinSessionServer(t)
	request, err := ws.NewRequest("request", "agent.session.new", map[string]any{
		"mcp_servers": []types.McpServer{{Name: "kandev-configuration", Type: "http", URL: "https://example.test/mcp"}},
	})
	require.NoError(t, err)
	response := s.handleAgentStreamRequest(context.Background(), request)
	require.Equal(t, ws.MessageTypeError, response.Type)
	require.Empty(t, capture.newSessionServers)
}

func TestBuiltinSessionCannotChangeItsBoundSurface(t *testing.T) {
	s, _ := newBuiltinSessionServer(t)
	require.Equal(t, http.StatusOK, setMcpMode(t, s, "task-title-pending").Code)
	require.True(t, s.mcpServer.Profile().HasCapability(p.CapabilityTaskTitle))
	response := setMcpMode(t, s, "config")
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Equal(t, p.SurfaceKanbanTask, s.mcpServer.Profile().Surface)
	require.NotContains(t, s.mcpServer.BuiltinCatalog()[p.PackageCore], "transfer_task_kandev")
}

func TestBuiltinPackageTransportRemainsReachableWithoutControlCredential(t *testing.T) {
	s, _ := newBuiltinSessionServer(t, "control-secret")
	request := httptest.NewRequest(http.MethodPost, "/mcp/packages/configuration", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response := httptest.NewRecorder()
	s.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	protected := httptest.NewRecorder()
	s.router.ServeHTTP(protected, httptest.NewRequest(http.MethodGet, "/api/v1/status", nil))
	require.Equal(t, http.StatusUnauthorized, protected.Code)
}

func newBuiltinSessionServer(t *testing.T, tokens ...string) (*Server, *resolutionCaptureAdapter) {
	t.Helper()
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface,
		Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	cfg, err := (&config.Config{}).NewInstanceConfig(12345, &config.InstanceOverrides{WorkDir: t.TempDir(), TaskID: "task", SessionID: "session", McpProfile: &profile})
	require.NoError(t, err)
	if len(tokens) > 0 {
		cfg.AuthToken = tokens[0]
	}
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcp := mcpserver.NewWithProfile(backend, "session", "task", cfg.Port, log, "", false, profile)
	t.Cleanup(func() { require.NoError(t, mcp.Close(context.Background())) })
	s := NewServer(cfg, manager, mcp, nil, log)
	capture := &resolutionCaptureAdapter{}
	manager.SetAdapterForTest(capture)
	return s, capture
}

func TestBuiltinSessionEvidenceUsesInjectedProvenance(t *testing.T) {
	s, _ := newBuiltinSessionServer(t)
	servers, err := s.sessionMCPServers(nil)
	require.NoError(t, err)
	for _, server := range servers {
		if server.Name != "kandev-configuration" {
			continue
		}
		evidence := mcpAttachmentEvidence("attempt", server, streams.MCPAttachmentEvidenceConfigured, "", "")
		require.Equal(t, streams.MCPServerSourceKandev, evidence.Source)
	}
	lookalike := types.McpServer{Name: "kandev-configuration", URL: "https://external.example/mcp", Type: "http"}
	evidence := mcpAttachmentEvidence("attempt", lookalike, streams.MCPAttachmentEvidenceConfigured, "", "")
	require.Equal(t, streams.MCPServerSourceProfile, evidence.Source)
}
