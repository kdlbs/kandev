package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// childSummariesOf returns the dispatched trigger's child summaries, failing if
// the call was not the children-completed trigger.
func childSummariesOf(t *testing.T, payload any) []engine.ChildSummary {
	t.Helper()
	p, ok := payload.(engine.OnChildrenCompletedPayload)
	if !ok {
		t.Fatalf("dispatched payload is %T, want engine.OnChildrenCompletedPayload", payload)
	}
	return p.ChildSummaries
}

// The prompt path derives the child list itself, so a producer that reads child
// summaries is paying for data nobody reads. Nothing about the wake it queues
// may depend on that read.
func TestQueueChildrenCompletedRun_PerformsNoChildSummaryRead(t *testing.T) {
	prs := &countingPRLister{}
	svc, eb := newTestServiceWithBusAndPRs(t, prs)
	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	event := bus.NewEvent(events.TaskMoved, "test", map[string]string{
		"task_id":                   "parent-1-child-0",
		"workspace_id":              "ws-1",
		"from_step_name":            "Work",
		"to_step_name":              "Done",
		"assignee_agent_profile_id": "worker-1",
		"parent_id":                 "parent-1",
	})
	if err := eb.Publish(context.Background(), events.TaskMoved, event); err != nil {
		t.Fatalf("publish task.moved: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("dispatched %d triggers, want 1: %#v", len(calls), calls)
	}
	if got := childSummariesOf(t, calls[0].payload); len(got) != 0 {
		t.Errorf("producer still assembled %d child summaries", len(got))
	}
	if prs.calls != 0 {
		t.Errorf("producer made %d PR lookups, want 0", prs.calls)
	}
	if calls[0].opID == "" {
		t.Error("operation id was lost with the child summary read")
	}
}

// The reconciler used to abort a tick when the child summary read failed. With
// the read gone it dispatches, which strictly increases delivery of a wake
// readiness already judged due.
func TestParentWakeReconciler_DispatchesWithoutChildSummaryRead(t *testing.T) {
	prs := &countingPRLister{}
	svc := newTestService(t, service.ServiceOptions{TaskPRs: prs})
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	// The child summary statement cannot run without this table. Before the
	// read was removed this aborted the tick before any dispatch.
	svc.ExecSQL(t, `DROP TABLE task_comments`)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}

	calls := dispatcher.Calls()
	if len(calls) != 1 {
		t.Fatalf("dispatched %d triggers, want 1: %#v", len(calls), calls)
	}
	if calls[0].trigger != engine.TriggerOnChildrenCompleted {
		t.Errorf("trigger = %v, want on_children_completed", calls[0].trigger)
	}
	if got := childSummariesOf(t, calls[0].payload); len(got) != 0 {
		t.Errorf("reconciler still assembled %d child summaries", len(got))
	}
	if prs.calls != 0 {
		t.Errorf("reconciler made %d PR lookups, want 0", prs.calls)
	}
	if calls[0].opID == "" {
		t.Error("operation id was lost with the child summary read")
	}
}

// Removing the summary read must not relax the guards that decide whether to
// dispatch at all.
func TestParentWakeReconciler_ChildSetChangeStillGatesDispatch(t *testing.T) {
	svc := newTestService(t, service.ServiceOptions{TaskPRs: &countingPRLister{}})
	ctx := context.Background()

	adoptOffice(t, svc, "ws-1")
	seedStuckParent(t, svc, "ws-1", "parent-1", "worker-1")

	dispatcher := &fakeDispatcher{}
	svc.SetWorkflowEngineDispatcher(dispatcher)

	// A child that is no longer terminal takes the parent out of the stuck
	// set, so the readiness gate must suppress the wake.
	svc.ExecSQL(t, `UPDATE tasks SET state = 'IN_PROGRESS' WHERE id = 'parent-1-child-0'`)

	handler := service.NewParentWakeReconciler(service.NewSchedulerIntegration(svc, 0))
	if err := handler.Tick(ctx); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if calls := dispatcher.Calls(); len(calls) != 0 {
		t.Errorf("dispatched %d triggers for a parent with a live child: %#v",
			len(calls), calls)
	}
}
