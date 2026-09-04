package engine

import (
	"context"
	"errors"
	"testing"
)

// TestResolveParticipantRole_ApproverSeat covers AC-2/AC-3: an agent
// occupying an approver seat at the current step resolves to role
// "approver" and the AC-50 canonical seat id.
func TestResolveParticipantRole_ApproverSeat(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-1", TaskID: "task-1", StepID: "review", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "approver" {
		t.Errorf("role = %q, want approver", role)
	}
	if participantID != "seat-1" {
		t.Errorf("participantID = %q, want seat-1", participantID)
	}
}

// TestResolveParticipantRole_ReviewerSeat covers the reviewer-only case.
func TestResolveParticipantRole_ReviewerSeat(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-1", TaskID: "task-1", StepID: "review", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "reviewer" {
		t.Errorf("role = %q, want reviewer", role)
	}
	if participantID != "seat-1" {
		t.Errorf("participantID = %q, want seat-1", participantID)
	}
}

// TestResolveParticipantRole_ApproverWinsOverReviewer is AC-4: a caller
// holding both seats resolves under approver.
func TestResolveParticipantRole_ApproverWinsOverReviewer(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "review", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "review", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "approver" || participantID != "seat-approver" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-approver", role, participantID)
	}
}

// TestResolveParticipantRole_CrossStepPopulation is AC-4a: the caller's
// participant row was attached at an earlier step (a per-task row, not
// scoped to the step currently being evaluated), matching the same
// AC-50 population the quorum evaluator itself reads — not a
// step-scoped subset.
func TestResolveParticipantRole_CrossStepPopulation(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-1", TaskID: "task-1", StepID: "earlier-step", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "approver" || participantID != "seat-1" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-1", role, participantID)
	}
}

// TestResolveParticipantRole_NotAParticipant is AC-3: a caller with no
// matching seat is rejected with ErrParticipantNotFound.
func TestResolveParticipantRole_NotAParticipant(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-1", TaskID: "task-1", StepID: "review", Role: "approver", AgentProfileID: "agent-other", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	if _, _, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a"); !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("err = %v, want ErrParticipantNotFound", err)
	}
}

// TestResolveParticipantRole_DecisionNotRequiredSeatIgnored proves a seat
// with decision_required = 0 does not count as participation (AC-3).
func TestResolveParticipantRole_DecisionNotRequiredSeatIgnored(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-1", TaskID: "task-1", StepID: "review", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: false},
	}}
	eng := quorumEngine(nil, participants)

	if _, _, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a"); !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("err = %v, want ErrParticipantNotFound", err)
	}
}

// TestResolveParticipantRole_EmptyAgentProfileIDRejected guards against an
// empty caller id spuriously matching a malformed seat row with no
// agent_profile_id set.
func TestResolveParticipantRole_EmptyAgentProfileIDRejected(t *testing.T) {
	eng := quorumEngine(nil, scopedParticipants{})

	if _, _, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", ""); !errors.Is(err, ErrParticipantNotFound) {
		t.Fatalf("err = %v, want ErrParticipantNotFound", err)
	}
}

// TestResolveParticipantRole_StoreErrorWrapped proves a participant-store
// failure surfaces as a method-level error rather than being swallowed
// into ErrParticipantNotFound.
func TestResolveParticipantRole_StoreErrorWrapped(t *testing.T) {
	boom := errors.New("boom")
	eng := quorumEngine(nil, scopedParticipants{err: boom})

	_, _, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapped store error", err)
	}
}

// TestResolveParticipantRole_StepPreferredOverApproverWins is the
// regression test for the Review re-entry bug: a caller holding both
// seats at different steps must resolve under the role whose seat sits
// at the step actually being evaluated, not under approver-wins. The
// approver seat was cast once the task first reached Approval and
// persists across the Review re-entry that follows an Approval
// rejection, so both rows coexist by the time this resolves at "review".
func TestResolveParticipantRole_StepPreferredOverApproverWins(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "review", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "approval", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "reviewer" || participantID != "seat-reviewer" {
		t.Errorf("role/participantID = %q/%q, want reviewer/seat-reviewer", role, participantID)
	}
}

// TestResolveParticipantRole_StepPreferredTransitionFires proves the
// second half of the DoD: resolving under the step-preferred role and
// recording the decision under the resolved seat actually satisfies the
// reviewer quorum guard and fires Review -> Approval, where it used to
// park forever because the decision was misfiled against a seat absent
// from the reviewer slate.
func TestResolveParticipantRole_StepPreferredTransitionFires(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	decisions := newFakeDecisionStore()
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "review", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "approval", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(participants))

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "reviewer" || participantID != "seat-reviewer" {
		t.Fatalf("role/participantID = %q/%q, want reviewer/seat-reviewer", role, participantID)
	}

	result, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: participantID, Decision: DecisionApproved,
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if !result.Transitioned {
		t.Fatalf("expected the resolved reviewer decision to satisfy quorum and transition: %#v", result)
	}
	if result.FromStepID != "review" || result.ToStepID != "approval" {
		t.Fatalf("unexpected transition endpoints: %#v", result)
	}
}

// TestResolveParticipantRole_NeitherSeatAtStep_ApproverWins is AC-4's
// fallback case: a caller holding both seats, with neither seat's StepID
// matching the step being queried, still resolves under approver-wins —
// the step-preference added for the Review re-entry bug only kicks in
// when the approver seat itself sits at the queried step.
func TestResolveParticipantRole_NeitherSeatAtStep_ApproverWins(t *testing.T) {
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "review-1", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "approval-1", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := quorumEngine(nil, participants)

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review-2", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "approver" || participantID != "seat-approver" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-approver", role, participantID)
	}
}
