package lifecycle

import (
	"context"
	"errors"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

type providerRuntimeProfileWriter interface {
	UpdateProviderMCPRuntimeProfile(context.Context, string, string, string, any) error
}

func (m *Manager) updateProviderRuntimeProfile(ctx context.Context, execution *AgentExecution, value any, change func(*p.Context) error, apply func() error) error {
	binding, _, err := p.ParseProviderMCPBinding(value)
	if err != nil {
		return err
	}
	previous, present := execution.metadataValue(p.ProviderMCPRuntimeProfileMetadataKey)
	runtime, err := p.ProviderRuntimeProfile(binding, previous, present)
	if err != nil {
		return err
	}
	if err := change(runtime); err != nil {
		return err
	}
	encoded, err := p.MarshalBuiltinBinding(*runtime)
	if err != nil {
		return err
	}
	if _, err := p.ProviderRuntimeProfile(binding, encoded, true); err != nil {
		return err
	}
	if present && encoded == previous {
		return apply()
	}
	pending := map[string]any{"update_incomplete": true}
	execution.setMetadataValue(p.ProviderMCPRuntimeProfileMetadataKey, pending)
	if err := m.persistProviderRuntimeProfile(ctx, execution, binding.Scope.BindingID, pending); err != nil {
		if present {
			execution.setMetadataValue(p.ProviderMCPRuntimeProfileMetadataKey, previous)
		} else {
			execution.deleteMetadataValues(p.ProviderMCPRuntimeProfileMetadataKey)
		}
		return err
	}
	if err := apply(); err != nil {
		return err
	}
	execution.setMetadataValue(p.ProviderMCPRuntimeProfileMetadataKey, encoded)
	if err := m.persistProviderRuntimeProfile(ctx, execution, binding.Scope.BindingID, encoded); err != nil {
		execution.setMetadataValue(p.ProviderMCPRuntimeProfileMetadataKey, pending)
		return err
	}
	return nil
}

func (m *Manager) persistProviderRuntimeProfile(ctx context.Context, execution *AgentExecution, bindingID string, value any) error {
	current, ok := m.executionStore.GetBySessionID(execution.SessionID)
	if !ok || current != execution {
		return models.ErrExecutionRotated
	}
	if m.runningWriter == nil {
		return nil
	}
	writer, ok := m.runningWriter.(providerRuntimeProfileWriter)
	if !ok {
		return errors.New("provider runtime context persistence is unavailable")
	}
	return writer.UpdateProviderMCPRuntimeProfile(ctx, execution.SessionID, execution.ID, bindingID, value)
}
