package engine

import (
	"context"
	"testing"
)

// TestQueueRunCallback_OnAgentErrorPayloadProjectsFailureFields is the Path A
// regression: on_agent_error's queue_run action must carry the failed
// agent/session id and error message into the queued run's payload so the
// CEO prompt can name them, instead of silently dropping OnAgentErrorPayload
// the way commentPayload's type switch does.
func TestQueueRunCallback_OnAgentErrorPayloadProjectsFailureFields(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}}

	in := newQueueRunInput("workspace.ceo_agent", "this")
	in.Trigger = TriggerOnAgentError
	in.Payload = OnAgentErrorPayload{
		FailedAgentID:   "worker-1",
		FailedSessionID: "sess-1",
		ErrorMessage:    "exit status 1: context deadline exceeded",
	}
	in.Action.QueueRun.Payload = nil

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	got := q.calls[0].Payload
	if got["failed_agent_id"] != "worker-1" {
		t.Errorf("failed_agent_id = %v, want worker-1", got["failed_agent_id"])
	}
	if got["failed_session_id"] != "sess-1" {
		t.Errorf("failed_session_id = %v, want sess-1", got["failed_session_id"])
	}
	if got["error"] != "exit status 1: context deadline exceeded" {
		t.Errorf("error = %v, want the real error text", got["error"])
	}
}

// TestQueueRunCallback_OnAgentErrorPayload_WorkflowAuthoredPayloadWins pins
// the existing precedence: an explicit payload: key in the workflow action
// still overrides the projected OnAgentErrorPayload field.
func TestQueueRunCallback_OnAgentErrorPayload_WorkflowAuthoredPayloadWins(t *testing.T) {
	q := &fakeRunQueue{}
	cb := QueueRunCallback{Adapter: q, CEOResolver: fakeCEO{id: "ceo-agent"}}

	in := newQueueRunInput("workspace.ceo_agent", "this")
	in.Trigger = TriggerOnAgentError
	in.Payload = OnAgentErrorPayload{FailedAgentID: "worker-1", ErrorMessage: "boom"}
	in.Action.QueueRun.Payload = map[string]any{"error": "overridden"}

	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	got := q.calls[0].Payload
	if got["error"] != "overridden" {
		t.Errorf("error = %v, want workflow-authored override to win", got["error"])
	}
	if got["failed_agent_id"] != "worker-1" {
		t.Errorf("failed_agent_id = %v, want worker-1 (still projected)", got["failed_agent_id"])
	}
}
