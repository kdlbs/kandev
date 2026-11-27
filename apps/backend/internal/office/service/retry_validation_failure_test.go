package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/service"
)

// A deterministic launch-validation failure must not consume the retry
// budget: the run fails immediately and escalates, so no new session is
// created per retry.
func TestHandleRunFailure_ValidationFailureDoesNotRetry(t *testing.T) {
	for _, command := range []string{"agent", "continue"} {
		t.Run(command, func(t *testing.T) {
			svc, _ := newTestServiceWithBus(t)
			ctx := context.Background()

			createTestAgent(t, svc, "ws-1", "agent-validation")
			taskID := "task-validation"
			insertSyntheticTask(t, svc, taskID, "ws-1", "agent-validation")
			run := queueAndReadRun(t, svc, "agent-validation", taskID)

			launchErr := fmt.Errorf("launch agent: %w", &routingerr.BuiltCommandValidationError{Command: command, Cause: errors.New("agent command cannot be empty")})
			if err := svc.HandleRunFailure(ctx, run, launchErr); err != nil {
				t.Fatalf("handle failure: %v", err)
			}

			refreshed, err := svc.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatalf("get run: %v", err)
			}
			if refreshed.Status != service.RunStatusFailed {
				t.Fatalf("run status = %q, want %q (validation failures must fail, not retry)", refreshed.Status, service.RunStatusFailed)
			}
			if refreshed.RetryCount != 0 {
				t.Fatalf("retry_count = %d, want 0 (no retry consumed)", refreshed.RetryCount)
			}
			if refreshed.ScheduledRetryAt != nil {
				t.Fatal("expected scheduled_retry_at to remain unset")
			}
		})
	}
}

// A transient failure keeps today's bounded retry behavior.
func TestHandleRunFailure_TransientFailureStillRetriesBounded(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-transient-kept")
	taskID := "task-transient-kept"
	insertSyntheticTask(t, svc, taskID, "ws-1", "agent-transient-kept")
	run := queueAndReadRun(t, svc, "agent-transient-kept", taskID)

	if err := svc.HandleRunFailure(ctx, run, errors.New("validate agent command: provider read temporarily unavailable")); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	refreshed, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if refreshed.Status != service.RunStatusQueued {
		t.Fatalf("run status = %q, want %q", refreshed.Status, service.RunStatusQueued)
	}
	if refreshed.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", refreshed.RetryCount)
	}
}
