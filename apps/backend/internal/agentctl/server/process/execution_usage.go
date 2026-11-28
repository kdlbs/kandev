package process

import (
	"context"
	"fmt"
	"sync"

	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

const nativeUsageBindingChanged = "binding_changed"

type nativeExecutionUsage struct {
	selectionModelPending bool
	mu                    sync.RWMutex
	policy                *usage.ExecutionUsagePolicy
	input                 usage.ExecutionUsageInput
	reason                string
	diagnostic            *usage.ExecutionUsageDiagnostic
	service               *usage.UsageService
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
	m.executionUsage.selectionModelPending = false
	m.executionUsage.input = usage.ExecutionUsageInput{}
	m.executionUsage.reason = "execution_unknown"
	m.executionUsage.diagnostic = nil
	if policy != nil {
		copy := *policy
		m.executionUsage.policy = &copy
		m.executionUsage.selectionModelPending = copy.ExpectedAccountRef != "" && copy.ExpectedModel != ""
		if m.executionUsage.service == nil {
			m.executionUsage.service = usage.NewUsageService()
		}
	}
	return nil
}

// Called under startMu after buildAdapterConfig has composed the exact child
// environment. Recovered old processes without a native pin remain unknown.
func (m *Manager) bindExecutionUsage(ctx context.Context, commands ...[]string) error {
	m.executionUsage.mu.RLock()
	policy, service := m.executionUsage.policy, m.executionUsage.service
	m.executionUsage.mu.RUnlock()
	if policy == nil {
		return nil
	}
	input := usage.ExecutionInputFromEnvironment(*policy, m.cfg.InstanceID, "", m.cfg.AgentEnv)
	input.WorkDir = m.cfg.WorkDir
	input.Command = append([]string(nil), m.cfg.AgentArgs...)
	if len(commands) == 1 {
		input.Command = append([]string(nil), commands[0]...)
	}
	reason := usage.NativeProviderSelectionReason(input)
	var bindingErr error
	if m.cfg.ProviderGatewayAuth != nil || !m.cfg.LaunchWrapper.IsZero() {
		reason = "provider_selection_unknown"
	} else if reason == "" {
		input, reason, bindingErr = service.BindExecutionUsage(ctx, input)
	}
	changed := usage.ExecutionUsageSelectionChanged(*policy, input, reason, "", false)
	if changed {
		reason = nativeUsageBindingChanged
	}
	m.executionUsage.mu.Lock()
	m.executionUsage.input, m.executionUsage.reason = input, reason
	m.executionUsage.diagnostic = usage.ExecutionUsageErrorDiagnostic(bindingErr)
	m.executionUsage.mu.Unlock()
	if changed {
		return &usage.ExecutionUsageBindingError{}
	}
	return nil
}

// ReadExecutionUsage never rebinds an existing process to a newly logged-in
// account. It checks the native pin against that process's final source.
func (m *Manager) ReadExecutionUsage(ctx context.Context, _ string) *usage.ExecutionUsageReport {
	return m.readExecutionUsage(ctx, false)
}

// PeekExecutionUsage is safe for eligibility previews before a real run exists:
// only this actor's native pin and existing observations are inspected.
func (m *Manager) PeekExecutionUsage(ctx context.Context) *usage.ExecutionUsageReport {
	return m.readExecutionUsage(ctx, true)
}

func (m *Manager) readExecutionUsage(ctx context.Context, cachedOnly bool) *usage.ExecutionUsageReport {
	model := m.executionUsageModel()
	m.executionUsage.mu.RLock()
	input, reason, service, policy := m.executionUsage.input, m.executionUsage.reason, m.executionUsage.service, m.executionUsage.policy
	selectionPending := m.executionUsage.selectionModelPending
	var diagnostic *usage.ExecutionUsageDiagnostic
	if m.executionUsage.diagnostic != nil {
		copy := *m.executionUsage.diagnostic
		diagnostic = &copy
	}
	m.executionUsage.mu.RUnlock()
	if reason != "ok" || service == nil {
		if reason == "" {
			reason = "execution_unknown"
		}
		changed := policy != nil && policy.ExpectedAccountRef != ""
		if changed {
			reason = nativeUsageBindingChanged
		}
		return &usage.ExecutionUsageReport{Reason: reason, Model: model, BindingChanged: changed, Diagnostic: diagnostic}
	}
	input.Model = model
	if policy != nil && usage.ExecutionUsageSelectionChanged(*policy, input, reason, model, selectionPending) {
		return &usage.ExecutionUsageReport{Reason: nativeUsageBindingChanged, Model: model, BindingChanged: true}
	}
	if !usage.NativeProviderSelectionKnown(input) {
		return &usage.ExecutionUsageReport{Reason: nativeUsageBindingChanged, Model: model, BindingChanged: true}
	}
	if cachedOnly {
		return service.PeekExecutionUsage(ctx, input)
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

// SetExecutionUsageService is composition-only, before this instance accepts
// requests. Native pins stay per actor; only the existing measured sample cache
// is shared within one native control process, never between runtime owners.
func (m *Manager) SetExecutionUsageService(service *usage.UsageService) {
	if service == nil {
		return
	}
	m.executionUsage.mu.Lock()
	defer m.executionUsage.mu.Unlock()
	m.executionUsage.service = service
}

// The selected model constrains the initial actual prompt, not later explicit
// model changes in the same conversation. The execution account pin remains.
func (m *Manager) ConsumeUsageSelectionModel() error {
	m.executionUsage.mu.RLock()
	pending, policy := m.executionUsage.selectionModelPending, m.executionUsage.policy
	m.executionUsage.mu.RUnlock()
	if !pending || policy == nil {
		return nil
	}
	if m.executionUsageModel() != policy.ExpectedModel {
		return &usage.ExecutionUsageBindingError{}
	}
	m.executionUsage.mu.Lock()
	if m.executionUsage.policy == policy {
		m.executionUsage.selectionModelPending = false
	}
	m.executionUsage.mu.Unlock()
	return nil
}
