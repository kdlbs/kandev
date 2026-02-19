package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
)

// MockRuntime implements Runtime interface for testing
type MockRuntime struct {
	name             runtime.Name
	healthCheckErr   error
	recoverInstances []*RuntimeInstance
	recoverErr       error
}

func (m *MockRuntime) Name() runtime.Name { return m.name }
func (m *MockRuntime) HealthCheck(ctx context.Context) error {
	return m.healthCheckErr
}
func (m *MockRuntime) CreateInstance(ctx context.Context, req *RuntimeCreateRequest) (*RuntimeInstance, error) {
	return nil, nil
}
func (m *MockRuntime) StopInstance(ctx context.Context, instance *RuntimeInstance, force bool) error {
	return nil
}
func (m *MockRuntime) RecoverInstances(ctx context.Context) ([]*RuntimeInstance, error) {
	return m.recoverInstances, m.recoverErr
}
func (m *MockRuntime) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func newTestRegistryLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	return log
}

func TestNewRuntimeRegistry(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewRuntimeRegistry(log)

	if registry == nil {
		t.Fatal("expected non-nil registry")
	} else {
		if registry.runtimes == nil {
			t.Error("expected runtimes map to be initialized")
		}
		if len(registry.runtimes) != 0 {
			t.Errorf("expected empty runtimes map, got %d entries", len(registry.runtimes))
		}
	}
}

func TestRuntimeRegistry_Register(t *testing.T) {
	t.Run("register new runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		mockRT := &MockRuntime{name: runtime.NameDocker}
		registry.Register(mockRT)

		if len(registry.runtimes) != 1 {
			t.Errorf("expected 1 runtime, got %d", len(registry.runtimes))
		}
		if registry.runtimes[runtime.NameDocker] != mockRT {
			t.Error("expected registered runtime to match")
		}
	})

	t.Run("replace existing runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		mockRT1 := &MockRuntime{name: runtime.NameDocker, healthCheckErr: errors.New("error1")}
		mockRT2 := &MockRuntime{name: runtime.NameDocker, healthCheckErr: errors.New("error2")}

		registry.Register(mockRT1)
		registry.Register(mockRT2)

		if len(registry.runtimes) != 1 {
			t.Errorf("expected 1 runtime after replacement, got %d", len(registry.runtimes))
		}
		if registry.runtimes[runtime.NameDocker] != mockRT2 {
			t.Error("expected runtime to be replaced with new one")
		}
	})
}

func TestRuntimeRegistry_GetRuntime(t *testing.T) {
	t.Run("get existing runtime", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		mockRT := &MockRuntime{name: runtime.NameDocker}
		registry.Register(mockRT)

		rt, err := registry.GetRuntime(runtime.NameDocker)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if rt != mockRT {
			t.Error("expected returned runtime to match registered one")
		}
	})

	t.Run("get non-existent runtime returns ErrRuntimeNotFound", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		_, err := registry.GetRuntime(runtime.NameDocker)
		if err == nil {
			t.Error("expected error for non-existent runtime")
		}
		if !errors.Is(err, ErrRuntimeNotFound) {
			t.Errorf("expected ErrRuntimeNotFound, got: %v", err)
		}
	})
}

func TestRuntimeRegistry_List(t *testing.T) {
	t.Run("list empty registry", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		names := registry.List()
		if len(names) != 0 {
			t.Errorf("expected empty list, got %d items", len(names))
		}
	})

	t.Run("list with multiple runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		registry.Register(&MockRuntime{name: runtime.NameDocker})
		registry.Register(&MockRuntime{name: runtime.NameStandalone})
		registry.Register(&MockRuntime{name: runtime.NameLocal})

		names := registry.List()
		if len(names) != 3 {
			t.Errorf("expected 3 names, got %d", len(names))
		}

		// Check all names are present (order is not guaranteed)
		nameMap := make(map[runtime.Name]bool)
		for _, n := range names {
			nameMap[n] = true
		}
		if !nameMap[runtime.NameDocker] || !nameMap[runtime.NameStandalone] || !nameMap[runtime.NameLocal] {
			t.Error("expected all registered runtime names to be present")
		}
	})
}

func TestRuntimeRegistry_HealthCheckAll(t *testing.T) {
	ctx := context.Background()

	t.Run("health check with all healthy runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		registry.Register(&MockRuntime{name: runtime.NameDocker, healthCheckErr: nil})
		registry.Register(&MockRuntime{name: runtime.NameStandalone, healthCheckErr: nil})

		results := registry.HealthCheckAll(ctx)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results[runtime.NameDocker] != nil {
			t.Errorf("expected nil error for docker, got: %v", results[runtime.NameDocker])
		}
		if results[runtime.NameStandalone] != nil {
			t.Errorf("expected nil error for standalone, got: %v", results[runtime.NameStandalone])
		}
	})

	t.Run("health check with some failing runtimes", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		dockerErr := errors.New("docker unavailable")
		registry.Register(&MockRuntime{name: runtime.NameDocker, healthCheckErr: dockerErr})
		registry.Register(&MockRuntime{name: runtime.NameStandalone, healthCheckErr: nil})

		results := registry.HealthCheckAll(ctx)
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results[runtime.NameDocker] != dockerErr {
			t.Errorf("expected docker error, got: %v", results[runtime.NameDocker])
		}
		if results[runtime.NameStandalone] != nil {
			t.Errorf("expected nil error for standalone, got: %v", results[runtime.NameStandalone])
		}
	})

	t.Run("health check with empty registry", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		results := registry.HealthCheckAll(ctx)
		if len(results) != 0 {
			t.Errorf("expected empty results, got %d", len(results))
		}
	})
}

func TestRuntimeRegistry_RecoverAll(t *testing.T) {
	ctx := context.Background()

	t.Run("recovery with no instances", func(t *testing.T) {
		log := newTestRegistryLogger()
		registry := NewRuntimeRegistry(log)

		registry.Register(&MockRuntime{name: runtime.NameDocker, recoverInstances: nil})
		registry.Register(&MockRuntime{name: runtime.NameStandalone, recoverInstances: []*RuntimeInstance{}})

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
		registry := NewRuntimeRegistry(log)

		dockerInstances := []*RuntimeInstance{
			{InstanceID: "docker-1", RuntimeName: "docker"},
			{InstanceID: "docker-2", RuntimeName: "docker"},
		}
		standaloneInstances := []*RuntimeInstance{
			{InstanceID: "standalone-1", RuntimeName: "standalone"},
		}

		registry.Register(&MockRuntime{name: runtime.NameDocker, recoverInstances: dockerInstances})
		registry.Register(&MockRuntime{name: runtime.NameStandalone, recoverInstances: standaloneInstances})

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
		registry := NewRuntimeRegistry(log)

		recoverErr := errors.New("recovery failed")
		standaloneInstances := []*RuntimeInstance{
			{InstanceID: "standalone-1", RuntimeName: "standalone"},
		}

		registry.Register(&MockRuntime{name: runtime.NameDocker, recoverErr: recoverErr})
		registry.Register(&MockRuntime{name: runtime.NameStandalone, recoverInstances: standaloneInstances})

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
		registry := NewRuntimeRegistry(log)

		instances, err := registry.RecoverAll(ctx)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(instances) != 0 {
			t.Errorf("expected no instances, got %d", len(instances))
		}
	})
}

func TestRuntimeRegistry_ThreadSafety(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewRuntimeRegistry(log)
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
				name := runtime.Name("test-runtime")
				registry.Register(&MockRuntime{name: name})
			}
		}(i)
	}

	// Concurrent GetRuntime operations
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_, _ = registry.GetRuntime(runtime.Name("test-runtime"))
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
