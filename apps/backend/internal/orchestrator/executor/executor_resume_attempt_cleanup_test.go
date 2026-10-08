package executor

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestStopFailedStartExecutionIfCurrentAttemptHonorsCleanupClaim(t *testing.T) {
	for _, tc := range []struct {
		name                string
		cleanupClaimGranted bool
		wantStopCalls       int32
	}{
		{name: "another cleanup path owns the execution"},
		{name: "startup callback owns cleanup", cleanupClaimGranted: true, wantStopCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const (
				sessionID   = "session-123"
				executionID = "exec-456"
				attemptID   = "attempt-789"
			)
			repo := newMockRepository()
			repo.sessions[sessionID] = &models.TaskSession{
				ID:               sessionID,
				State:            models.TaskSessionStateCancelled,
				AgentExecutionID: executionID,
				Metadata: map[string]interface{}{
					models.SessionMetaKeyAgentStartAttemptID: attemptID,
				},
			}
			var stopCalls atomic.Int32
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) {
					return executionID, nil
				},
				stopAgentWithReasonFunc: func(context.Context, string, string, bool) error {
					stopCalls.Add(1)
					return nil
				},
			}
			exec := newTestExecutor(t, manager, repo)
			exec.SetOnExecutionCleanupClaim(func(string, string) bool {
				return tc.cleanupClaimGranted
			})

			if !exec.stopFailedStartExecutionIfCurrentAttempt(
				context.Background(), "task-123", sessionID, executionID, attemptID, "cancelled resume startup",
			) {
				t.Fatal("cancelled startup attempt no longer owns the execution")
			}
			if got := stopCalls.Load(); got != tc.wantStopCalls {
				t.Fatalf("stop calls = %d, want %d", got, tc.wantStopCalls)
			}
		})
	}
}
