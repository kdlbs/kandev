package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

func TestClaimNextRun_ReturnsNilWhenEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	req, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req != nil {
		t.Errorf("expected nil, got %+v", req)
	}
}

func TestClaimNextRun_ClaimsQueued(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", ""); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, err := svc.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if req == nil {
		t.Fatal("expected a run, got nil")
	}
	if req.AgentProfileID != agent.ID {
		t.Errorf("agent = %q, want %q", req.AgentProfileID, agent.ID)
	}
}

func TestProcessRunGuard_AllowsActiveAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	run := &models.Run{AgentProfileID: agent.ID}
	ok, err := svc.ProcessRunGuard(ctx, run)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if !ok {
		t.Error("expected guard to allow idle agent")
	}
}

func TestProcessRunGuard_BlocksPausedAgent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := svc.UpdateAgentStatus(ctx, agent.ID, models.AgentStatusPaused, "test"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	run := &models.Run{AgentProfileID: agent.ID}
	ok, err := svc.ProcessRunGuard(ctx, run)
	if err != nil {
		t.Fatalf("guard: %v", err)
	}
	if ok {
		t.Error("expected guard to block paused agent")
	}
}

func TestFinishAndFailRun(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k1"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, _ := svc.ClaimNextRun(ctx)
	if req == nil {
		t.Fatal("expected claimed run")
	}

	if err := svc.FinishRun(ctx, req.ID, service.RunOutcomeProcessed); err != nil {
		t.Fatalf("finish: %v", err)
	}

	// Should be empty now.
	next, _ := svc.ClaimNextRun(ctx)
	if next != nil {
		t.Error("expected no more runs")
	}
}

// TestFailRun_WritesFailedStatusAndNullOutcome is Review round 2, finding
// #1: TestFinishAndFailRun never actually calls FailRun, so nothing
// asserted that a failed run's outcome column is written NULL rather than
// one of the eight finished-path outcome values. FailRun's own doc comment
// says outcome must be NULL because a failed run "never reaches the
// outcome-derived buckets" — this pins that contract against a regression
// that starts writing a real outcome string for a failed run.
func TestFailRun_WritesFailedStatusAndNullOutcome(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-1", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k1"); err != nil {
		t.Fatalf("queue: %v", err)
	}

	req, _ := svc.ClaimNextRun(ctx)
	if req == nil {
		t.Fatal("expected claimed run")
	}

	if err := svc.FailRun(ctx, req.ID); err != nil {
		t.Fatalf("fail: %v", err)
	}

	got, err := svc.GetRun(ctx, req.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != models.RunStatusFailed {
		t.Errorf("status = %q, want %q", got.Status, models.RunStatusFailed)
	}
	if got.Outcome != nil {
		t.Errorf("outcome = %q, want nil (a failed run must never carry a finished-path outcome value)", *got.Outcome)
	}
}

// TestTransitionRunTerminal_LastWriterWinsOnStatusAndOutcome is Review
// round 2's lower-priority TEST-002 (spec:2130-2132): two terminal callers
// reaching the same run row must never leave status and outcome
// disagreeing, because transitionRunTerminal's underlying FinishRun writes
// both columns in a single UPDATE statement. This drives both orderings —
// FinishRun-then-FailRun and FailRun-then-FinishRun — and asserts the row
// always ends up wholly consistent with whichever call went last.
func TestTransitionRunTerminal_LastWriterWinsOnStatusAndOutcome(t *testing.T) {
	ctx := context.Background()

	queueAndClaim := func(t *testing.T, svc *service.Service) string {
		t.Helper()
		agent := makeAgent("worker-1", models.AgentRoleWorker)
		if err := svc.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent: %v", err)
		}
		if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k1"); err != nil {
			t.Fatalf("queue: %v", err)
		}
		req, _ := svc.ClaimNextRun(ctx)
		if req == nil {
			t.Fatal("expected claimed run")
		}
		return req.ID
	}

	t.Run("FailRun after FinishRun", func(t *testing.T) {
		svc := newTestService(t)
		id := queueAndClaim(t, svc)
		if err := svc.FinishRun(ctx, id, service.RunOutcomeProcessed); err != nil {
			t.Fatalf("finish: %v", err)
		}
		if err := svc.FailRun(ctx, id); err != nil {
			t.Fatalf("fail: %v", err)
		}
		got, err := svc.GetRun(ctx, id)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if got.Status != models.RunStatusFailed || got.Outcome != nil {
			t.Fatalf("row = {status: %q, outcome: %v}, want the later FailRun write: {failed, nil}", got.Status, got.Outcome)
		}
	})

	t.Run("FinishRun after FailRun", func(t *testing.T) {
		svc := newTestService(t)
		id := queueAndClaim(t, svc)
		if err := svc.FailRun(ctx, id); err != nil {
			t.Fatalf("fail: %v", err)
		}
		if err := svc.FinishRun(ctx, id, service.RunOutcomeProcessed); err != nil {
			t.Fatalf("finish: %v", err)
		}
		got, err := svc.GetRun(ctx, id)
		if err != nil {
			t.Fatalf("get run: %v", err)
		}
		if got.Status != models.RunStatusFinished || got.Outcome == nil || *got.Outcome != service.RunOutcomeProcessed {
			t.Fatalf("row = {status: %q, outcome: %v}, want the later FinishRun write: {finished, %q}", got.Status, got.Outcome, service.RunOutcomeProcessed)
		}
	})
}
