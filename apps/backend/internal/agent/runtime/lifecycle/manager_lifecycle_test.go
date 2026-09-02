package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestManager_MarkCompleted_Success(t *testing.T) {
	mgr := newTestManager(t)

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
	mgr := newTestManager(t)

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
	mgr := newTestManager(t)

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
	mgr := newTestManager(t)

	err := mgr.MarkCompleted("non-existent", 0, "")
	if err == nil {
		t.Error("expected error for non-existent execution")
	}
}

func TestManager_RemoveExecution(t *testing.T) {
	mgr := newTestManager(t)

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
	mgr := newTestManager(t)

	err := mgr.CleanupStaleExecutionBySessionID(context.Background(), "non-existent")
	if err != nil {
		t.Fatalf("expected nil error for non-existent session, got: %v", err)
	}
}

func TestManager_CleanupStaleExecution_SkipsStopWhenNoRuntime(t *testing.T) {
	mgr := newTestManager(t) // no executor registry

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

func TestManager_CleanupStaleExecution_PreservesTrackingOnStopError(t *testing.T) {
	log := newTestRegistryLogger()
	reg := newTestRegistry()
	execRegistry := NewExecutorRegistry(log)
	mock := &mockStopTracker{name: "standalone", stopErr: errors.New("runtime unavailable")}
	execRegistry.Register(mock)
	mgr := NewManager(reg, &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)

	execution := &AgentExecution{ID: "exec-error", SessionID: "session-error", RuntimeName: "standalone"}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("failed to seed execution: %v", err)
	}

	if err := mgr.CleanupStaleExecutionBySessionID(context.Background(), "session-error"); err == nil {
		t.Fatal("cleanup must report a runtime stop failure")
	}
	if _, found := mgr.GetExecutionBySessionID("session-error"); !found {
		t.Fatal("failed cleanup must retain execution tracking for a retry")
	}
}

// mockStopTracker is a minimal ExecutorBackend that records StopInstance calls.
type mockStopTracker struct {
	name              executor.Name
	stopCalled        bool
	stoppedInstanceID string
	stopReason        string
	stopErr           error
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
	m.stopReason = instance.StopReason
	return m.stopErr
}

func TestManagerStopAllAgentsPassesBackendShutdownReason(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	mock := &mockStopTracker{name: "standalone"}
	execRegistry.Register(mock)
	mgr := NewManager(nil, &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	if err := mgr.executionStore.Add(&AgentExecution{
		ID:          "exec-1",
		SessionID:   "session-1",
		RuntimeName: "standalone",
	}); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.StopAllAgents(context.Background()); err != nil {
		t.Fatalf("StopAllAgents: %v", err)
	}
	if mock.stopReason != StopReasonBackendShutdown {
		t.Fatalf("StopInstance reason = %q, want %q", mock.stopReason, StopReasonBackendShutdown)
	}
}

func TestManagerStopAllAgentsDetachesTaskHostWithoutStoppingRuntime(t *testing.T) {
	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	mock := &mockStopTracker{name: "standalone"}
	execRegistry.Register(mock)
	mgr := NewManager(nil, &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	host := &AgentExecution{
		ID: "task-host-1", TaskID: "task-1", TaskEnvironmentID: "env-1",
		RuntimeName: "standalone", IsTaskHost: true,
	}
	if err := mgr.executionStore.Add(host); err != nil {
		t.Fatal(err)
	}

	if err := mgr.StopAllAgents(context.Background()); err != nil {
		t.Fatal(err)
	}
	if mock.stopCalled {
		t.Fatal("backend shutdown stopped task-host runtime instead of preserving it for reattachment")
	}
	if _, exists := mgr.executionStore.Get(host.ID); exists {
		t.Fatal("detached task host remains in backend memory")
	}
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
	mgr := newTestManager(t)

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

// TestManager_StartSeedsRecoveredExecution verifies the recovery path itself,
// not only the readiness helper. A recovered agentctl never passes through
// waitForAgentctlReady, so startup must seed its base-branch map before stream
// reconnection begins.
func TestManager_StartSeedsRecoveredExecution(t *testing.T) {
	log := newTestRegistryLogger()
	branchesCh := make(chan map[string]string, 1)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/workspace/base-branches" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			BaseBranches map[string]string `json:"base_branches"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		branchesCh <- body.BaseBranches
		w.WriteHeader(http.StatusOK)
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		_ = server.Close()
	})

	port := listener.Addr().(*net.TCPAddr).Port
	client := agentctl.NewClient("127.0.0.1", port, log)
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(&MockExecutor{
		name: executor.NameStandalone,
		recoverInstances: []*ExecutorInstance{{
			InstanceID: "exec-recovered",
			TaskID:     "task-recovered",
			Client:     client,
		}},
	})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
	t.Cleanup(func() { _ = mgr.Stop() })
	mgr.SetBaseBranchProvider(func(context.Context, string) (map[string]string, error) {
		return map[string]string{"frontend": "develop"}, nil
	})

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case got := <-branchesCh:
		if got["frontend"] != "develop" {
			t.Fatalf("recovered base branch = %q, want develop; map=%v", got["frontend"], got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recovered execution base-branch seed")
	}
}
