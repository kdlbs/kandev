package api

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// Every session request family hands the provider built-in entries that
// carry the instance credential, and leaves third-party headers alone.
func TestBuiltinSessionRequestsCarryInstanceCredential(t *testing.T) {
	want := config.BuiltinMCPHeaders(testMCPCredential)
	for _, action := range []string{"new", "load", "resume", "reset"} {
		t.Run(action, func(t *testing.T) {
			s, capture := newBuiltinSessionServer(t)
			s.cfg.BuiltinMCPCredential = testMCPCredential
			request, err := ws.NewRequest("request", "agent.session."+action, map[string]any{
				"session_id": "native-session",
				"mcp_servers": []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp",
					Headers: map[string]string{"X-Key": "user"}}},
			})
			require.NoError(t, err)
			response := s.handleAgentStreamRequest(context.Background(), request)
			require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
			servers := append([]types.McpServer(nil), capture.newSessionServers...)
			servers = append(servers, capture.loadSessionServers...)
			require.NotEmpty(t, servers)
			for _, server := range servers {
				if server.Name == "third-party" {
					require.Equal(t, map[string]string{"X-Key": "user"}, server.Headers)
					continue
				}
				require.Equal(t, want, server.Headers, server.Name)
			}
		})
	}
}

// Legacy (non-package) instances inject the core kandev server the same way.
func TestLegacySessionInjectionCarriesInstanceCredential(t *testing.T) {
	s := newTestServerWithMCP(t)
	s.cfg.BuiltinMCPCredential = testMCPCredential
	servers := s.injectKandevMcpServers(nil)
	require.Len(t, servers, 2)
	for _, server := range servers {
		require.Equal(t, config.BuiltinMCPHeaders(testMCPCredential), server.Headers)
	}
}
