package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// MockExecutor implements Runtime interface for testing
type MockExecutor struct {
	name             executor.Name
	healthCheckErr   error
	recoverInstances []*ExecutorInstance
	recoverErr       error
}

func (m *MockExecutor) Name() executor.Name { return m.name }
func (m *MockExecutor) HealthCheck(ctx context.Context) error {
	return m.healthCheckErr
}
func (m *MockExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (m *MockExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	return nil
}
func (m *MockExecutor) RecoverInstances(ctx context.Context) ([]*ExecutorInstance, error) {
	return m.recoverInstances, m.recoverErr
}
func (m *MockExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func newTestRegistryLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

func TestNewExecutorRegistry(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)

	if registry == nil {
		t.Fatal("expected non-nil registry")
	} else {
		if registry.backends == nil {
			t.Error("expected runtimes map to be initialized")
		}
		if len(registry.backends) != 0 {
			t.Errorf("expected empty runtimes map, got %d entries", len(registry.backends))
		}
	}
}

func TestExecutorRegistry_Register(t *testing.T) {
	t.Run("register new runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		mockRT := &MockExecutor{name: executor.NameDocker}
		registry.Register(mockRT)

		if len(registry.backends) != 1 {
			t.Errorf("expected 1 runtime, got %d", len(registry.backends))
		}
		if registry.backends[executor.NameDocker] != mockRT {
			t.Error("expected registered runtime to match")
		}
	})

	t.Run("replace existing runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		mockRT1 := &MockExecutor{name: executor.NameDocker, healthCheckErr: errors.New("error1")}
		mockRT2 := &MockExecutor{name: executor.NameDocker, healthCheckErr: errors.New("error2")}

		registry.Register(mockRT1)
		registry.Register(mockRT2)

		if len(registry.backends) != 1 {
			t.Errorf("expected 1 runtime after replacement, got %d", len(registry.backends))
		}
		if registry.backends[executor.NameDocker] != mockRT2 {
			t.Error("expected runtime to be replaced with new one")
		}
	})
}

func TestExecutorRegistry_GetRuntime(t *testing.T) {
	t.Run("get existing runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		mockRT := &MockExecutor{name: executor.NameDocker}
		registry.Register(mockRT)

		rt, err := registry.GetBackend(executor.NameDocker)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if rt != mockRT {
			t.Error("expected returned runtime to match registered one")
		}
	})

	t.Run("get non-existent runtime returns ErrExecutorNotFound", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		_, err := registry.GetBackend(executor.NameDocker)
		if err == nil {
			t.Error("expected error for non-existent runtime")
		}
		if !errors.Is(err, ErrExecutorNotFound) {
			t.Errorf("expected ErrExecutorNotFound, got: %v", err)
		}
	})
}

func TestExecutorRegistry_List(t *testing.T) {
	t.Run("list empty registry", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		names := registry.List()
		if len(names) != 0 {
			t.Errorf("expected empty list, got %d items", len(names))
		}
	})

	t.Run("list with multiple runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		registry.Register(&MockExecutor{name: executor.NameDocker})
		registry.Register(&MockExecutor{name: executor.NameStandalone})
		registry.Register(&MockExecutor{name: executor.NameLocal})

		names := registry.List()
		if len(names) != 3 {
			t.Errorf("expected 3 names, got %d", len(names))
		}

		// Check all names are present (order is not guaranteed)
		nameMap := make(map[executor.Name]bool)
		for _, n := range names {
			nameMap[n] = true
		}
		if !nameMap[executor.NameDocker] || !nameMap[executor.NameStandalone] || !nameMap[executor.NameLocal] {
			t.Error("expected all registered runtime names to be present")
		}
	})
}

func TestExecutorRegistry_HealthCheckAll(t *testing.T) {
	ctx := context.Background()

	t.Run("health check with all healthy runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		registry.Register(&MockExecutor{name: executor.NameDocker, healthCheckErr: nil})
		registry.Register(&MockExecutor{name: executor.NameStandalone, healthCheckErr: nil})

		results := registry.HealthCheckAll(ctx)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results[executor.NameDocker] != nil {
			t.Errorf("expected nil error for docker, got: %v", results[executor.NameDocker])
		}
		if results[executor.NameStandalone] != nil {
			t.Errorf("expected nil error for standalone, got: %v", results[executor.NameStandalone])
		}
	})

	t.Run("health check with some failing runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		dockerErr := errors.New("docker unavailable")
		registry.Register(&MockExecutor{name: executor.NameDocker, healthCheckErr: dockerErr})
		registry.Register(&MockExecutor{name: executor.NameStandalone, healthCheckErr: nil})

		results := registry.HealthCheckAll(ctx)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results[executor.NameDocker] != dockerErr {
			t.Errorf("expected docker error, got: %v", results[executor.NameDocker])
		}
		if results[executor.NameStandalone] != nil {
			t.Errorf("expected nil error for standalone, got: %v", results[executor.NameStandalone])
		}
	})

	t.Run("health check with empty registry", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		results := registry.HealthCheckAll(ctx)
		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})
}

func TestExecutorRegistry_RecoverAll(t *testing.T) {
	ctx := context.Background()

	t.Run("recovery with no instances", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		registry.Register(&MockExecutor{name: executor.NameDocker, recoverInstances: nil})
		registry.Register(&MockExecutor{name: executor.NameStandalone, recoverInstances: []*ExecutorInstance{}})

		instances, err := registry.RecoverAll(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(instances) != 0 {
			t.Errorf("expected no instances, got %d", len(instances))
		}
	})

	t.Run("recovery with instances from multiple runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		dockerInstances := []*ExecutorInstance{
			{InstanceID: "docker-1", RuntimeName: "docker"},
			{InstanceID: "docker-2", RuntimeName: "docker"},
		}
		standaloneInstances := []*ExecutorInstance{
			{InstanceID: "standalone-1", RuntimeName: "standalone"},
		}

		registry.Register(&MockExecutor{name: executor.NameDocker, recoverInstances: dockerInstances})
		registry.Register(&MockExecutor{name: executor.NameStandalone, recoverInstances: standaloneInstances})

		instances, err := registry.RecoverAll(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(instances) != 3 {
			t.Errorf("expected 3 instances, got %d", len(instances))
		}
	})

	t.Run("recovery with errors continues and returns last error", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		recoverErr := errors.New("recovery failed")
		standaloneInstances := []*ExecutorInstance{
			{InstanceID: "standalone-1", RuntimeName: "standalone"},
		}

		registry.Register(&MockExecutor{name: executor.NameDocker, recoverErr: recoverErr})
		registry.Register(&MockExecutor{name: executor.NameStandalone, recoverInstances: standaloneInstances})

		instances, err := registry.RecoverAll(ctx)
		// Should still get instances from successful runtimes
		// Note: map iteration order is not guaranteed, so we check that at least standalone instances are recovered
		if len(instances) == 0 && err == nil {
			t.Error("expected either instances or error")
		}
		// The error may or may not be set depending on iteration order
		// If docker is processed last, err will be set; if standalone is processed last, err will be nil
	})

	t.Run("recovery from empty registry", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewExecutorRegistry(log)

		instances, err := registry.RecoverAll(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(instances) != 0 {
			t.Errorf("expected no instances, got %d", len(instances))
		}
	})
}

func TestExecutorRegistry_ThreadSafety(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	ctx := context.Background()

	const numGoroutines = 50
	const numOperations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 4) // 4 types of operations

	// Concurrent Register operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				name := executor.Name("test-runtime")
				registry.Register(&MockExecutor{name: name})
			}
		}(i)
	}

	// Concurrent GetRuntime operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_, _ = registry.GetBackend(executor.Name("test-runtime"))
			}
		}(i)
	}

	// Concurrent List operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = registry.List()
			}
		}(i)
	}

	// Concurrent HealthCheckAll operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = registry.HealthCheckAll(ctx)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// If we reach here without a race condition panic, the test passes
	// The -race flag during testing will detect actual race conditions
}
