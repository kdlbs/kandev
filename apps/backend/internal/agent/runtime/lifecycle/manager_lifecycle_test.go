package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestManager_MarkCompleted_Success(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Mark as completed successfully (exit code 0)
	err := mgr.MarkCompleted("test-execution-id", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("expected status %v, got %v", v1.AgentStatusCompleted, got.Status)
	}
	if got.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
	if got.ExitCode == nil || *got.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %v", got.ExitCode)
	}
}

func TestManager_MarkCompleted_Failure(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		ContainerID:    "container-123",
		Status:         v1.AgentStatusRunning,
		StartedAt:      time.Now(),
	}

	mgr.executionStore.Add(execution)

	// Mark as failed
	err := mgr.MarkCompleted("test-execution-id", 1, "process failed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
	if got.Status != v1.AgentStatusFailed {
		t.Errorf("expected status %v, got %v", v1.AgentStatusFailed, got.Status)
	}
	if got.ErrorMessage != "process failed" {
		t.Errorf("expected error message 'process failed', got %q", got.ErrorMessage)
	}
	if got.ExitCode == nil || *got.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %v", got.ExitCode)
	}
}

func TestManager_MarkCompleted_Idempotent(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:             "test-execution-id",
		TaskID:         "test-task-id",
		AgentProfileID: "test-agent",
		Status:         v1.AgentStatusFailed,
		StartedAt:      time.Now(),
	}
	exitCode := 1
	execution.ExitCode = &exitCode
	execution.ErrorMessage = "first error"

	mgr.executionStore.Add(execution)

	// Second MarkCompleted should be a no-op (already terminal)
	err := mgr.MarkCompleted("test-execution-id", 1, "second error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, _ := mgr.GetExecution("test-execution-id")
	if got.ErrorMessage != "first error" {
		t.Errorf("expected original error message preserved, got %q", got.ErrorMessage)
	}
}

func TestManager_MarkCompleted_NotFound(t *testing.T) {
	mgr := newTestManager()

	err := mgr.MarkCompleted("non-existent", 0, "")
	if err == nil {
		t.Error("expected error for non-existent execution")
	}
}

func TestManager_RemoveExecution(t *testing.T) {
	mgr := newTestManager()

	execution := &AgentExecution{
		ID:          "test-execution-id",
		TaskID:      "test-task-id",
		SessionID:   "test-session-id",
		ContainerID: "container-123",
	}

	mgr.executionStore.Add(execution)

	// Remove execution
	mgr.RemoveExecution("test-execution-id")

	// Verify it's gone from all maps
	if _, found := mgr.GetExecution("test-execution-id"); found {
		t.Error("execution should be removed from executions map")
	}
	if _, found := mgr.GetExecutionBySessionID("test-session-id"); found {
		t.Error("execution should be removed from bySession map")
	}

	// Remove non-existent should not panic
	mgr.RemoveExecution("non-existent")
}

func TestManager_CleanupStaleExecution_StopsRuntimeInstance(t *testing.T) {
	log := newTestRegistryLogger()
	reg := newTestRegistry()
	eventBus := &MockEventBus{}
	credsMgr := &MockCredentialsManager{}
	profileResolver := &MockProfileResolver{}

	// Create executor registry with a mock backend that tracks StopInstance calls
	execRegistry := NewExecutorRegistry(log)
	mock := &mockStopTracker{name: "standalone"}
	execRegistry.Register(mock)

	mgr := NewManager(reg, eventBus, execRegistry, credsMgr, profileResolver, nil, ExecutorFallbackWarn, "", log)

	execution := &AgentExecution{
		ID:          "exec-1",
		SessionID:   "session-1",
		RuntimeName: "standalone",
	}
	mgr.executionStore.Add(execution)

	err := mgr.CleanupStaleExecutionBySessionID(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify StopInstance was called on the backend
	if !mock.stopCalled {
		t.Error("expected StopInstance to be called on the executor backend")
	}
	if mock.stoppedInstanceID != "exec-1" {
		t.Errorf("expected StopInstance with instance ID exec-1, got %q", mock.stoppedInstanceID)
	}

	// Verify execution was removed from store
	if _, found := mgr.GetExecutionBySessionID("session-1"); found {
		t.Error("expected execution to be removed from store")
	}
}

func TestManager_CleanupStaleExecution_NoopForMissingSession(t *testing.T) {
	mgr := newTestManager()

	err := mgr.CleanupStaleExecutionBySessionID(context.Background(), "non-existent")
	if err != nil {
		t.Fatalf("expected nil error for non-existent session, got: %v", err)
	}
}

func TestManager_CleanupStaleExecution_SkipsStopWhenNoRuntime(t *testing.T) {
	mgr := newTestManager() // no executor registry

	execution := &AgentExecution{
		ID:          "exec-1",
		SessionID:   "session-1",
		RuntimeName: "standalone",
	}
	mgr.executionStore.Add(execution)

	err := mgr.CleanupStaleExecutionBySessionID(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should still remove from store even without executor registry
	if _, found := mgr.GetExecutionBySessionID("session-1"); found {
		t.Error("expected execution to be removed from store")
	}
}

// mockStopTracker is a minimal ExecutorBackend that records StopInstance calls.
type mockStopTracker struct {
	name              executor.Name
	stopCalled        bool
	stoppedInstanceID string
}

func (m *mockStopTracker) Name() executor.Name { return m.name }
func (m *mockStopTracker) HealthCheck(ctx context.Context) error {
	return nil
}
func (m *mockStopTracker) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	return nil, nil
}
func (m *mockStopTracker) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	m.stopCalled = true
	m.stoppedInstanceID = instance.InstanceID
	return nil
}
func (m *mockStopTracker) RecoverInstances(ctx context.Context) ([]*ExecutorInstance, error) {
	return nil, nil
}
func (m *mockStopTracker) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}
func (m *mockStopTracker) RequiresCloneURL() bool          { return false }
func (m *mockStopTracker) ShouldApplyPreferredShell() bool { return false }
func (m *mockStopTracker) IsAlwaysResumable() bool         { return false }

func TestManager_StartStop(t *testing.T) {
	mgr := newTestManager()

	ctx := context.Background()

	// Test Start
	err := mgr.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error starting manager: %v", err)
	}

	// Test Stop
	err = mgr.Stop()
	if err != nil {
		t.Fatalf("unexpected error stopping manager: %v", err)
	}
}
