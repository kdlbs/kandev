package process

import (
	"github.com/kandev/kandev/internal/agentctl/server/config"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestWorkspaceClaudeBrokerUsesExistingNarrowToolPolicy(t *testing.T) {
	for _, tc := range []struct {
		agent   string
		surface mcpprofile.Surface
		policy  string
	}{
		{"claude-acp", mcpprofile.SurfaceOrchestratorBroker, config.AssistantToolPolicy},
		{"auggie", mcpprofile.SurfaceOrchestratorBroker, ""},
	} {
		t.Run(tc.agent, func(t *testing.T) {
			profile := mcpprofile.New(tc.surface, nil, nil)
			m := &Manager{cfg: &config.InstanceConfig{AgentArgs: []string{"cat"}, AgentType: tc.agent, WorkDir: t.TempDir(), Protocol: agent.ProtocolACP, McpProfile: &profile, AgentEnv: []string{"CLAUDE_CONFIG_DIR=/synthetic/account"}}, logger: newTestLogger(t)}
			require.NoError(t, m.buildAdapterConfig())
			t.Cleanup(func() { _ = m.adapter.Close() })
			require.Equal(t, tc.policy, m.adapterCfg.ToolPolicy)
			require.False(t, m.adapterCfg.AutoApprove)
			require.Contains(t, m.cfg.AgentEnv, "CLAUDE_CONFIG_DIR=/synthetic/account")
			require.Len(t, m.adapterCfg.McpServers, 1)
			require.Equal(t, "workspace", m.adapterCfg.McpServers[0].Env["KANDEV_ORCHESTRATOR_SCOPE"])
		})
	}
}
