package sandbox

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxMCPPathsStayInsideCapturedRelay(t *testing.T) {
	launch := launchFixture(t)
	launch.MCPURL = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
	root, err := launch.MCPURLForPath("/mcp")
	require.NoError(t, err)
	require.Equal(t, launch.MCPURL, root)
	_, err = launch.MCPURLForPath("/mcp/packages/configuration")
	require.ErrorIs(t, err, ErrInvalidPolicy, "legacy captures allow only their original core route")
	launch.MCPPaths = []string{"/mcp", "/mcp/packages/rich-output"}
	require.NoError(t, launch.Validate())
	selected, err := launch.MCPURLForPath("/mcp/packages/rich-output")
	require.NoError(t, err)
	require.Equal(t, launch.MCPURL+"/packages/rich-output", selected)
	_, err = launch.MCPURLForPath("/mcp/packages/configuration")
	require.ErrorIs(t, err, ErrInvalidPolicy)
	copy := launch.Clone()
	copy.MCPPaths[1] = "/mcp/packages/configuration"
	require.Equal(t, "/mcp/packages/rich-output", launch.MCPPaths[1], "launch clones must not share mutable route authority")
}

func TestSandboxLaunchRefusesInvalidMCPPaths(t *testing.T) {
	for _, paths := range [][]string{
		{"/mcp", "/api/v1/settings"},
		{"/mcp", "/mcp/packages/../configuration"},
		{"/mcp", "/mcp/packages/%63onfiguration"},
		{"/mcp", "/mcp/packages/configuration?other=1"},
		{"/mcp", "/mcp/packages/configuration/child"},
		{"/mcp", "/mcp"},
		{"/mcp/packages/configuration"},
	} {
		launch := launchFixture(t)
		launch.MCPURL = "http://127.0.0.1:31234/" + strings.Repeat("a", 64) + "/mcp"
		launch.MCPPaths = paths
		require.ErrorIs(t, launch.Validate(), ErrInvalidPolicy)
	}
	launch := launchFixture(t)
	launch.MCPPaths = []string{"/mcp"}
	require.ErrorIs(t, launch.Validate(), ErrInvalidPolicy, "route declarations cannot create a missing MCP grant")
}
