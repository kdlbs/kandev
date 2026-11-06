package process

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

type nativeExecutionUsage struct {
	mu      sync.RWMutex
	policy  *usage.ExecutionUsagePolicy
	input   usage.ExecutionUsageInput
	reason  string
	service *usage.UsageService
}

// ConfigureExecutionUsage follows Configure, while no child is running. A nil
// policy preserves the disabled legacy path and does not inspect credentials.
func (m *Manager) ConfigureExecutionUsage(policy *usage.ExecutionUsagePolicy) error {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return fmt.Errorf("cannot configure usage while agent is running")
	}
	if policy != nil && (policy.AgentID != m.cfg.AgentType) {
		return fmt.Errorf("usage execution identity does not match instance")
	}
	m.executionUsage.mu.Lock()
	defer m.executionUsage.mu.Unlock()
	m.executionUsage.policy = nil
	m.executionUsage.input = usage.ExecutionUsageInput{}
	m.executionUsage.reason = "execution_unknown"
	if policy != nil {
		copy := *policy
		m.executionUsage.policy = &copy
		if m.executionUsage.service == nil {
			m.executionUsage.service = usage.NewUsageService()
		}
	}
	return nil
}

// Called under startMu after buildAdapterConfig has composed the exact child
// environment. Recovered old processes without a native pin remain unknown.
func (m *Manager) bindExecutionUsage(ctx context.Context, commands ...[]string) {
	m.executionUsage.mu.RLock()
	policy, service := m.executionUsage.policy, m.executionUsage.service
	m.executionUsage.mu.RUnlock()
	if policy == nil {
		return
	}
	input := usage.ExecutionInputFromEnvironment(*policy, m.cfg.InstanceID, "", m.cfg.AgentEnv)
	input.WorkDir = m.cfg.WorkDir
	input.Command = append([]string(nil), m.cfg.AgentArgs...)
	if len(commands) == 1 {
		input.Command = append([]string(nil), commands[0]...)
	}
	reason := "provider_selection_unknown"
	if m.cfg.ProviderGatewayAuth == nil && usage.NativeProviderSelectionKnown(input) {
		input, reason, _ = service.BindExecutionUsage(ctx, input)
	}
	m.executionUsage.mu.Lock()
	m.executionUsage.input, m.executionUsage.reason = input, reason
	m.executionUsage.mu.Unlock()
}

// ReadExecutionUsage never rebinds an existing process to a newly logged-in
// account. It checks the native pin against that process's final source.
func (m *Manager) ReadExecutionUsage(ctx context.Context, _ string) *usage.ExecutionUsageReport {
	model := m.executionUsageModel()
	m.executionUsage.mu.RLock()
	input, reason, service := m.executionUsage.input, m.executionUsage.reason, m.executionUsage.service
	m.executionUsage.mu.RUnlock()
	if reason != "ok" || service == nil {
		if reason == "" {
			reason = "execution_unknown"
		}
		return &usage.ExecutionUsageReport{Reason: reason, Model: model}
	}
	input.Model = model
	if !usage.NativeProviderSelectionKnown(input) {
		return &usage.ExecutionUsageReport{Reason: "binding_changed", Model: model, BindingChanged: true}
	}
	return usage.ReadExecutionUsageReport(ctx, service, input)
}

// The native adapter's actual model snapshot is authoritative. An eventual
// backend cache or query argument cannot exclude a model-specific limit.
func (m *Manager) executionUsageModel() string {
	provider, ok := m.GetAdapter().(adapter.SessionModelStateProvider)
	if !ok {
		return ""
	}
	state := provider.GetSessionModelState()
	if state == nil {
		return ""
	}
	return state.CurrentModelID
}
