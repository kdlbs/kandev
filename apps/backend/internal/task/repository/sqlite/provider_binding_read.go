package sqlite

import (
	"context"
	"database/sql"
	"errors"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// GetAcceptedProviderMCPBinding reports application only after helper acceptance
// and the final SQL publication. Initial launch evidence stays independent.
func (r *Repository) GetAcceptedProviderMCPBinding(ctx context.Context, sessionID, executionID string) (*models.ProviderMCPBindingSnapshot, error) {
	running, err := r.GetExecutorRunningBySessionID(ctx, sessionID)
	if errors.Is(err, models.ErrExecutorRunningNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if running.AgentExecutionID != executionID {
		return nil, nil
	}
	if flag, present := running.Metadata[p.ProviderMCPEffectsClosedMetadataKey]; present {
		closed, valid := flag.(bool)
		if !valid || closed {
			return nil, nil
		}
	}
	binding, _, err := p.ParseProviderMCPBinding(running.Metadata[p.ProviderMCPBindingMetadataKey])
	if err != nil {
		return nil, nil
	}
	var snapshot models.ProviderMCPBindingSnapshot
	err = r.ro.GetContext(ctx, &snapshot, r.ro.Rebind(`SELECT * FROM agent_provider_mcp_bindings WHERE binding_id=? AND session_id=? AND task_id=? AND execution_id=? AND accepted_at IS NOT NULL`), binding.Scope.BindingID, sessionID, running.TaskID, executionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &snapshot, nil
}
