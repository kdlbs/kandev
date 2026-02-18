package lifecycle

import (
	"context"
	"testing"
	"time"

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
