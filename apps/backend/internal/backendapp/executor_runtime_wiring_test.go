package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestRuntimeAdmissionWiring proves the production adapter satisfies the
// orchestrator's required-runtime capability and delegates to the lifecycle
// registry. Docker is absent from this registry, so the selection must be
// refused; the standalone alias must remain admitted.
func TestRuntimeAdmissionWiring(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	registry := lifecycle.NewExecutorRegistry(log)
	registry.Register(lifecycle.NewStandaloneExecutor(nil, "", 0, log))
	manager := lifecycle.NewManager(
		nil, bus.NewMemoryEventBus(log), registry, nil, nil, nil,
		lifecycle.ExecutorFallbackDeny, "", log,
	)
	adapter := newLifecycleAdapter(manager, nil, log)

	var _ executor.ExecutorTypeCapabilities = adapter
	var _ executor.ExecutorExecutionRuntimeAdmission = adapter

	if err := adapter.ValidateExecutorRuntime(string(models.ExecutorTypeLocalDocker)); !errors.Is(err, lifecycle.ErrExecutorRuntimeUnavailable) {
		t.Fatalf("ValidateExecutorRuntime(local_docker) = %v, want unavailable", err)
	}
	for _, selection := range []string{"", "local", "local_pc", "worktree", "mock_remote"} {
		if err := adapter.ValidateExecutorRuntime(selection); err != nil {
			t.Fatalf("ValidateExecutorRuntime(%q) = %v, want admitted", selection, err)
		}
	}
	if err := adapter.ValidateExecutionRuntime(context.Background(), "", "", ""); err != nil {
		t.Fatalf("empty retained check = %v, want admitted", err)
	}
}
