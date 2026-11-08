package executor

import (
	"context"
	"fmt"
	"maps"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// AdmitInheritedEnvironment uses the launch-time profile/default resolution
// without mutating task metadata or persisting a session.
func (e *Executor) AdmitInheritedEnvironment(ctx context.Context, task *v1.Task, env *models.TaskEnvironment, executorID, executorProfileID string) error {
	metadata := maps.Clone(task.Metadata)
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	if executorProfileID != "" {
		metadata["executor_profile_id"] = executorProfileID
	}
	cfg := e.resolveExecutorConfig(ctx, executorID, task.WorkspaceID, metadata)
	if env != nil && env.TaskID == task.ID {
		if env.ExecutorType == "" {
			return fmt.Errorf("%w: canonical task environment executor could not be verified", models.ErrWorkspaceReuseUnsafe)
		}
		if cfg.ExecutorType != "" && env.ExecutorType != cfg.ExecutorType {
			return fmt.Errorf("%w: canonical task environment uses executor %q; selected executor is %q",
				models.ErrWorkspaceExecutorMismatch, env.ExecutorType, cfg.ExecutorType)
		}
	}
	return rejectInheritedEnvironmentExecutorMismatch(task, env, cfg.ExecutorType, true)
}
