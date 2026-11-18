package lifecycle

import (
	"context"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	p "github.com/kandev/kandev/internal/mcp/profile"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderReplacementClosedBarrierBlocksPromptAndContextChanges(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls++; w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	manager, execution := workspaceSourceTestManager(t, server.URL, nil)
	execution.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, true)
	release, err := execution.acquireContextResetOperation(context.Background())
	if release != nil {
		release()
	}
	require.Error(t, err, "an irreversible provider refresh must close prompt admission")
	err = manager.SetMcpMode(context.Background(), execution.ID, "task_title_pending")
	require.Error(t, err, "context updates cannot rewrite a provider while replacement owns it")
	require.Zero(t, calls)
}

func TestProviderReplacementFinalizationCannotFailTaskTurn(t *testing.T) {
	manager, bus := createTestManagerWithTracking()
	execution := createTestExecution("execution", "task", "session")
	require.NoError(t, manager.executionStore.Add(execution))
	execution.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, true)
	manager.handleAgentEvent(execution, agentctl.AgentEvent{Type: "error", Error: "replacement provider exited before final publication"})
	require.Equal(t, v1.AgentStatusRunning, execution.Status)
	require.Empty(t, bus.PublishedEvents, "replacement failure must remain separate from task completion and workflow routing")
}
