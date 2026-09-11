package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// cancelBudgetRun and cancelUnresolvableAgentRun mirror cancelStaleRun's
// release/cancel/classify/publish/log sequence, but their CancelRun call
// must still be gated the same way: a run that another writer already
// moved out of claimed/queued before this cancel runs must not be
// classified, broadcast, or logged as cancelled.
func TestCancelBudgetRun_LostRaceDoesNotClassifyPublishOrLog(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	agent := makeAgent("worker-budget-cancel-race", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}

	// Another writer wins the terminal-transition race before gate 1's
	// cancel below runs: the row is already finished, so CancelRun's
	// guarded UPDATE (status IN ('queued','claimed')) will match nothing.
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

	unresolvedAgent := *agent
	unresolvedAgent.WorkspaceID = ""
	service.AdmitRunForTest(svc, ctx, run, &unresolvedAgent)

	select {
	case e := <-got:
		t.Fatalf("expected no OfficeRunProcessed publish for a lost budget-cancel race; got %+v", e)
	default:
	}

	entries, err := svc.RepoForTest().ListActivityEntriesByAction(ctx, "", "run_budget_workspace_unresolvable", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("run_budget_workspace_unresolvable activity entries = %d, want 0 for a lost cancel race", len(entries))
	}

	survivor, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if survivor.Status != service.RunStatusFinished {
		t.Fatalf("status = %q, want the winning writer's finished state to survive", survivor.Status)
	}
}

// cancelUnresolvableAgentRun (reached when the run's agent was deleted)
// has the same lost-race obligation as cancelBudgetRun above.
func TestCancelUnresolvableAgentRun_LostRaceDoesNotClassifyPublishOrLog(t *testing.T) {
	svc, eb := newTestServiceWithBus(t)
	ctx := context.Background()

	agent := makeAgent("worker-unresolvable-cancel-race", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if err := svc.QueueRun(ctx, agent.ID, service.RunReasonRoutineTrigger, `{}`, ""); err != nil {
		t.Fatalf("queue: %v", err)
	}
	run, err := svc.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim: %v %v", run, err)
	}
	if err := svc.RepoForTest().DeleteAgentInstance(ctx, agent.ID); err != nil {
		t.Fatalf("delete agent: %v", err)
	}

	// Another writer wins the terminal-transition race before the
	// missing-agent cancel below runs.
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

	service.ProcessRunForTest(svc, ctx, run)

	select {
	case e := <-got:
		t.Fatalf("expected no OfficeRunProcessed publish for a lost missing-agent cancel race; got %+v", e)
	default:
	}

	entries, err := svc.RepoForTest().ListActivityEntriesByAction(ctx, "", "run_budget_workspace_unresolvable", 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("run_budget_workspace_unresolvable activity entries = %d, want 0 for a lost cancel race", len(entries))
	}

	survivor, err := svc.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if survivor.Status != service.RunStatusFinished {
		t.Fatalf("status = %q, want the winning writer's finished state to survive", survivor.Status)
	}
}
