package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// A stale-retry cancel that loses the terminal-transition race (another
// writer already finished the run) must not tell subscribers the run was
// cancelled: the run's real terminal state is finished, and a status
// mismatch between the persisted row and the broadcast event misleads
// every WS-driven UI reading it.
func TestHandleRunFailure_LostCancelRaceDoesNotPublishFalseCancelled(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	agent := makeAgent("worker-stale-retry-race", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned, "{}", "k-stale-retry-race"); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	staleRequestedAt := time.Now().UTC().Add(-25 * time.Hour)
	svc.ExecSQL(t,
		`UPDATE kandev_meta SET value = ? WHERE key = 'telemetry.office_loop_liveness.activated_at'`,
		staleRequestedAt.Add(-time.Hour).Format(time.RFC3339))
	svc.ExecSQL(t, `UPDATE runs SET requested_at = ? WHERE id = ?`, staleRequestedAt, run.ID)
	run.RequestedAt = staleRequestedAt

	// Another writer wins the terminal-transition race before the stale
	// cancel below runs: the row is already finished, so CancelRun's
	// guarded UPDATE (status IN ('queued','claimed')) will match it.
	if _, err := svc.FinishRun(ctx, run.ID, service.RunOutcomeProcessed); err != nil {
		t.Fatalf("finish (simulating the winning writer): %v", err)
	}

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	netErr := errors.New("connection timed out")
	if err := svc.HandleRunFailure(ctx, run, netErr); err != nil {
		t.Fatalf("handle failure: %v", err)
	}

	select {
	case e := <-got:
		t.Fatalf("expected no OfficeRunProcessed publish for a lost cancel race; got %+v", e)
	default:
	}

	reqs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(reqs) != 1 || reqs[0].Status != service.RunStatusFinished {
		t.Fatalf("run status = %+v, want the winning writer's finished state to survive", reqs)
	}
}

// The stale-claim cancel path (evaluateRunStaleness's workflow_step_changed
// branch, reached from the scheduler tick) has the same obligation as the
// stale-retry path above: a lost cancel race must not publish a false
// "cancelled" event or log a false cancel activity entry.
func TestSchedulerIntegration_CancelStaleRunLostRaceDoesNotPublishOrLog(t *testing.T) {
	mock := &mockTaskStarter{}
	eb := bus.NewMemoryEventBus(logger.Default())
	svc := newTestService(t, service.ServiceOptions{TaskStarter: mock, EventBus: eb})
	ctx := context.Background()

	agent := makeAgent("worker-moved-step-race", models.AgentRoleWorker)
	agent.ExecutorPreference = `{"type":"local_pc"}`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	svc.ExecSQL(t, `INSERT INTO tasks
		(id, workspace_id, workflow_step_id, title, created_at, updated_at)
		VALUES ('task-moved-step-race', 'ws-1', 'step-current', 'Moved task',
		        CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonTaskAssigned,
		`{"task_id":"task-moved-step-race","workflow_step_id":"step-old"}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	runs, err := svc.ListRuns(ctx, "ws-1")
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs: %v (%d)", err, len(runs))
	}
	runID := runs[0].ID

	// Simulate another writer winning the race for this exact row: the
	// tick's own CancelRun write below will match zero rows and return
	// no error, the same shape a genuinely lost race produces.
	svc.ExecSQL(t, fmt.Sprintf(`
		CREATE TRIGGER ignore_stale_cancel_write
		BEFORE UPDATE OF status ON runs
		WHEN NEW.status = 'cancelled' AND OLD.id = '%s'
		BEGIN
			SELECT RAISE(IGNORE);
		END;
	`, runID))

	got := make(chan *bus.Event, 1)
	sub, err := eb.Subscribe(events.OfficeRunProcessed, func(_ context.Context, e *bus.Event) error {
		got <- e
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	service.RunSchedulerTick(svc, ctx)

	select {
	case e := <-got:
		t.Fatalf("expected no OfficeRunProcessed publish for a lost stale-cancel race; got %+v", e)
	default:
	}

	entries, err := svc.RepoForTest().ListActivityEntriesByAction(ctx, "ws-1", "run_cancelled_stale", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("run_cancelled_stale activity entries = %d, want 0 for a lost cancel race", len(entries))
	}
}
