// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/common/logger"
)

// ErrExecutorNotFound is returned when a runtime doesn't exist in the registry.
var ErrExecutorNotFound = fmt.Errorf("runtime not found")

// ExecutorRegistry manages multiple Runtime implementations and provides
// thread-safe access to them. It supports registering runtimes by name,
// health checking, and instance recovery across all registered runtimes.
type ExecutorRegistry struct {
	backends map[executor.Name]ExecutorBackend
	mu       sync.RWMutex
	logger   *logger.Logger
}

// NewExecutorRegistry creates a new ExecutorRegistry with the given logger.
func NewExecutorRegistry(log *logger.Logger) *ExecutorRegistry {
	return &ExecutorRegistry{
		backends: make(map[executor.Name]ExecutorBackend),
		logger:   log,
	}
}

// Register adds a runtime to the registry using its Name() as the key.
// If a runtime with the same name already exists, it will be replaced.
func (r *ExecutorRegistry) Register(rt ExecutorBackend) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := rt.Name()
	r.backends[name] = rt
	r.logger.Info("registered runtime", zap.String("name", string(name)))
}

// GetRuntime returns a runtime by its name.
// Returns ErrExecutorNotFound if the runtime doesn't exist.
func (r *ExecutorRegistry) GetBackend(name executor.Name) (ExecutorBackend, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, exists := r.backends[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrExecutorNotFound, name)
	}
	return rt, nil
}

// List returns the names of all registered runtimes.
func (r *ExecutorRegistry) List() []executor.Name {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]executor.Name, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	return names
}

// HealthCheckAll performs health checks on all registered runtimes.
// Returns a map of runtime names to errors. A nil error indicates the runtime is healthy.
func (r *ExecutorRegistry) HealthCheckAll(ctx context.Context) map[executor.Name]error {
	r.mu.RLock()
	backends := make(map[executor.Name]ExecutorBackend, len(r.backends))
	for name, rt := range r.backends {
		backends[name] = rt
	}
	r.mu.RUnlock()

	results := make(map[executor.Name]error, len(backends))
	for name, rt := range backends {
		err := rt.HealthCheck(ctx)
		results[name] = err
		if err != nil {
			r.logger.Warn("runtime health check failed",
				zap.String("runtime", string(name)),
				zap.Error(err))
		} else {
			r.logger.Debug("runtime health check passed",
				zap.String("runtime", string(name)))
		}
	}
	return results
}

// Closeable is an optional interface for executors that hold resources
// requiring cleanup on shutdown (e.g., Docker SDK client connections).
type Closeable interface {
	Close() error
}

// CloseAll closes all registered backends that implement Closeable.
func (r *ExecutorRegistry) CloseAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, rt := range r.backends {
		if closer, ok := rt.(Closeable); ok {
			if err := closer.Close(); err != nil {
				r.logger.Warn("failed to close runtime",
					zap.String("runtime", string(name)),
					zap.Error(err))
			}
		}
	}
}

// RecoverAll recovers instances from all registered runtimes.
// Returns all recovered instances and any error encountered.
// If multiple runtimes fail, only the last error is returned.
func (r *ExecutorRegistry) RecoverAll(ctx context.Context) ([]*ExecutorInstance, error) {
	r.mu.RLock()
	backends := make(map[executor.Name]ExecutorBackend, len(r.backends))
	for name, rt := range r.backends {
		backends[name] = rt
	}
	r.mu.RUnlock()

	var allInstances []*ExecutorInstance
	var lastErr error

	for name, rt := range backends {
		instances, err := rt.RecoverInstances(ctx)
		if err != nil {
			r.logger.Error("failed to recover instances from runtime",
				zap.String("runtime", string(name)),
				zap.Error(err))
			lastErr = err
			continue
		}

		if len(instances) > 0 {
			r.logger.Info("recovered instances from runtime",
				zap.String("runtime", string(name)),
				zap.Int("count", len(instances)))
			allInstances = append(allInstances, instances...)
		}
	}

	return allInstances, lastErr
}
