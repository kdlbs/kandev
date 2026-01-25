// Package lifecycle provides agent runtime abstractions.
package lifecycle

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/common/logger"
)

// ErrRuntimeNotFound is returned when a runtime doesn't exist in the registry.
var ErrRuntimeNotFound = fmt.Errorf("runtime not found")

// RuntimeRegistry manages multiple Runtime implementations and provides
// thread-safe access to them. It supports registering runtimes by name,
// health checking, and instance recovery across all registered runtimes.
type RuntimeRegistry struct {
	runtimes map[runtime.Name]Runtime
	mu       sync.RWMutex
	logger   *logger.Logger
}

// NewRuntimeRegistry creates a new RuntimeRegistry with the given logger.
func NewRuntimeRegistry(log *logger.Logger) *RuntimeRegistry {
	return &RuntimeRegistry{
		runtimes: make(map[runtime.Name]Runtime),
		logger:   log,
	}
}

// Register adds a runtime to the registry using its Name() as the key.
// If a runtime with the same name already exists, it will be replaced.
func (r *RuntimeRegistry) Register(rt Runtime) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := rt.Name()
	r.runtimes[name] = rt
	r.logger.Info("registered runtime", zap.String("name", string(name)))
}

// GetRuntime returns a runtime by its name.
// Returns ErrRuntimeNotFound if the runtime doesn't exist.
func (r *RuntimeRegistry) GetRuntime(name runtime.Name) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rt, exists := r.runtimes[name]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrRuntimeNotFound, name)
	}
	return rt, nil
}

// List returns the names of all registered runtimes.
func (r *RuntimeRegistry) List() []runtime.Name {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]runtime.Name, 0, len(r.runtimes))
	for name := range r.runtimes {
		names = append(names, name)
	}
	return names
}

// HealthCheckAll performs health checks on all registered runtimes.
// Returns a map of runtime names to errors. A nil error indicates the runtime is healthy.
func (r *RuntimeRegistry) HealthCheckAll(ctx context.Context) map[runtime.Name]error {
	r.mu.RLock()
	runtimes := make(map[runtime.Name]Runtime, len(r.runtimes))
	for name, rt := range r.runtimes {
		runtimes[name] = rt
	}
	r.mu.RUnlock()

	results := make(map[runtime.Name]error, len(runtimes))
	for name, rt := range runtimes {
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

// RecoverAll recovers instances from all registered runtimes.
// Returns all recovered instances and any error encountered.
// If multiple runtimes fail, only the last error is returned.
func (r *RuntimeRegistry) RecoverAll(ctx context.Context) ([]*RuntimeInstance, error) {
	r.mu.RLock()
	runtimes := make(map[runtime.Name]Runtime, len(r.runtimes))
	for name, rt := range r.runtimes {
		runtimes[name] = rt
	}
	r.mu.RUnlock()

	var allInstances []*RuntimeInstance
	var lastErr error

	for name, rt := range runtimes {
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
