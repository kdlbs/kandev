package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeReadinessWaitPreservesExpiryAndCancellation(t *testing.T) {
	svc := &Service{agentManager: &mockAgentManager{
		isAgentReadyFn: func(context.Context, string) bool { return false },
	}}
	for name, wait := range map[string]func(context.Context, string) error{
		"session state":   svc.waitForSessionReady,
		"agent readiness": svc.waitForAgentPromptReady,
	} {
		t.Run(name, func(t *testing.T) {
			expiredCtx, stopExpired := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
			defer stopExpired()
			err := wait(expiredCtx, "runtime-wait-session")
			classified := svc.classifyEnsureSessionRunningFailureForPrompt(
				t.Context(), "runtime-wait-task", "runtime-wait-session", "prompt", "", false,
				nil, false, promptTaskOptions{}, err,
			)
			if !errors.Is(classified, context.DeadlineExceeded) || !isEnsureSessionRunningTimeout(classified) {
				t.Fatalf("readiness expiry lost its retry identity: %v", classified)
			}

			cancelledCtx, stopCancelled := context.WithCancel(t.Context())
			stopCancelled()
			err = wait(cancelledCtx, "runtime-wait-session")
			classified = svc.classifyEnsureSessionRunningFailureForPrompt(
				t.Context(), "runtime-wait-task", "runtime-wait-session", "prompt", "", false,
				nil, false, promptTaskOptions{}, err,
			)
			if !errors.Is(classified, context.Canceled) || isEnsureSessionRunningTimeout(classified) {
				t.Fatalf("cancellation became readiness expiry: %v", classified)
			}
		})
	}
}
