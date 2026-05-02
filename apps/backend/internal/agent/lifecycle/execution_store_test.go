package lifecycle

import (
	"errors"
	"testing"
)

// TestExecutionStore_AddRejectsDuplicateSession is the regression test for the
// process-leak bug where two paths created executions for the same session
// concurrently and Add silently overwrote the bySession index, orphaning the
// first execution's agent subprocess.
func TestExecutionStore_AddRejectsDuplicateSession(t *testing.T) {
	store := NewExecutionStore()

	first := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(first); err != nil {
		t.Fatalf("first Add: unexpected error: %v", err)
	}

	second := &AgentExecution{ID: "exec-2", SessionID: "session-1"}
	err := store.Add(second)
	if !errors.Is(err, ErrExecutionAlreadyExistsForSession) {
		t.Fatalf("second Add: want ErrExecutionAlreadyExistsForSession, got %v", err)
	}

	got, ok := store.GetBySessionID("session-1")
	if !ok {
		t.Fatalf("GetBySessionID: not found")
	}
	if got.ID != "exec-1" {
		t.Errorf("bySession index: want exec-1, got %s (overwrite was supposed to be rejected)", got.ID)
	}
	// Second execution must not be in the executions map either — otherwise
	// it'd live as an unreachable orphan.
	if _, ok := store.Get("exec-2"); ok {
		t.Errorf("Get(exec-2): rejected execution must not be tracked")
	}
}

func TestExecutionStore_AddSameExecutionTwiceIsIdempotent(t *testing.T) {
	store := NewExecutionStore()

	exec := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(exec); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	if err := store.Add(exec); err != nil {
		t.Errorf("re-adding the SAME execution must be a no-op, got %v", err)
	}
}

func TestExecutionStore_AddReplaceAfterRemove(t *testing.T) {
	store := NewExecutionStore()

	first := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
	if err := store.Add(first); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	store.Remove("exec-1")

	second := &AgentExecution{ID: "exec-2", SessionID: "session-1"}
	if err := store.Add(second); err != nil {
		t.Errorf("Add after Remove must succeed, got %v", err)
	}
	got, _ := store.GetBySessionID("session-1")
	if got == nil || got.ID != "exec-2" {
		t.Errorf("after Remove+Add: want exec-2, got %v", got)
	}
}

func TestExecutionStore_AddNoSessionIDAlwaysSucceeds(t *testing.T) {
	store := NewExecutionStore()

	if err := store.Add(&AgentExecution{ID: "exec-a"}); err != nil {
		t.Errorf("Add without SessionID: %v", err)
	}
	if err := store.Add(&AgentExecution{ID: "exec-b"}); err != nil {
		t.Errorf("Add without SessionID (second): %v", err)
	}
}
