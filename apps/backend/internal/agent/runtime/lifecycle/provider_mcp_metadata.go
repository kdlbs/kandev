package lifecycle

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// A retained peer restores authority from its SQL execution owner, never from
// the preferences of the launch that discovered it.
func (m *Manager) restoreProviderMCPMetadata(ctx context.Context, execution *AgentExecution) error {
	reader, ok := m.runningWriter.(interface {
		GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error)
	})
	if !ok {
		return nil
	}
	running, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	if err != nil {
		return err
	}
	if running == nil || running.AgentExecutionID != execution.ID || running.TaskID != execution.TaskID {
		return models.ErrExecutionRotated
	}
	for key, value := range running.Metadata {
		if p.IsProviderMCPMetadataKey(key) {
			execution.setMetadataValue(key, value)
		}
	}
	return nil
}
