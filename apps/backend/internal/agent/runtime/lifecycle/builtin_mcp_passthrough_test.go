package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinPassthroughUsesBoundPackagesAndRejectsCollisions(t *testing.T) {
	mgr := newTestManager(t)
	execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task-a", WorkspaceID: "workspace", standalonePort: 41234}
	profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
	encoded, err := p.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
	operation := &preparedMCPOperation{servers: []types.McpServer{{Name: "third-party", Type: "http", URL: "https://example.test/mcp"}}}
	servers, err := mgr.passthroughMCPServers(context.Background(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, operation)
	require.NoError(t, err)
	require.Len(t, servers, 3)
	require.Equal(t, "kandev-configuration", servers[1].Name)
	require.Equal(t, "http://localhost:41234/mcp/packages/configuration", servers[1].URL)

	evidence := buildPassthroughMCPAttempt(execution, "process", &preparedMCPOperation{servers: servers})
	for _, server := range evidence.Servers {
		if server.Name == "kandev-configuration" {
			require.Equal(t, streams.MCPServerSourceKandev, server.Source)
		}
	}
	operation.servers[0].Name = "kandev-diagnostics"
	_, err = mgr.passthroughMCPServers(context.Background(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, operation)
	require.Error(t, err)
}
