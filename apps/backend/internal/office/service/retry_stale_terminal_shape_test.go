package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// cancelRetry (the 24h-stale-retry path reached through HandleRunFailure)
// bypasses transitionRunTerminal's own counting the same way
// HandleAgentFailure and cancelStaleRun do, so it must record its own
// terminal shape or office_loop_terminal_total silently misses every
// abandoned-retry cancellation (Review round 1, R1-1). This is a distinct
// trigger condition from cancelStaleRun's workflow_step_changed path,
// covered separately in TestSchedulerIntegration_CancelStaleRunRecordsTerminalShape.
func TestHandleRunFailure_CancelsStaleRetryAndRecordsTerminalShape(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-stale-retry", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k-stale-retry"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	// Backdate past retryMaxAge (24h) so isRetryStale routes this failure
	// to cancelRetry instead of a normal backoff reschedule. Also push the
	// loop-liveness activation instant back further still, so the
	// backdated run doesn't fall outside the activation window and
	// classify as pre_activation instead of the shape under test.
	staleRequestedAt := time.Now().UTC().Add(-25 * time.Hour)
	svc.ExecSQL(t,
		`UPDATE kandev_meta SET value = ? WHERE key = 'telemetry.office_loop_liveness.activated_at'`,
		staleRequestedAt.Add(-time.Hour).Format(time.RFC3339))
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ? WHERE id = ?`, staleRequestedAt, run.ID)
	run.RequestedAt = staleRequestedAt

	key := service.LoopMetricLabel("workspace", "ws-1", "shape", string(service.ShapeUnlaunchedFailed))
	before := terminalShapeExpvarInt(t, key)

	netErr := errors.New("connection timed out")
	if err := svc.HandleRunFailure(ctx, run, netErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	after := terminalShapeExpvarInt(t, key)
	if after != before+1 {
		t.Fatalf("unlaunched_failed delta = %d, want 1", after-before)
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(reqs))
	}
	if reqs[0].Status != service.RunStatusCancelled {
		t.Errorf("status = %q, want cancelled", reqs[0].Status)
	}
	if reqs[0].CancelReason == nil || *reqs[0].CancelReason != "run_too_old" {
		t.Errorf("cancel_reason = %v, want run_too_old", reqs[0].CancelReason)
	}
}
