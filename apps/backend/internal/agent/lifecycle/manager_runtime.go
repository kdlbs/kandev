package lifecycle

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

// getRuntimeForExecutorType returns the appropriate runtime for the given executor type.
// If the executor type is empty or the runtime is not available, behavior depends on runtimeFallbackPolicy.
func (m *Manager) getRuntimeForExecutorType(executorType string) (Runtime, error) {
	if m.runtimeRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}

	if executorType != "" {
		runtimeName := runtime.ExecutorTypeToRuntime(models.ExecutorType(executorType))
		rt, err := m.runtimeRegistry.GetRuntime(runtimeName)
		if err == nil {
			return rt, nil
		}

		// Handle fallback based on policy
		switch m.runtimeFallbackPolicy {
		case RuntimeFallbackDeny:
			return nil, fmt.Errorf("runtime %s not available and fallback is denied: %w", runtimeName, err)
		case RuntimeFallbackWarn:
			m.logger.Warn("requested runtime not available, falling back to default",
				zap.String("executor_type", executorType),
				zap.String("runtime", string(runtimeName)),
				zap.Error(err))
		case RuntimeFallbackAllow:
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

	return m.runtimeRegistry.GetRuntime(runtime.NameStandalone)
}

// getDefaultRuntime returns the default runtime (standalone).
// This is used when no executor type is specified.
func (m *Manager) getDefaultRuntime() (Runtime, error) {
	if m.runtimeRegistry == nil {
		return nil, fmt.Errorf("no runtime registry configured")
	}
	return m.runtimeRegistry.GetRuntime(runtime.NameStandalone)
}
