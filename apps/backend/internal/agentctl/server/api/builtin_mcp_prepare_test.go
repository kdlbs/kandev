package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/stretchr/testify/require"
)

func TestBuiltinWorkspacePreparationBindsBeforeAgentConfiguration(t *testing.T) {
	cfg, err := (&config.Config{}).NewInstanceConfig(12345, &config.InstanceOverrides{WorkDir: t.TempDir(), TaskID: "task", SessionID: "session"})
	require.NoError(t, err)
	cfg.AuthToken = "control-secret"
	log := newTestLogger()
	manager := process.NewManager(cfg, log)
	backend := mcpserver.NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	mcp := mcpserver.NewWithProfile(backend, "session", "task", cfg.Port, log, "", false, p.Legacy("", false, nil))
	t.Cleanup(func() { require.NoError(t, mcp.Close(context.Background())) })
	server := NewServer(cfg, manager, mcp, nil, log)
	profile := p.New(p.SurfaceKanbanTask, nil, nil)
	profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageConfiguration}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageConfiguration: "task"}}
	put := func(token string) *httptest.ResponseRecorder {
		body, err := json.Marshal(profile)
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPut, "/api/v1/mcp/builtin-binding", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		return response
	}
	require.Equal(t, http.StatusUnauthorized, put("").Code)
	response := put("control-secret")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, profile.BuiltinPackages, mcp.Profile().BuiltinPackages)
	require.Contains(t, mcp.BuiltinCatalog()[p.PackageConfiguration], "create_workflow_kandev")
	require.Equal(t, http.StatusOK, put("control-secret").Code, "retry is idempotent")
	request := httptest.NewRequest(http.MethodPost, "/mcp/packages/configuration", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response = httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	for _, token := range []string{"", "control-secret"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/builtin-binding", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if token == "" {
			require.Equal(t, http.StatusUnauthorized, response.Code)
			continue
		}
		require.Equal(t, http.StatusOK, response.Code)
		var actual p.Context
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &actual))
		require.Equal(t, profile.BuiltinPackages, actual.BuiltinPackages)
	}
	profile.BuiltinPackages.Packages = []p.PackageID{p.PackageCore}
	delete(profile.BuiltinPackages.Origins, p.PackageConfiguration)
	require.Equal(t, http.StatusConflict, put("control-secret").Code, "binding cannot change after preparation")
}
