package lifecycle

import (
	"context"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"strings"
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

func TestSandboxBuiltinPassthroughPreservesNamespacesAndDenial(t *testing.T) {
	for _, mode := range []string{"allowed", "denied", "uncaptured", "unprepared"} {
		t.Run(mode, func(t *testing.T) {
			mgr := newTestManager(t)
			profile := builtinDispatchProfile(p.PackageCore, p.PackageConfiguration)
			encoded, err := p.MarshalBuiltinBinding(profile)
			require.NoError(t, err)
			execution := &AgentExecution{ID: "execution", SessionID: "session", TaskID: "task-a", WorkspaceID: "workspace", standalonePort: 41234, SandboxSelection: &policy.SelectionContext{ExecutorID: "local"}}
			execution.setMetadataValue(p.BuiltinBindingMetadataKey, encoded)
			launch := &contract.Launch{MCPURL: "http://127.0.0.1:45679/" + strings.Repeat("a", 64) + "/mcp", MCPPaths: []string{"/mcp", "/mcp/packages/configuration"}}
			if mode == "denied" {
				launch.MCPURL = ""
				launch.MCPPaths = nil
			}
			if mode == "uncaptured" {
				launch.MCPPaths = nil
			}
			if mode != "unprepared" {
				execution.sandboxAttempt = &startSandboxFixture{launch: launch}
			}
			external := types.McpServer{Name: "third-party", Type: "http", URL: "https://example.test/mcp"}
			operation := &preparedMCPOperation{servers: []types.McpServer{external}}
			servers, err := mgr.passthroughMCPServers(t.Context(), execution, nil, nil, "", mcpconfig.CodexStrategy{}, operation)
			if mode == "uncaptured" || mode == "unprepared" {
				require.Error(t, err)
				require.Nil(t, servers)
				return
			}
			require.NoError(t, err)
			if mode == "denied" {
				require.Equal(t, []types.McpServer{external}, servers)
				return
			}
			require.Len(t, servers, 3)
			require.Equal(t, launch.MCPURL, servers[0].URL)
			require.Equal(t, "kandev-configuration", servers[1].Name)
			require.Equal(t, launch.MCPURL+"/packages/configuration", servers[1].URL)
			require.Equal(t, external, servers[2])
			composed, _, err := composeEffectiveMCPServers(execution, nil, nil, nil, "localhost")
			require.NoError(t, err)
			require.Equal(t, map[string]any{"url": launch.MCPURL + "/packages/configuration"}, composed["kandev-configuration"])
		})
	}
}
