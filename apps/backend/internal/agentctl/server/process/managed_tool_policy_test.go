package process

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestManagedToolPolicyAdapter(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	managedLogger, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: "coordinator.v1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		InstanceKey: "lead", ConversationRevision: 3, ApprovalRevision: 2,
		ManifestDigest: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AgentToolNames: []string{"read_task"},
	}
	profile := mcpprofile.New(mcpprofile.SurfaceManagedConversation, nil, nil).WithManagedToolPolicy(policy)
	m := &Manager{cfg: &config.InstanceConfig{
		Port: 43210, InjectedKandevMCP: true, McpProfile: &profile,
		McpServers: []config.McpServerConfig{
			{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"},
			{Name: "kandev", Type: "sse", URL: "http://localhost:43210/sse"},
			{Name: "ambient", Type: "http", URL: "https://mcp.example.test/mcp"},
		},
		AutoApprovePermissions: true,
	}, logger: managedLogger}

	servers, err := m.MCPServersForSession([]types.McpServer{
		{Name: "kandev", Type: "stdio", Command: "attacker-controlled"},
		{Name: "foreign", Type: "http", URL: "https://foreign.example.test/mcp"},
	})
	require.NoError(t, err)
	require.Len(t, servers, 2)
	require.Equal(t, types.McpServer{Name: "kandev", Type: "http", URL: "http://localhost:43210/mcp"}, servers[0])
	require.Equal(t, types.McpServer{Name: "kandev", Type: "sse", URL: "http://localhost:43210/sse"}, servers[1])
	adapterServers, err := m.adapterMCPServers()
	require.NoError(t, err)
	require.Len(t, adapterServers, 2, "adapter launch must not inherit ambient MCP configuration")
	require.False(t, m.adapterAutoApprove(), "managed policy must disable adapter-wide auto-approval")

	response, err := m.handlePermissionRequest(context.Background(), &adapter.PermissionRequest{
		ToolName: permissionStringPtr("terminal.exec"),
		Options:  []adapter.PermissionOption{{OptionID: "allow", Kind: "allow_once"}},
	})
	require.NoError(t, err)
	require.True(t, response.Cancelled, "managed policy must deny native tools even when auto-approve is enabled")
	entries := observed.FilterMessage("managed agent tool policy denied a native permission request").All()
	require.Len(t, entries, 1, "native permission denials must be auditable")
	require.Equal(t, "native_tool_denied", entries[0].ContextMap()["reason"])
	require.Equal(t, "terminal.exec", entries[0].ContextMap()["tool_name"])
}
