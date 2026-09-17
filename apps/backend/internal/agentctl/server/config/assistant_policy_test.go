package config

import (
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestAssistantReadOnlyConfigurationOverrides(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceAssistantBroker, nil, nil)
	config := &Config{Defaults: InstanceDefaults{AutoApprovePermissions: true}, ShellEnabled: true}
	cfg := config.NewInstanceConfig(43210, &InstanceOverrides{McpProfile: &profile,
		McpServers: []McpServerConfig{{Name: "external", Command: "mutate"}},
		Env:        []string{"KANDEV_API_KEY=synthetic-token", "CLAUDE_CODE_EXECUTABLE=mutate", "NODE_OPTIONS=--require=mutate", "PATH=/bin"}})
	require.False(t, cfg.AutoApprovePermissions)
	require.False(t, cfg.ShellEnabled)
	require.Len(t, cfg.McpServers, 1)
	require.Equal(t, "kandev_assistant", cfg.McpServers[0].Name)
	require.Equal(t, "synthetic-token", cfg.McpServers[0].Env["KANDEV_API_KEY"])
	require.NotContains(t, strings.Join(cfg.AgentEnv, "\n"), "mutate")
	good := []string{"npx", "--yes", "--prefer-offline", "@agentclientprotocol/claude-agent-acp@0.75.1"}
	require.NoError(t, ValidateAssistantCommand(cfg, good))
	for _, bad := range [][]string{{"sh", "-c", "mutate"}, append(good, "--dangerously-skip-permissions"), {"npx", "--yes", "--prefer-offline", "@agentclientprotocol/claude-agent-acp@latest"}} {
		require.Error(t, ValidateAssistantCommand(cfg, bad))
	}
}
