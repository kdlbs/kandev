package lifecycle

import (
	"context"
	"errors"
	"testing"
)

// TestExecutionAccessChecksGateBeforeCache proves the per-user access checks
// added for the security-audit IDOR fixes run BEFORE the in-memory execution
// cache is consulted — a cached execution must not be reachable by a
// non-owner. Regression guard for the vscode/port/terminal IDORs.
func TestExecutionAccessChecksGateBeforeCache(t *testing.T) {
	denied := errors.New("denied")

	t.Run("EnsurePassthroughExecution honors sessionAccessCheck", func(t *testing.T) {
		mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
		// Seed a cached execution so a missing check would short-circuit to it.
		if err := mgr.executionStore.Add(&AgentExecution{ID: "ex-1", SessionID: "sess-1", PassthroughProcessID: "p1"}); err != nil {
			t.Fatal(err)
		}
		mgr.SetSessionAccessChecker(func(_ context.Context, sessionID string) error {
			if sessionID == "sess-1" {
				return denied
			}
			return nil
		})
		if _, err := mgr.EnsurePassthroughExecution(context.Background(), "sess-1"); !errors.Is(err, denied) {
			t.Fatalf("expected denial before cache hit, got %v", err)
		}
	})

	t.Run("GetOrEnsureExecutionForEnvironment honors environmentAccessCheck", func(t *testing.T) {
		mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
		if err := mgr.executionStore.Add(&AgentExecution{ID: "ex-2", SessionID: "sess-2", TaskEnvironmentID: "env-1"}); err != nil {
			t.Fatal(err)
		}
		mgr.SetEnvironmentAccessChecker(func(_ context.Context, envID string) error {
			if envID == "env-1" {
				return denied
			}
			return nil
		})
		if _, err := mgr.GetOrEnsureExecutionForEnvironment(context.Background(), "env-1"); !errors.Is(err, denied) {
			t.Fatalf("expected denial before cache hit, got %v", err)
		}
	})

	t.Run("CheckSessionAccess is a no-op without a checker", func(t *testing.T) {
		mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
		if err := mgr.CheckSessionAccess(context.Background(), "sess-x"); err != nil {
			t.Fatalf("nil checker must pass: %v", err)
		}
	})

	t.Run("CheckTaskAccess delegates to the installed checker", func(t *testing.T) {
		mgr := &Manager{logger: newTestLogger(), executionStore: NewExecutionStore()}
		if err := mgr.CheckTaskAccess(context.Background(), "task-x"); err != nil {
			t.Fatalf("nil checker must pass: %v", err)
		}
		mgr.SetTaskAccessChecker(func(_ context.Context, taskID string) error {
			if taskID == "task-1" {
				return denied
			}
			return nil
		})
		if err := mgr.CheckTaskAccess(context.Background(), "task-1"); !errors.Is(err, denied) {
			t.Fatalf("expected denial, got %v", err)
		}
		if err := mgr.CheckTaskAccess(context.Background(), "task-2"); err != nil {
			t.Fatalf("unexpected denial: %v", err)
		}
	})
}
