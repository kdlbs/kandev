package api

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/sandbox"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestSandboxBuiltinPackagesReachEverySessionRequestFamily(t *testing.T) {
	for _, action := range []string{"new", "load", "resume", "reset"} {
		for _, allowed := range []bool{false, true} {
			t.Run(action+"/"+map[bool]string{false: "denied", true: "allowed"}[allowed], func(t *testing.T) {
				s, capture := newBuiltinSessionServer(t)
				launch := &sandbox.Launch{}
				if allowed {
					launch.MCPURL = "http://127.0.0.1:45679/" + strings.Repeat("a", 64) + "/mcp"
					launch.MCPPaths = []string{"/mcp", "/mcp/packages/configuration"}
				}
				s.cfg.Sandbox = launch
				request, err := ws.NewRequest("request", "agent.session."+action, map[string]any{"session_id": "native-session", "mcp_servers": []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp"}}})
				require.NoError(t, err)
				response := s.handleAgentStreamRequest(t.Context(), request)
				require.Equal(t, ws.MessageTypeResponse, response.Type, string(response.Payload))
				servers := append([]types.McpServer(nil), capture.newSessionServers...)
				servers = append(servers, capture.loadSessionServers...)
				expected := []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp"}}
				if allowed {
					require.Len(t, servers, 3)
					require.Equal(t, launch.MCPURL, servers[0].URL)
					require.Equal(t, launch.MCPURL+"/packages/configuration", servers[1].URL)
					require.Equal(t, "kandev-configuration", servers[1].Name)
					require.Equal(t, expected[0], servers[2])
				} else {
					require.Equal(t, expected, servers)
				}
			})
		}
	}
}
