package api

import (
	"context"
	"os"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type assistantSessionCapture struct {
	mcpCaptureAdapter
	resetServers []types.McpServer
}

func (a *assistantSessionCapture) ResetSession(_ context.Context, servers []types.McpServer) (string, error) {
	a.resetServers = servers
	return "reset-session", nil
}

// @covers AC-ORCHESTRATION-ASSISTANT-004.1
func TestAssistantSessionLifecycleAttachesOnlyManagedBroker(t *testing.T) {
	for _, localMCP := range []bool{false, true} {
		for _, operation := range []string{"new", "load", "reset"} {
			t.Run(operation+map[bool]string{false: "/without-local", true: "/with-local"}[localMCP], func(t *testing.T) {
				s := newTestServer(t)
				if localMCP {
					s = newTestServerWithMCP(t)
				}
				profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
				s.cfg.McpProfile = &profile
				s.cfg.AgentEnv = []string{"KANDEV_API_KEY=synthetic-broker-token", "KANDEV_RUN_ID=example-run"}
				config.RefreshAssistantPolicy(s.cfg)
				capture := &assistantSessionCapture{}
				s.procMgr.SetAdapterForTest(capture)
				msg, err := ws.NewRequest("request", "agent.session."+operation, LoadSessionRequest{
					SessionID: "existing-session",
					McpServers: []types.McpServer{
						{Name: "ambient", Type: "http", URL: "https://example.invalid/mcp"},
						{Name: "kandev_assistant", Type: "stdio", Command: "untrusted-broker"},
					},
				})
				require.NoError(t, err)
				var response *ws.Message
				var servers []types.McpServer
				switch operation {
				case "new":
					response = s.handleWSNewSession(context.Background(), msg)
					servers = capture.newSessionServers
				case "load":
					response = s.handleWSLoadSession(context.Background(), msg)
					servers = capture.loadSessionServers
				case "reset":
					response = s.handleWSResetSession(context.Background(), msg)
					servers = capture.resetServers
				}
				require.Equal(t, ws.MessageTypeResponse, response.Type)
				executable, err := os.Executable()
				require.NoError(t, err)
				require.Equal(t, []types.McpServer{{Name: "kandev_assistant", Type: "stdio", Command: executable,
					Args: []string{"kandev", "assistant-mcp"},
					Env:  map[string]string{"KANDEV_API_KEY": "synthetic-broker-token", "KANDEV_RUN_ID": "example-run"}}}, servers)
			})
		}
	}
}
