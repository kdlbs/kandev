package lifecycle

import (
	"context"
	"errors"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

type builtinMCPBindingWriter interface {
	UpdateBuiltinMCPBinding(context.Context, string, string, any) error
}

func (m *Manager) persistBuiltinMCPBinding(ctx context.Context, execution *AgentExecution) error {
	current, ok := m.executionStore.GetBySessionID(execution.SessionID)
	if !ok || current != execution {
		return models.ErrExecutionRotated
	}
	if m.runningWriter == nil {
		return nil
	}
	writer, ok := m.runningWriter.(builtinMCPBindingWriter)
	if !ok {
		return errors.New("built-in MCP binding persistence is unavailable")
	}
	value, present := execution.metadataValue(p.BuiltinBindingMetadataKey)
	if !present {
		return errors.New("built-in MCP binding is missing")
	}
	return writer.UpdateBuiltinMCPBinding(ctx, execution.SessionID, execution.ID, value)
}
