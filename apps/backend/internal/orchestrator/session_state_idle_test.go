package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestUpdateTaskSessionState_IdleTransitions exercises the IDLE-aware state
// machine relax (B2): IDLE is non-terminal, and the IDLE↔RUNNING cycle plus
// IDLE→COMPLETED / IDLE→CANCELLED transitions are allowed. Existing terminal
// states (COMPLETED / FAILED / CANCELLED) remain absorbing.
func TestUpdateTaskSessionState_IdleTransitions(t *testing.T) {
	cases := []struct {
		name      string
		from      models.TaskSessionState
		to        models.TaskSessionState
		wantState models.TaskSessionState
	}{
		{"running_to_idle", models.TaskSessionStateRunning, models.TaskSessionStateIdle, models.TaskSessionStateIdle},
		{"idle_to_running", models.TaskSessionStateIdle, models.TaskSessionStateRunning, models.TaskSessionStateRunning},
		{"idle_to_completed", models.TaskSessionStateIdle, models.TaskSessionStateCompleted, models.TaskSessionStateCompleted},
		{"idle_to_cancelled", models.TaskSessionStateIdle, models.TaskSessionStateCancelled, models.TaskSessionStateCancelled},
		{"idle_to_failed", models.TaskSessionStateIdle, models.TaskSessionStateFailed, models.TaskSessionStateFailed},
		// Terminal states stay absorbing — IDLE must not "wake" them.
		{"completed_idle_blocked", models.TaskSessionStateCompleted, models.TaskSessionStateIdle, models.TaskSessionStateCompleted},
		{"cancelled_idle_blocked", models.TaskSessionStateCancelled, models.TaskSessionStateIdle, models.TaskSessionStateCancelled},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "task1", "session1", "step1")

			// Drive the seeded session into the desired starting state.
			if err := repo.UpdateTaskSessionState(ctx, "session1", tc.from, ""); err != nil {
				t.Fatalf("seed initial state: %v", err)
			}

			svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
			svc.updateTaskSessionState(ctx, "task1", "session1", tc.to, "", false)

			got, err := repo.GetTaskSession(ctx, "session1")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if got.State != tc.wantState {
				t.Errorf("state: got %q want %q", got.State, tc.wantState)
			}
		})
	}
}

// TestIsTerminalSessionState_ExcludesIdle is a tiny regression net for the
// promote-primary helper: IDLE is NOT terminal. This protects against any
// future change that lumps IDLE in with COMPLETED/FAILED/CANCELLED.
func TestIsTerminalSessionState_ExcludesIdle(t *testing.T) {
	if isTerminalSessionState(models.TaskSessionStateIdle) {
		t.Error("IDLE must not be terminal")
	}
	for _, s := range []models.TaskSessionState{
		models.TaskSessionStateCompleted,
		models.TaskSessionStateFailed,
		models.TaskSessionStateCancelled,
	} {
		if !isTerminalSessionState(s) {
			t.Errorf("%q must be terminal", s)
		}
	}
}
