package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestStopWithSession_DoesNotOverwriteTerminalSession covers a session whose
// DB row already reached a terminal state (e.g. FAILED with a diagnostic
// error_message) while the lifecycle manager still reports a live execution
// for it — the shape observed when an SSH launch fails at the agentctl
// handshake after the in-memory execution was registered. The legacy
// stopWithSession path (used by orchestrator.Service.StopSession, including
// the async archive-cleanup stop) must not clobber that terminal row with
// CANCELLED / the stop reason; it must still tear down the leaked runtime.
func TestStopWithSession_DoesNotOverwriteTerminalSession(t *testing.T) {
	repo := newMockRepository()
	session := &models.TaskSession{
		ID:           "session-1",
		TaskID:       "task-1",
		State:        models.TaskSessionStateFailed,
		ErrorMessage: "ssh: agentctl handshake returned 403",
	}
	repo.sessions[session.ID] = session

	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-leaked", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.stopWithSession(context.Background(), session, "task archived", true); err != nil {
		t.Fatalf("stopWithSession: %v", err)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-leaked" {
			t.Fatalf("stopped execution = %q, want execution-leaked", executionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for leaked execution teardown")
	}

	got := repo.sessions[session.ID]
	if got.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %q, want FAILED to survive the stop", got.State)
	}
	if got.ErrorMessage != "ssh: agentctl handshake returned 403" {
		t.Fatalf("session error_message = %q, want the original diagnosis preserved", got.ErrorMessage)
	}
}

// TestStopWithSession_DoesNotOverwriteTerminalSession_StaleCallerSnapshot
// covers the freshness gap the guard above doesn't exercise: the caller's
// in-hand *models.TaskSession still shows RUNNING (read before a concurrent
// launch failure landed), but the DB row has already moved to FAILED by the
// time stopWithSession runs. A guard that trusts the caller-supplied
// snapshot instead of re-reading current state would see "not terminal" and
// clobber the FAILED row anyway.
func TestStopWithSession_DoesNotOverwriteTerminalSession_StaleCallerSnapshot(t *testing.T) {
	repo := newMockRepository()
	repo.sessions["session-1"] = &models.TaskSession{
		ID:           "session-1",
		TaskID:       "task-1",
		State:        models.TaskSessionStateFailed,
		ErrorMessage: "ssh: agentctl handshake returned 403",
	}
	staleSnapshot := &models.TaskSession{
		ID:     "session-1",
		TaskID: "task-1",
		State:  models.TaskSessionStateRunning,
	}

	stopCalls := make(chan string, 1)
	manager := &mockAgentManager{
		getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
			return "execution-leaked", nil
		},
		stopAgentWithReasonFunc: func(_ context.Context, executionID, _ string, _ bool) error {
			stopCalls <- executionID
			return nil
		},
	}
	exec := newTestExecutor(t, manager, repo)

	if err := exec.stopWithSession(context.Background(), staleSnapshot, "task archived", true); err != nil {
		t.Fatalf("stopWithSession: %v", err)
	}

	select {
	case executionID := <-stopCalls:
		if executionID != "execution-leaked" {
			t.Fatalf("stopped execution = %q, want execution-leaked", executionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for leaked execution teardown")
	}

	got := repo.sessions["session-1"]
	if got.State != models.TaskSessionStateFailed {
		t.Fatalf("session state = %q, want FAILED to survive the stop despite a stale RUNNING caller snapshot", got.State)
	}
	if got.ErrorMessage != "ssh: agentctl handshake returned 403" {
		t.Fatalf("session error_message = %q, want the original diagnosis preserved", got.ErrorMessage)
	}
}
