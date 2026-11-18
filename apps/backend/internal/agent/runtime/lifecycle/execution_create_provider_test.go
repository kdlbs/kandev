package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/stretchr/testify/require"
)

type failingSelectionReader struct{}

func (failingSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, errors.New("selection store unavailable")
}

func TestPrepareExecutionProviderFallsBackWhenRuntimeResolutionFails(t *testing.T) {
	mgr := newTestManager(t)
	mgr.managedRuntimeSelections = failingSelectionReader{}
	config, err := mgr.resolveLaunchableAgent(agents.CopilotACPAgentID)
	require.NoError(t, err)
	require.True(t, config.Enabled(), "the provider must be enabled so the runtime-resolution branch is reached")
	info := &WorkspaceInfo{AgentID: agents.CopilotACPAgentID, ExecutorType: "local"}

	provider, err := mgr.prepareExecutionProvider(context.Background(), info, false)
	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Nil(t, provider.agentConfig, "workspace-only creation must stay provider-free")

	_, err = mgr.prepareExecutionProvider(context.Background(), info, true)
	require.ErrorContains(t, err, "selection store unavailable", "explicit agent launches keep the resolution error")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = mgr.prepareExecutionProvider(ctx, info, false)
	require.Error(t, err, "cancellation must not fall back to a provider-free execution")
}
