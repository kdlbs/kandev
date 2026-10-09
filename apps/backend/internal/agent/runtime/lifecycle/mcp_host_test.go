package lifecycle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestPassthroughMCPUsesStandaloneListenHost(t *testing.T) {
	const host = "192.0.2.10"
	mgr, execution, profile := newClaudePassthroughMCPTestManager(t)
	execution.RuntimeName = agentruntime.RuntimeStandalone
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(NewStandaloneExecutor(nil, host, 0, newTestRegistryLogger()))
	agent := agents.NewClaudeACP()

	servers, err := mgr.passthroughMCPServers(
		context.Background(), execution, agent, profile, string(executor.NameLocal), mcpconfig.ClaudeStrategy{},
	)
	require.NoError(t, err)
	require.NotEmpty(t, servers)
	require.Equal(t, "http://192.0.2.10:45678/mcp", servers[0].URL)
}

func TestCursorProjectMCPUsesStandaloneListenHost(t *testing.T) {
	const host = "192.0.2.10"
	t.Setenv("HOME", t.TempDir())
	mgr := newTestManager(t)
	mgr.executorRegistry = NewExecutorRegistry(newTestRegistryLogger())
	mgr.executorRegistry.Register(NewStandaloneExecutor(nil, host, 0, newTestRegistryLogger()))
	execution := &AgentExecution{
		ID: "exec-mcp-host", WorkspacePath: t.TempDir(), standalonePort: 45678,
		RuntimeName: agentruntime.RuntimeStandalone,
	}
	agent := agents.NewCursorACP()

	err := mgr.reconcileAndMaterializeCursorProjectMCP(
		context.Background(), execution, agent, &AgentProfileInfo{ProfileID: "profile-1"},
		string(executor.NameLocal), agent.Runtime().ProjectMCPStrategy,
	)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(execution.WorkspacePath, ".cursor", "mcp.json"))
	require.NoError(t, err)
	var payload struct {
		MCPServers map[string]struct {
			URL string `json:"url"`
		} `json:"mcpServers"`
	}
	require.NoError(t, json.Unmarshal(data, &payload))
	require.Equal(t, "http://192.0.2.10:45678/mcp", payload.MCPServers["kandev"].URL)
}
