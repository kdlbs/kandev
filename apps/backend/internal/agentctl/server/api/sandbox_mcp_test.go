package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	"github.com/stretchr/testify/require"
)

func TestSandboxMCPControlRouteRequiresLiveOwnerAndAuthentication(t *testing.T) {
	for _, token := range []string{"", "control"} {
		log := newTestLogger()
		cfg := &config.InstanceConfig{AuthToken: token, WorkDir: t.TempDir()}
		backend := mcpserver.NewChannelBackendClient(log)
		t.Cleanup(backend.Close)
		mcp := mcpserver.New(backend, "session", "task", 0, log, "", false, mcpserver.ModeTask)
		server := NewServer(cfg, process.NewManager(cfg, log), mcp, nil, log)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/agent/sandbox/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		server.Router().ServeHTTP(response, request)
		require.Equal(t, http.StatusForbidden, response.Code)
	}
}
