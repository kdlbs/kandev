package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func installContextAPIEnv(t *testing.T) (*Server, *config.InstanceConfig) {
	t.Helper()
	cfg := &config.InstanceConfig{
		InstanceID: "instance-1", Port: 43211, MCPHost: "localhost",
		WorkDir: t.TempDir(), TaskID: "task-1", AuthToken: "test-control",
	}
	log := newTestLogger()
	return NewServer(cfg, process.NewManager(cfg, log), nil, nil, log), cfg
}

func postInstallContextConfigure(
	t *testing.T,
	server *Server,
	authenticated bool,
	installContext *types.ManagedRuntimeInstallContext,
) *httptest.ResponseRecorder {
	t.Helper()
	payload := map[string]any{"command": "npx", "agent_args": []string{"npx", "--yes"}}
	if installContext != nil {
		payload["install_context"] = installContext
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

func TestHandleAgentConfigureInstallContext(t *testing.T) {
	valid := &types.ManagedRuntimeInstallContext{
		InstallerEnvDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		OwnerScope:         "task:owner-a",
	}

	_, unauthenticatedCfg := installContextAPIEnv(t)
	unauthenticatedCfg.AuthToken = ""
	unauthenticated := NewServer(unauthenticatedCfg, process.NewManager(unauthenticatedCfg, newTestLogger()), nil, nil, newTestLogger())
	response := postInstallContextConfigure(t, unauthenticated, false, valid)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Nil(t, unauthenticatedCfg.ManagedRuntimeInstallContext)

	server, cfg := installContextAPIEnv(t)
	response = postInstallContextConfigure(t, server, true, valid)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, valid, cfg.ManagedRuntimeInstallContext)

	invalid := *valid
	invalid.InstallerEnvDigest = "not-a-digest"
	response = postInstallContextConfigure(t, server, true, &invalid)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	require.Equal(t, valid, cfg.ManagedRuntimeInstallContext, "invalid provenance replaced accepted configuration")

	response = postInstallContextConfigure(t, server, true, nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Nil(t, cfg.ManagedRuntimeInstallContext, "configuration without provenance retained a stale context")
}
