package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

type promoteCall struct {
	sessionID string
	status    string
}

type fakeRunningWriter struct {
	promoted []promoteCall
	err      error
}

func (f *fakeRunningWriter) UpsertExecutorRunning(context.Context, *models.ExecutorRunning) error {
	return nil
}
func (f *fakeRunningWriter) DeleteExecutorRunningBySessionID(context.Context, string) error {
	return nil
}
func (f *fakeRunningWriter) UpdateExecutorRunningStatus(_ context.Context, sessionID, status string) error {
	f.promoted = append(f.promoted, promoteCall{sessionID, status})
	return f.err
}

// TestPromoteExecutorRunningReadyHelper verifies the manager helper promotes the
// row to status='ready', is a safe no-op when no writer is wired or the session
// id is empty, and swallows a not-found row without failing the ready transition.
func TestPromoteExecutorRunningReadyHelper(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})

	fake := &fakeRunningWriter{}
	m := &Manager{logger: log, runningWriter: fake}
	exec := &AgentExecution{SessionID: "s1"}

	m.promoteExecutorRunningReady(context.Background(), exec)
	if len(fake.promoted) != 1 {
		t.Fatalf("expected 1 promote call, got %d", len(fake.promoted))
	}
	if got := fake.promoted[0]; got.sessionID != "s1" || got.status != models.ExecutorRunningStatusReady {
		t.Fatalf("promote call = %+v, want {s1 ready}", got)
	}

	// No writer wired (tests / early boot) → no-op, no panic.
	(&Manager{logger: log}).promoteExecutorRunningReady(context.Background(), exec)

	// Empty session id → skipped.
	fake.promoted = nil
	m.promoteExecutorRunningReady(context.Background(), &AgentExecution{})
	if len(fake.promoted) != 0 {
		t.Fatalf("empty session should not promote, got %d calls", len(fake.promoted))
	}

	// A not-found row must be swallowed (the row may have been torn down
	// concurrently) — the helper must not panic or propagate.
	fake.err = fmt.Errorf("%w for session: s1", models.ErrExecutorRunningNotFound)
	m.promoteExecutorRunningReady(context.Background(), exec)
}
