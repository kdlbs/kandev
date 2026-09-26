package lifecycle

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// RegisterExecutorBackend adds a runtime backend before the manager starts.
func (m *Manager) RegisterExecutorBackend(backend ExecutorBackend) {
	if backend == nil || m.executorRegistry == nil {
		return
	}
	m.executorRegistry.Register(backend)
}

// getExecutorBackend returns the appropriate runtime for the given executor type.
// If the executor type is empty or the runtime is not available, behavior depends on executorFallbackPolicy.
func (m *Manager) getExecutorBackend(executorType string) (ExecutorBackend, error) {
	if m.executorRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}

	if executorType != "" {
		runtimeName := executor.ExecutorTypeToBackend(models.ExecutorType(executorType))
		rt, err := m.executorRegistry.GetBackend(runtimeName)
		if err == nil {
			return rt, nil
		}
		if runtimeName == executor.NameUnknown || runtimeName == executor.NamePluginRemote {
			return nil, fmt.Errorf("required executor runtime %q is unavailable: %w", runtimeName, err)
		}

		// Handle fallback based on policy
		switch m.executorFallbackPolicy {
		case ExecutorFallbackDeny:
			return nil, fmt.Errorf("runtime %s not available and fallback is denied: %w", runtimeName, err)
		case ExecutorFallbackWarn:
			m.logger.Warn("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)),
				zap.Error(err))
		case ExecutorFallbackAllow:
			m.logger.Debug("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)))
		default:
			// Default to warn behavior for backwards compatibility
			m.logger.Warn("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)),
				zap.Error(err))
		}
	}

	return m.executorRegistry.GetBackend(executor.NameStandalone)
}

// getDefaultExecutorBackend returns the default runtime (standalone).
// This is used when no executor type is specified.
func (m *Manager) getDefaultExecutorBackend() (ExecutorBackend, error) {
	if m.executorRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}
	return m.executorRegistry.GetBackend(executor.NameStandalone)
}
