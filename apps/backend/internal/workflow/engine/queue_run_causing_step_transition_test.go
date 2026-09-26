package engine

import (
	"context"
	"testing"
)

// TestQueueRunCallback_CarriesCausingStepTransitionID covers
// AC-OFFICE-RUN-CAUSATION-001.25: a queue_run action dispatched through
// DispatchStepEntry sets ActionInput.EntryID (and, symmetrically,
// CausingStepTransitionID) to the step-transition ledger row's own
// identifier. Before this change QueueRunCallback never forwarded that
// identity onto the queued request at all, so a carrier resolver had
// nothing but an empty CausingAgentProfileID to work with — every
// step-entry wake rooted as a fresh system chain instead of inheriting the
// causing run.
func TestQueueRunCallback_CarriesCausingStepTransitionID(t *testing.T) {
	q := &fakeRunQueue{}
	primary := fakePrimary{id: "agent-1"}
	cb := QueueRunCallback{Adapter: q, Primary: primary}
	in := ActionInput{
		Trigger:                 TriggerOnEnter,
		State:                   MachineState{TaskID: "task-1"},
		Step:                    StepSpec{ID: "step-1"},
		EntryID:                 "4242",
		CausingStepTransitionID: "4242",
		Action:                  Action{Kind: ActionQueueRun, QueueRun: &QueueRunAction{}},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if got := q.calls[0].CausingStepTransitionID; got != "4242" {
		t.Fatalf("causing_step_transition_id = %q, want %q", got, "4242")
	}
}

// TestQueueRunCallback_NoStepEntry_LeavesCausingStepTransitionIDEmpty is the
// regression guard for every non-step-entry trigger (on_turn_complete,
// on_children_completed, ...): ActionInput.CausingStepTransitionID is only
// ever populated by DispatchStepEntry, so a live-session trigger must reach
// the adapter with it empty, falling back to today's
// CausingAgentProfileID-scoped carrier resolution.
func TestQueueRunCallback_NoStepEntry_LeavesCausingStepTransitionIDEmpty(t *testing.T) {
	q := &fakeRunQueue{}
	primary := fakePrimary{id: "agent-1"}
	cb := QueueRunCallback{Adapter: q, Primary: primary}
	in := ActionInput{
		Trigger:     TriggerOnTurnComplete,
		State:       MachineState{TaskID: "task-1", AgentProfileID: "executing-agent"},
		Step:        StepSpec{ID: "step-1"},
		OperationID: "op-1",
		Action:      Action{Kind: ActionQueueRun, QueueRun: &QueueRunAction{}},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if got := q.calls[0].CausingStepTransitionID; got != "" {
		t.Fatalf("causing_step_transition_id = %q, want empty", got)
	}
	if got := q.calls[0].CausingAgentProfileID; got != "executing-agent" {
		t.Fatalf("causing_agent_profile_id = %q, want %q (unaffected regression)", got, "executing-agent")
	}
}

// TestQueueRunForEachParticipantCallback_CarriesCausingStepTransitionID
// covers the marker-bearing fan-out sibling: a
// queue_run_for_each_participant action executes through
// MarkerBearingStepEntryExecutor, which must NOT set ActionInput.EntryID
// (that would change the step_entry:<entry>:<pos> idempotency key), so this
// callback reads the separate CausingStepTransitionID field instead.
func TestQueueRunForEachParticipantCallback_CarriesCausingStepTransitionID(t *testing.T) {
	q := &fakeRunQueue{}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"},
	}}
	cb := QueueRunForEachParticipantCallback{Adapter: q, Participants: parts}
	in := ActionInput{
		Trigger:                 TriggerOnEnter,
		State:                   MachineState{TaskID: "task-1"},
		Step:                    StepSpec{ID: "step-1"},
		OperationID:             "step_entry:99:0",
		CausingStepTransitionID: "99",
		Action: Action{
			Kind: ActionQueueRunForEachParticipant,
			QueueRunForEachParticipant: &QueueRunForEachParticipantAction{
				Role: "reviewer",
			},
		},
	}
	if _, err := cb.Execute(context.Background(), in); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(q.calls))
	}
	if got := q.calls[0].CausingStepTransitionID; got != "99" {
		t.Fatalf("causing_step_transition_id = %q, want %q", got, "99")
	}
	// EntryID must stay empty on the marker path so idempotencyKey keeps
	// using OperationID (the step_entry:<entry>:<pos> key), not a
	// different derivation that would change on redelivery.
	if in.EntryID != "" {
		t.Fatalf("EntryID = %q, want empty (must not change the marker-path idempotency key)", in.EntryID)
	}
}
