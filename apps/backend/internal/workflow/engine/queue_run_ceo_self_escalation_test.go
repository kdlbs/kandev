package engine

import (
	"context"
	"testing"
)

// newOnAgentErrorInput builds a workspace.ceo_agent queue_run ActionInput
// carrying an on_agent_error trigger and payload, matching how
// office/service/event_subscribers.go's dispatchAgentErrorTrigger dispatches
// Path A (terminal agent-session failure).
func newOnAgentErrorInput(failedAgentID string) ActionInput {
	in := newQueueRunInput("workspace.ceo_agent", "this")
	in.Trigger = TriggerOnAgentError
	in.Payload = OnAgentErrorPayload{FailedAgentID: failedAgentID}
	return in
}

// TestQueueRunCallback_TargetWorkspaceCEO_SkipsSelfEscalation covers the
// guard: an on_agent_error trigger whose FailedAgentID is the CEO's own
// resolved profile id must not re-queue the CEO. Without the guard this
// would queue the CEO to handle its own failure, and a deterministically
// failing CEO would re-trigger the same escalation on the next failure.
func TestQueueRunCallback_TargetWorkspaceCEO_SkipsSelfEscalation(t *testing.T) {
	q := &fakeRunQueue{}
	log, logs := newObservedSeatLogger(t)
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}, Logger: log}

	if _, err := cb.Execute(context.Background(), newOnAgentErrorInput("ceo-agent")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 0 {
		t.Fatalf("expected 0 calls (self-escalation skipped), got %d: %+v", len(q.calls), q.calls)
	}

	matches := logs.FilterMessage("queue_run: skipped workspace.ceo_agent self-escalation")
	if matches.Len() != 1 {
		t.Fatalf("expected one warning for self-escalation skip, got %d", matches.Len())
	}
	fields := matches.All()[0].ContextMap()
	if fields["task_id"] != "task-1" {
		t.Fatalf("task_id field = %v, want task-1", fields["task_id"])
	}
	if fields["ceo_agent_id"] != "ceo-agent" {
		t.Fatalf("ceo_agent_id field = %v, want ceo-agent", fields["ceo_agent_id"])
	}
}

// TestQueueRunCallback_TargetWorkspaceCEO_OnAgentErrorNonCEOStillQueues is
// the no-regression witness: a sub-agent's failure must still wake the CEO.
func TestQueueRunCallback_TargetWorkspaceCEO_OnAgentErrorNonCEOStillQueues(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}}

	if _, err := cb.Execute(context.Background(), newOnAgentErrorInput("worker-agent")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if q.calls[0].AgentProfileID != "ceo-agent" {
		t.Fatalf("agent_profile_id = %q, want ceo-agent", q.calls[0].AgentProfileID)
	}
}

// TestQueueRunCallback_TargetWorkspaceCEO_NonAgentErrorTriggerUnaffected
// pins that the guard only applies to on_agent_error: any other trigger
// targeting workspace.ceo_agent queues normally even if the payload happens
// to be an OnAgentErrorPayload (defense against a too-broad type check).
func TestQueueRunCallback_TargetWorkspaceCEO_NonAgentErrorTriggerUnaffected(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}}

	in := newOnAgentErrorInput("ceo-agent")
	in.Trigger = TriggerOnComment

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
}
