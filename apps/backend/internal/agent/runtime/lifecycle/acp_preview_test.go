package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/acpprovider"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestACPPreviewLaunchAdmission(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	manager := &Manager{sessionManager: &SessionManager{}}
	req := &LaunchRequest{TaskID: "task", SessionID: "session", DeliveryStreamID: "stream", StartAgent: true}
	launch := &ExecutorCreateRequest{
		Protocol: "acp", AgentConfig: agents.NewClaudeACP(), ManagedRuntimeVersion: "0.109.0",
	}
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	manager.SetACPV2Enabled(true)
	require.True(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeSSH))
	req.StartAgent = false
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	req.StartAgent = true
	req.IsPassthrough = true
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	req.IsPassthrough = false
	launch.ProviderGatewayAuth = &acpprovider.GatewayAuth{}
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	launch.ProviderGatewayAuth = nil
	launch.ManagedRuntimeVersion = "0.110.0"
	require.False(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
	launch.AgentConfig, launch.ManagedRuntimeVersion = agents.NewCodexACP(), "2.9.0"
	require.True(t, manager.prefersACPV2(req, launch, agentruntime.RuntimeStandalone))
}

func TestACPPreviewRollbackFencesRetainedPromptBeforeAdmission(t *testing.T) {
	manager := &SessionManager{
		executorAdmission: func(ctx context.Context, _, _, _ string) (context.Context, error) {
			t.Fatal("retained v2 prompt reached executor admission")
			return ctx, nil
		},
	}
	client := agentctl.NewClient("127.0.0.1", 1, logger.Default(), agentctl.WithACPProtocolVersion(2))
	t.Cleanup(client.Close)
	execution := &AgentExecution{}
	execution.SetAgentCtlClientForTesting(client)
	_, err := manager.sendPrompt(t.Context(), execution, "new prompt", false, nil, true, sendPromptCallbacks{}, false)
	require.ErrorIs(t, err, errACPV2ReconnectRequired)
	require.ErrorIs(t, manager.callAgentctlPrompt(t.Context(), execution, "new prompt", nil, 1, false, ""), errACPV2ReconnectRequired)
	client.SetACPProtocolVersion(1)
	require.NoError(t, manager.acpPromptAdmission(execution))
}
