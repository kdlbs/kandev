package lifecycle

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/profiles"
)

var errACPV2ReconnectRequired = errors.New("ACP v2 is disabled. Reconnect this session to send new prompts over ACP v1")

// SetACPV2Enabled binds the effective startup flag to launch and dispatch.
func (m *Manager) SetACPV2Enabled(enabled bool) {
	m.sessionManager.acpV2Enabled = enabled
}

func (m *Manager) prefersACPV2(req *LaunchRequest, launch *ExecutorCreateRequest, runtime agentruntime.Runtime) bool {
	if launch.ProviderGatewayAuth != nil || launch.AuthPolicy != nil || launch.AgentConfig == nil {
		return false
	}
	preferNative := m.preferNativeBinary(launch.AgentConfig, runtime, launch.Metadata)
	return m.canLaunchACPV2(req, launch.AgentConfig, runtime, launch.ManagedRuntimeVersion, preferNative)
}

func (m *Manager) canLaunchACPV2(req *LaunchRequest, agentConfig agents.Agent, runtime agentruntime.Runtime, version string, preferNative bool) bool {
	if m.sessionManager == nil || !m.sessionManager.acpV2Enabled || runtime != agentruntime.RuntimeStandalone ||
		!req.StartAgent || req.IsPassthrough || req.TaskID == "" || req.SessionID == "" ||
		req.DeliveryStreamID == "" || protocolForAgent(agentConfig) != "acp" {
		return false
	}
	switch req.McpMode {
	case "", "task", "task-title-pending":
	default:
		return false
	}
	id := agentConfig.ID()
	if id == "mock-agent" && profiles.DetectEnvironment() == profiles.EnvE2E {
		return true
	}
	return qualifiedACPV2Version(id, version) && !preferNative
}

func (m *Manager) buildPromotedAgentCommand(
	ctx context.Context, req *LaunchRequest, profile *AgentProfileInfo, agentConfig agents.Agent, execution *AgentExecution,
) (agentCommands, bool, error) {
	runtime := execution.RuntimeName
	preferNative := m.preferNativeBinary(agentConfig, runtime, execution.MetadataSnapshot())
	options, err := m.resolveManagedRuntimeCommandOptions(ctx, runtime, agentConfig)
	if err != nil {
		return agentCommands{}, false, err
	}
	cmds, err := m.buildAgentCommandWithRuntimeOptions(ctx, req, profile, agentConfig, runtime, preferNative, options)
	if err != nil {
		return agentCommands{}, false, err
	}
	ordinaryAuth := req.AuthPolicy == nil && execution.AuthPolicy == nil &&
		(profile == nil || profile.ProviderKind != settingsmodels.ProviderKindOpenAICompatible)
	preferV2 := ordinaryAuth && m.canLaunchACPV2(req, agentConfig, runtime, options.ManagedRuntimeVersion, preferNative)
	return cmds, preferV2, nil
}

func qualifiedACPV2Version(id, version string) bool {
	switch id {
	case "claude-acp":
		return version == "0.109.0" || version == "0.109.1"
	case "codex-acp":
		return version == "2.8.0" || version == "2.9.0"
	default:
		return false
	}
}

func (sm *SessionManager) acpPromptAdmission(execution *AgentExecution) error {
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	return sm.acpClientPromptAdmission(client)
}

func (sm *SessionManager) acpClientPromptAdmission(client *agentctl.Client) error {
	if !sm.acpV2Enabled && client != nil && client.ACPProtocolVersion() == 2 {
		return errACPV2ReconnectRequired
	}
	return nil
}
