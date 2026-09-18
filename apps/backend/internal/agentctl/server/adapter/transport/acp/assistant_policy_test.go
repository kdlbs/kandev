package acp

import (
	"context"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyAdapterNewAndResume(t *testing.T) {
	a, capture := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.ToolPolicy = "claude-broker-v1"
	a.agentID = "claude-acp"
	a.agentInfo = &AgentInfo{Name: "@agentclientprotocol/claude-agent-acp", Version: "0.75.1"}
	servers := []types.McpServer{{Name: "kandev_assistant", Command: "/owned/agentctl", Args: []string{"kandev", "assistant-mcp"}}, {Name: "untrusted", Command: "mutate"}}
	_, err := a.NewSession(context.Background(), servers)
	require.ErrorContains(t, err, "attachment")
	_, err = a.NewSession(context.Background(), servers[:1])
	require.NoError(t, err)
	require.Len(t, capture.newRequest.McpServers, 1)
	options := capture.newRequest.Meta["claudeCode"].(map[string]any)["options"].(map[string]any)
	require.Empty(t, options["tools"])
	require.Empty(t, options["settingSources"])
	require.Equal(t, true, options["strictMcpConfig"])
	require.Equal(t, map[string]any{"disableAllHooks": true}, options["settings"])
	require.Equal(t, map[string]any{"disable-slash-commands": nil, "permission-mode": "dontAsk"}, options["extraArgs"])
	require.NoError(t, a.LoadSession(context.Background(), "previous-restricted-session", servers[:1]))
	require.Equal(t, capture.newRequest.Meta, capture.loadRequest.Meta)
	require.Error(t, a.SetMode(context.Background(), "bypassPermissions"))
	require.Error(t, a.SetConfigOption(context.Background(), "mode", "bypassPermissions"))
	response, err := a.handlePermissionRequest(context.Background(), &PermissionRequest{})
	require.NoError(t, err)
	require.True(t, response.Cancelled)
}

func TestAssistantReadOnlyAdapterRejectsUnprovenVersion(t *testing.T) {
	a, _ := newSessionRequestCaptureAdapter(t, acpsdk.McpCapabilities{})
	a.cfg.ToolPolicy = "claude-broker-v1"
	a.agentID = "claude-acp"
	a.agentInfo = &AgentInfo{Version: "0.75.2"}
	_, err := a.NewSession(context.Background(), nil)
	require.ErrorContains(t, err, "unsupported")
}
