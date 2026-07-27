package lifecycle

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ExecutorProfileReader loads the executor profile bound to a task environment.
// Implemented by the task repository; wired via SetExecutorProfileReader.
type ExecutorProfileReader interface {
	GetTaskEnvironment(ctx context.Context, id string) (*models.TaskEnvironment, error)
	GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error)
}

// SetExecutorProfileReader wires the reader used to expose executor-profile env
// vars to user shell terminals. Nil → terminals fall back to inheriting only the
// backend process environment.
func (m *Manager) SetExecutorProfileReader(reader ExecutorProfileReader) {
	m.executorProfileReader = reader
}

// ExecutorProfileEnvForEnvironment resolves the executor profile's env vars for a
// task environment, revealing secret-backed entries. It mirrors what the agent
// subprocess receives (the orchestrator merges the same profile env into the
// launch request), so a user shell terminal opened on the workspace sees the
// same tokens the agent and the repository setup script do.
//
// Resolution is best-effort: a missing reader, environment, profile, or secret
// yields the entries that could be resolved rather than failing the terminal.
func (m *Manager) ExecutorProfileEnvForEnvironment(ctx context.Context, taskEnvironmentID string) map[string]string {
	if taskEnvironmentID == "" || m.executorProfileReader == nil {
		return nil
	}
	env, err := m.executorProfileReader.GetTaskEnvironment(ctx, taskEnvironmentID)
	if err != nil {
		m.logger.Warn("failed to load task environment for terminal env",
			zap.String("task_environment_id", taskEnvironmentID),
			zap.Error(err))
		return nil
	}
	if env == nil || env.ExecutorProfileID == "" {
		return nil
	}
	profile, err := m.executorProfileReader.GetExecutorProfile(ctx, env.ExecutorProfileID)
	if err != nil {
		m.logger.Warn("failed to load executor profile for terminal env",
			zap.String("task_environment_id", taskEnvironmentID),
			zap.String("executor_profile_id", env.ExecutorProfileID),
			zap.Error(err))
		return nil
	}
	if profile == nil || len(profile.EnvVars) == 0 {
		return nil
	}
	// ExecutorProfile.EnvVars and agent-profile env vars are the same type, so
	// the secret-revealing resolver is shared.
	return m.resolveAgentProfileEnvVars(ctx, profile.EnvVars)
}
