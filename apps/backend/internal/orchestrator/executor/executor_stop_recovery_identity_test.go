package executor

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/task/models"
)

// TestStopByTaskID_DoesNotStopReplacementExecution reproduces the registry
// recovery race: the execution seen during recovery is replaced before the
// stop path resolves the session again. Recovery must keep the original
// execution identity instead of stopping the replacement.
func TestStopByTaskID_DoesNotStopReplacementExecution(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateFailed,
	}

	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{{SessionID: "session-1", ExecutionID: "execution-original"}}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			// Model the old execution being removed and a replacement being
			// registered before the recovery path performs its fresh lookup.
			return "execution-replacement", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopByTaskID: %v", err)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-original" {
			t.Fatalf("stopped execution = %q, want execution-original", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registry-recovered execution was not stopped")
	}
}

// TestStopByTaskID_UnloadableRecoveryDoesNotStopReplacementExecution covers
// the same identity race when the session row cannot be read. The captured
// execution must remain reachable even when the session index now points at a
// replacement.
func TestStopByTaskID_UnloadableRecoveryDoesNotStopReplacementExecution(t *testing.T) {
	loadFailure := errors.New("database is locked")
	repo := newMockRepository()
	repo.getTaskSessionFunc = func(context.Context, string) (*models.TaskSession, error) {
		return nil, loadFailure
	}

	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{{SessionID: "session-1", ExecutionID: "execution-original"}}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-replacement", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true)
	if !errors.Is(err, loadFailure) {
		t.Fatalf("StopByTaskID error = %v, want it to wrap %v", err, loadFailure)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-original" {
			t.Fatalf("stopped execution = %q, want execution-original", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registry-recovered execution was not stopped")
	}
}

// TestStopByTaskID_CancelsSessionThatBecomesActiveDuringRecovery models a
// session that changes from terminal to active after the initial recovery
// read. When the captured execution still owns it, recovery must use the
// normal CANCELLED transition before scheduling teardown.
func TestStopByTaskID_CancelsSessionThatBecomesActiveDuringRecovery(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateFailed,
	}
	var reads atomic.Int32
	repo.getTaskSessionFunc = func(_ context.Context, sessionID string) (*models.TaskSession, error) {
		read := reads.Add(1)
		switch read {
		case 1:
			return &models.TaskSession{
				ID: sessionID, TaskID: "task-1", State: models.TaskSessionStateFailed,
			}, nil
		case 2, 3:
			return &models.TaskSession{
				ID: sessionID, TaskID: "task-1", State: models.TaskSessionStateRunning,
			}, nil
		default:
			repo.mu.Lock()
			defer repo.mu.Unlock()
			return cloneMockTaskSession(repo.sessions[sessionID]), nil
		}
	}

	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		listExecutionsForTaskFunc: func(taskID string) []lifecycle.ExecutionReference {
			if taskID != "task-1" {
				return nil
			}
			return []lifecycle.ExecutionReference{{SessionID: "session-1", ExecutionID: "execution-original"}}
		},
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-original", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.StopByTaskID(context.Background(), "task-1", "cleanup", true); err != nil {
		t.Fatalf("StopByTaskID: %v", err)
	}
	select {
	case executionID := <-stopCalls:
		if executionID != "execution-original" {
			t.Fatalf("stopped execution = %q, want execution-original", executionID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("registry-recovered execution was not stopped")
	}

	repo.mu.Lock()
	state := repo.sessions["session-1"].State
	repo.mu.Unlock()
	if state != models.TaskSessionStateCancelled {
		t.Fatalf("session state = %q, want CANCELLED", state)
	}
}
