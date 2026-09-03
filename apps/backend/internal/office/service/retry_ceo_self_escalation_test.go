package service_test

// Regression coverage for the Path B self-escalation loop: escalateFailure
// -> queueCEOAgentError (retry.go) previously queued a CEO-targeted
// agent_error run without checking whether the failing run's agent WAS the
// CEO. escalateFailure's FailRun -> transitionRunTerminal path never touches
// the consecutive-failure counter (HandleAgentFailure does; escalateFailure
// does not call it), so a deterministically-failing CEO run retried
// MaxRetryCount times, escalated to itself, and repeated forever — see
// docs/specs/office/system-design/scheduler-01.md's escalation clause.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestHandleRunFailure_CEOOwnFailure_DoesNotSelfEscalate proves the loop is
// broken: a run belonging to the CEO agent itself, exhausted on retries,
// must not produce a new agent_error run for that same CEO. The escalation
// is repeated 3 times to prove the loop stays broken, not just skipped once.
func TestHandleRunFailure_CEOOwnFailure_DoesNotSelfEscalate(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	ceo := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "ceo-self-fail",
		Role:        models.AgentRoleCEO,
		Status:      models.AgentStatusIdle,
	}
	if err := svc.CreateAgentInstance(ctx, ceo); err != nil {
		t.Fatalf("create ceo: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := svc.QueueRun(
			ctx, ceo.ID, service.RunReasonAgentError, `{}`, "",
		); err != nil {
			t.Fatalf("queue ceo run %d: %v", i, err)
		}
		run, err := svc.ClaimNextRun(ctx)
		if err != nil || run == nil {
			t.Fatalf("claim ceo run %d: %v (run=%v)", i, err, run)
		}
		if run.AgentProfileID != ceo.ID {
			t.Fatalf("claimed run %d belongs to %q, want ceo %q", i, run.AgentProfileID, ceo.ID)
		}
		run.RetryCount = service.MaxRetryCount

		if err := svc.HandleRunFailure(ctx, run, errForTest("ceo boom")); err != nil {
			t.Fatalf("handle run failure %d: %v", i, err)
		}

		next, err := svc.ClaimNextRun(ctx)
		if err != nil {
			t.Fatalf("claim after failure %d: %v", i, err)
		}
		if next != nil {
			t.Fatalf("iteration %d: expected no self-escalation run queued, got agent=%q reason=%q",
				i, next.AgentProfileID, next.Reason)
		}
	}
}
