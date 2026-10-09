package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// ExecuteBackgroundWorkAction sends an action request to the execution's agentctl instance.
func (m *Manager) ExecuteBackgroundWorkAction(ctx context.Context, executionID string, req streams.BackgroundWorkActionRequest) (streams.BackgroundWorkActionResponse, error) {
	var execution *AgentExecution
	if m.executionStore != nil {
		if exec, ok := m.executionStore.Get(executionID); ok && exec != nil {
			execution = exec
		} else if exec, ok := m.executionStore.GetBySessionID(executionID); ok && exec != nil {
			execution = exec
		}
	}
	if execution == nil {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "agent execution not found or not connected",
		}, nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return streams.BackgroundWorkActionResponse{
			Success: false,
			WorkID:  req.WorkID,
			RunID:   req.RunID,
			Action:  req.Action,
			Error:   "agentctl client not connected",
		}, nil
	}
	return client.ExecuteBackgroundWorkAction(ctx, req)
}
