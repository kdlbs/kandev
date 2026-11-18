package process

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// ProviderReplacementSource is an observation, never permission to stop. The
// control protocol revalidates the complete tuple after it reserves admission.
func (m *Manager) ProviderReplacementSource() (types.ProviderReplacementSource, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.providerReplacementSourceLocked()
}

func (m *Manager) providerReplacementSourceLocked() (types.ProviderReplacementSource, error) {
	source := types.ProviderReplacementSource{
		ExecutionID: m.cfg.InstanceID, TaskID: m.cfg.TaskID, SessionID: m.cfg.SessionID,
		HelperID: m.ProcessManagerID(), ProcessGeneration: m.ProcessGeneration(), ConfigurationRevision: m.configRevision, StopGeneration: m.stopGeneration,
		StreamID: m.DeliveryStreamID(), IncarnationID: m.DeliveryIncarnationID(), HarnessGeneration: m.DeliveryHarnessGeneration(), NativeSessionID: m.GetSessionID(),
	}
	if m.cfg.McpProfile != nil && m.cfg.McpProfile.BuiltinPackages != nil {
		var err error
		source.ProfileDigest, err = p.BuiltinBindingDigest(*m.cfg.McpProfile)
		if err != nil {
			return source, err
		}
	}
	if m.cfg.ProviderMCP != nil {
		source.BindingID = m.cfg.ProviderMCP.Scope.BindingID
	}
	return source, nil
}

// RetireProviderReplacement stops only the observed provider group. The helper
// protocol owns the durable receipt and MCP effect barrier around this call.
func (m *Manager) RetireProviderReplacement(ctx context.Context, expected types.ProviderReplacementSource) error {
	return m.retireProviderReplacement(ctx, expected, false)
}

func (m *Manager) RetireProviderReplacementCandidate(ctx context.Context, expected types.ProviderReplacementSource) error {
	return m.retireProviderReplacement(ctx, expected, true)
}

func (m *Manager) retireProviderReplacement(ctx context.Context, expected types.ProviderReplacementSource, candidate bool) error {
	m.startOperationMu.Lock()
	defer m.startOperationMu.Unlock()
	m.startMu.Lock()
	current, err := m.providerReplacementSourceLocked()
	if candidate {
		current.NativeSessionID = expected.NativeSessionID
	}
	valid := err == nil && current == expected && expected.ProcessGeneration != 0 && (m.Status() == StatusRunning || candidate && m.Status() == StatusError)
	if valid && m.workspaceEnvironment == nil {
		m.workspaceEnvironment = &providerWorkspaceEnvironment{env: append([]string(nil), m.cfg.AgentEnv...)}
	}
	m.startMu.Unlock()
	if !valid {
		return errors.New("provider replacement source changed")
	}
	return m.stopWithWorkspace(ctx, false)
}
