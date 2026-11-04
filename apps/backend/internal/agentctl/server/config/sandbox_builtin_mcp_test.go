package config

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestBuiltinMCPServersUseCapturedSandboxRoutes(t *testing.T) {
	profile := builtinInstanceTestProfile()
	launch := &sandbox.Launch{MCPURL: "http://127.0.0.1:45679/" + strings.Repeat("a", 64) + "/mcp", MCPPaths: []string{"/mcp", "/mcp/packages/configuration", "/mcp/packages/rich-output"}}
	servers, err := BuiltinMCPServersForLaunch(profile.BuiltinPackages, "localhost", 12345, launch)
	require.NoError(t, err)
	require.Equal(t, []McpServerConfig{
		{Name: "kandev", Type: "http", URL: launch.MCPURL},
		{Name: "kandev-configuration", Type: "http", URL: launch.MCPURL + "/packages/configuration"},
		{Name: "kandev-rich-output", Type: "http", URL: launch.MCPURL + "/packages/rich-output"},
	}, servers)
	launch.MCPPaths = []string{"/mcp", "/mcp/packages/configuration"}
	servers, err = BuiltinMCPServersForLaunch(profile.BuiltinPackages, "localhost", 12345, launch)
	require.Error(t, err, "an uncaptured selected route must not fall back to agentctl")
	require.Nil(t, servers)
	launch.MCPURL = ""
	launch.MCPPaths = nil
	servers, err = BuiltinMCPServersForLaunch(profile.BuiltinPackages, "localhost", 12345, launch)
	require.NoError(t, err)
	require.Empty(t, servers, "no logical grant means no automatic MCP access")
	servers, err = BuiltinMCPServersForLaunch(profile.BuiltinPackages, "localhost", 12345, nil)
	require.NoError(t, err)
	require.Len(t, servers, 6, "ordinary launches retain HTTP and SSE")
}
