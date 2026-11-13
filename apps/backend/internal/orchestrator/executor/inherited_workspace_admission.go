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
	var err error
	cfg, err = e.resolveImplicitExecutorForEnvironment(ctx, env, cfg)
	if err != nil {
		return err
	}
	if env != nil && cfg.ExecutorType == "" {
		return fmt.Errorf("%w: selected executor type could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	if env != nil && env.TaskID == task.ID {
		environmentType, known := env.EffectiveExecutorType()
		if !known {
			return fmt.Errorf("%w: canonical task environment executor could not be verified", models.ErrWorkspaceReuseUnsafe)
		}
		if string(environmentType) != cfg.ExecutorType {
			return fmt.Errorf("%w: canonical task environment uses executor %q; selected executor is %q",
				models.ErrWorkspaceExecutorMismatch, environmentType, cfg.ExecutorType)
		}
	}
	return rejectInheritedEnvironmentExecutorMismatch(task, env, cfg.ExecutorType, true)
}

func (e *Executor) resolveImplicitExecutorForEnvironment(
	ctx context.Context,
	environment *models.TaskEnvironment,
	config executorConfig,
) (executorConfig, error) {
	if !config.ImplicitSelection || environment == nil {
		return config, nil
	}
	environmentType, known := environment.EffectiveExecutorType()
	if !known {
		return executorConfig{}, fmt.Errorf("%w: inherited environment executor could not be verified", models.ErrWorkspaceReuseUnsafe)
	}
	config.ExecutorType = string(environmentType)
	if environment.ExecutorID == "" && environment.ExecutorProfileID == "" {
		return config, nil
	}
	metadata := cloneMetadata(config.Metadata)
	if environment.ExecutorProfileID != "" {
		if metadata == nil {
			metadata = make(map[string]interface{})
		}
		metadata["executor_profile_id"] = environment.ExecutorProfileID
	}
	resolved := e.resolveExecutorConfig(ctx, environment.ExecutorID, "", metadata)
	if resolved.ExecutorType == "" || resolved.ExecutorType != string(environmentType) {
		return executorConfig{}, fmt.Errorf("%w: inherited environment executor identity is inconsistent", models.ErrWorkspaceReuseUnsafe)
	}
	resolved.ImplicitSelection = true
	return resolved, nil
}
