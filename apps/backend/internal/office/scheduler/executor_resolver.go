package scheduler

import (
	"context"

	"github.com/kandev/kandev/internal/office/service"
)

// ExecutorConfig represents resolved executor configuration.
// This is a type alias to service.ExecutorConfig for use within the scheduler.
type ExecutorConfig = service.ExecutorConfig

// ResolveExecutor walks the resolution chain to determine executor config.
// Delegates to service.Service.ResolveExecutor.
func (ss *SchedulerService) ResolveExecutor(
	ctx context.Context,
	taskExecutionPolicy, agentInstanceID, projectID, workspaceDefaultJSON string,
) (*ExecutorConfig, error) {
	return ss.svc.ResolveExecutor(ctx, taskExecutionPolicy, agentInstanceID, projectID, workspaceDefaultJSON)
}
