package lifecycle

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// finalWorkspaceRefreshTimeout bounds the synchronous scan at a turn
// boundary. It is a variable so lifecycle tests can keep the boundary short.
var finalWorkspaceRefreshTimeout = 10 * time.Second

// finalWorkspaceRefresh completes the workspace side of a turn before the
// lifecycle manager releases runtime interest. That ordering leaves the
// cached Git/file state current when the tracker transitions to idle polling.
func (m *Manager) finalWorkspaceRefresh(execution *AgentExecution, trigger string) {
	if execution == nil {
		return
	}
	client, releaseClient := execution.AcquireAgentCtlClient()
	if client == nil {
		return
	}
	defer releaseClient()
	timeout := finalWorkspaceRefreshTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := client.RefreshWorkspace(ctx, trigger); err != nil && m.logger != nil {
		m.logger.Warn("final workspace refresh failed",
			zap.String("operation", "workspace.final_refresh"),
			zap.String("trigger", trigger),
			zap.String("execution_id", execution.ID),
			zap.String("task_id", execution.TaskID),
			zap.String("session_id", execution.SessionID),
			zap.Error(err))
	}
}

func (m *Manager) finishExecutionWorkspaceActivity(execution *AgentExecution, trigger string) {
	if execution == nil {
		return
	}
	m.finalWorkspaceRefresh(execution, trigger)
	m.releaseActivity(executionActivityKey(execution.ID))
	m.setRuntimeInterest(execution.SessionID, false)
}
