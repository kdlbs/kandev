package lifecycle

import (
	"context"
	"errors"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

var ErrProviderMCPApplicationPending = errors.New("provider MCP application is pending")

// ProviderMCPControl exists only while provider prompt and context admission is
// exclusively held. Workspace services keep their independent ownership.
type ProviderMCPControl interface {
	Status(context.Context, string) (types.ProviderReplacementStatus, error)
	Command(context.Context, types.ProviderReplacementRequest) (types.ProviderReplacementStatus, error)
	CloseEffects() error
	OpenEffects()
	AdoptBinding(p.ProviderMCPBinding) error
}

type providerMCPControl struct {
	execution *AgentExecution
	client    *agentctl.Client
}

func (m *Manager) WithProviderMCPReplacement(ctx context.Context, sessionID, executionID string, fn func(ProviderMCPControl) error) error {
	execution, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok || execution.ID != executionID {
		return models.ErrExecutionRotated
	}
	release, err := execution.acquireContextResetExclusive(ctx)
	if err != nil {
		return err
	}
	defer release()
	execution.builtinMCPMu.Lock()
	defer execution.builtinMCPMu.Unlock()
	current, ok := m.executionStore.GetBySessionID(sessionID)
	if !ok || current != execution {
		return models.ErrExecutionRotated
	}
	if execution.idleSuspensionInProgress.Load() || execution.dispatchedPromptPending.Load() || m.sessionManager.activePromptGeneration(execution) != 0 {
		return errors.New("provider work is active")
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return errors.New("provider helper is unavailable")
	}
	return fn(&providerMCPControl{execution: execution, client: client})
}

func (c *providerMCPControl) Status(ctx context.Context, operationID string) (types.ProviderReplacementStatus, error) {
	return c.client.GetProviderReplacementStatus(ctx, operationID)
}
func (c *providerMCPControl) Command(ctx context.Context, request types.ProviderReplacementRequest) (types.ProviderReplacementStatus, error) {
	return c.client.ProviderReplacement(ctx, request)
}
func (c *providerMCPControl) CloseEffects() error { return c.execution.closeProviderMCPEffects() }
func (c *providerMCPControl) OpenEffects() {
	c.execution.providerEffectMu.Lock()
	defer c.execution.providerEffectMu.Unlock()
	c.execution.setMetadataValue(p.ProviderMCPEffectsClosedMetadataKey, false)
	c.execution.deleteMetadataValues(p.ProviderMCPOperationMetadataKey)
}
func (c *providerMCPControl) AdoptBinding(binding p.ProviderMCPBinding) error {
	_, profile, err := p.ParseProviderMCPBinding(binding)
	if err != nil {
		return err
	}
	if binding.Scope.ExecutionID != c.execution.ID || binding.Scope.HarnessGeneration != c.execution.DeliveryHarnessGeneration || profile.BuiltinPackages.TaskID != c.execution.TaskID || profile.BuiltinPackages.WorkspaceID != c.execution.WorkspaceID {
		return models.ErrExecutionRotated
	}
	c.execution.setMetadataValue(p.ProviderMCPBindingMetadataKey, binding)
	c.execution.deleteMetadataValues(p.ProviderMCPRuntimeProfileMetadataKey)
	return nil
}
