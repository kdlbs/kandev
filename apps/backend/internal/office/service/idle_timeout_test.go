package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/service"
)

func TestIdleTimeout_TerminalTaskStartsTimer(t *testing.T) {
	svc := newTestService(t)

	// Insert a completed task.
	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, created_at, updated_at)
		VALUES ('task-done', 'ws-1', 'COMPLETED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	mgr := service.NewIdleTimeoutManager(svc, 50*time.Millisecond)
	mgr.OnRunFinished(t.Context(), "sess-1", "task-done")

	if mgr.PendingCount() != 1 {
		t.Fatalf("expected 1 pending timer, got %d", mgr.PendingCount())
	}

	// Wait for the timer to fire.
	time.Sleep(150 * time.Millisecond)

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers after cleanup, got %d", mgr.PendingCount())
	}
}

func TestIdleTimeout_ViewerConnectedCancelsTimer(t *testing.T) {
	svc := newTestService(t)

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, created_at, updated_at)
		VALUES ('task-cancelled', 'ws-1', 'CANCELLED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	mgr := service.NewIdleTimeoutManager(svc, 1*time.Second)
	mgr.OnRunFinished(t.Context(), "sess-2", "task-cancelled")

	if mgr.PendingCount() != 1 {
		t.Fatalf("expected 1 pending timer, got %d", mgr.PendingCount())
	}

	// Viewer connects -- should cancel the timer.
	mgr.OnViewerConnected("sess-2")

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers after viewer connected, got %d", mgr.PendingCount())
	}
}

func TestIdleTimeout_NonTerminalTaskNoTimer(t *testing.T) {
	svc := newTestService(t)

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, created_at, updated_at)
		VALUES ('task-progress', 'ws-1', 'IN_PROGRESS', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	mgr := service.NewIdleTimeoutManager(svc, 50*time.Millisecond)
	mgr.OnRunFinished(t.Context(), "sess-3", "task-progress")

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers for non-terminal task, got %d", mgr.PendingCount())
	}
}

func TestIdleTimeout_ViewerDisconnectedTerminal(t *testing.T) {
	svc := newTestService(t)

	mgr := service.NewIdleTimeoutManager(svc, 50*time.Millisecond)
	mgr.OnViewerDisconnected("sess-4", true)

	if mgr.PendingCount() != 1 {
		t.Fatalf("expected 1 pending timer, got %d", mgr.PendingCount())
	}

	// Wait for cleanup.
	time.Sleep(150 * time.Millisecond)

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers after cleanup, got %d", mgr.PendingCount())
	}
}

func TestIdleTimeout_ViewerDisconnectedNonTerminal(t *testing.T) {
	mgr := service.NewIdleTimeoutManager(newTestService(t), 50*time.Millisecond)
	mgr.OnViewerDisconnected("sess-5", false)

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers for non-terminal disconnect, got %d", mgr.PendingCount())
	}
}

func TestIdleTimeout_EmptySessionOrTaskIgnored(t *testing.T) {
	mgr := service.NewIdleTimeoutManager(newTestService(t), 50*time.Millisecond)

	mgr.OnRunFinished(t.Context(), "", "task-1")
	mgr.OnRunFinished(t.Context(), "sess-1", "")

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers for empty IDs, got %d", mgr.PendingCount())
	}
}

// TestIdleTimeout_CancelledContextDoesNotStartTimer verifies that when the
// caller's context is already cancelled, the task-state lookup short-circuits
// and the manager does not schedule an idle timer. Without ctx propagation
// the lookup would run against context.Background() and could block on a
// stalled database before falsely tripping (or not tripping) the timer.
//
// The elapsed-time assertion guards against a future refactor that drops
// ctx propagation but happens to fall through some other no-timer path:
// in that regression the lookup would either still run to completion or
// hit the 5s isTaskTerminalLookupTimeout, both of which take far longer
// than this bound.
func TestIdleTimeout_CancelledContextDoesNotStartTimer(t *testing.T) {
	svc := newTestService(t)

	svc.ExecSQL(t, `INSERT INTO tasks (id, workspace_id, state, created_at, updated_at)
		VALUES ('task-ctx', 'ws-1', 'COMPLETED', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)

	mgr := service.NewIdleTimeoutManager(svc, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	mgr.OnRunFinished(ctx, "sess-cancelled", "task-ctx")
	elapsed := time.Since(start)

	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending timers when ctx is already cancelled, got %d",
			mgr.PendingCount())
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected prompt return when ctx is cancelled, elapsed=%v", elapsed)
	}
}
