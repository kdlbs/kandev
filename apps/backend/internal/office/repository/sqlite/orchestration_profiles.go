package sqlite

import (
	"context"
	orchestrationstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
)

type OrchestratedTask = orchestrationstore.OrchestratedTask

func (r *Repository) ExecutionProfileDirectory(ctx context.Context, workspaceID string) ([]map[string]string, error) {
	return r.OrchestrationStore().ExecutionProfileDirectory(ctx, workspaceID)
}
func (r *Repository) OrchestratedTasks(ctx context.Context, workspaceID, agentID string) ([]OrchestratedTask, error) {
	return r.OrchestrationStore().OrchestratedTasks(ctx, workspaceID, agentID)
}
