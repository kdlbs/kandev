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
// no-guard-at-step fallback: a caller holding both seats, with neither
// seat's StepID matching the step being queried and that step naming no
// wait_for_quorum guard (quorumEngine's step has a nil Guard), still
// resolves under approver-wins — the step-preference early return applies
// when either seat sits at the queried step, and the guard-role tiebreak only
// kicks in when the queried step names exactly one guard role.
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

// TestResolveParticipantRole_BothSeatsAtSameNonCurrentStep_GuardRoleWins is
// the regression test for the thin-workspace Review deadlock: both seats
// were cast at Backlog (before the task ever reached Review), so neither
// sits at the step being evaluated and the old approver-first cross-step
// scan always won. The step actually being evaluated (Review) has a
// reviewer wait_for_quorum guard, so resolution must prefer reviewer, and
// recording the decision under the resolved seat must satisfy that guard
// and fire Review -> Approval instead of parking forever.
func TestResolveParticipantRole_BothSeatsAtSameNonCurrentStep_GuardRoleWins(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	decisions := newFakeDecisionStore()
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "backlog", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "backlog", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
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

// TestResolveParticipantRole_IgnoresApprovalRequiredGuard keeps role
// resolution aligned with automatic transition evaluation: an approval-only
// action is not a guard that the current step can satisfy automatically.
func TestResolveParticipantRole_IgnoresApprovalRequiredGuard(t *testing.T) {
	store := &stepStoreForQuorum{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToNext, RequiresApproval: true, Guard: &TransitionGuard{
						WaitForQuorum: &WaitForQuorumGuard{Role: "approver", Threshold: QuorumAllApprove},
					}},
					{Kind: ActionMoveToNext, Guard: &TransitionGuard{
						WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
					}},
				},
			},
		},
		next:    StepSpec{ID: "approval", Position: 2},
		applied: map[string]bool{},
	}
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "backlog", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "backlog", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := New(store, MapRegistry{}, WithParticipantStore(participants))

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "reviewer" || participantID != "seat-reviewer" {
		t.Errorf("role/participantID = %q/%q, want reviewer/seat-reviewer", role, participantID)
	}
}

// TestResolveParticipantRole_BothSeatsAtDifferentNonCurrentSteps_GuardRoleWins
// documents the AC-4 behavior for seats attached to two different earlier
// steps. The implementation already applies the guard-role tie-break here.
func TestResolveParticipantRole_BothSeatsAtDifferentNonCurrentSteps_GuardRoleWins(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "backlog", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "approval", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := New(store, MapRegistry{}, WithParticipantStore(participants))

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "reviewer" || participantID != "seat-reviewer" {
		t.Errorf("role/participantID = %q/%q, want reviewer/seat-reviewer", role, participantID)
	}
}

// TestResolveParticipantRole_BothSeatsAtSameNonCurrentStep_StepNamesBothRoles_ApproverWins
// covers AC-4's third fallback branch: the queried step names *two* guard
// roles (reviewer and approver each gated separately), so the single-role
// tiebreak in ResolveParticipantRole cannot apply and resolution must fall
// through to the final approver-wins line — distinct from the sibling test
// above, where the step names no guard role at all.
func TestResolveParticipantRole_BothSeatsAtSameNonCurrentStep_StepNamesBothRoles_ApproverWins(t *testing.T) {
	store := &stepStoreForQuorum{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToNext, Guard: &TransitionGuard{
						WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
					}},
					{Kind: ActionMoveToNext, Guard: &TransitionGuard{
						WaitForQuorum: &WaitForQuorumGuard{Role: "approver", Threshold: QuorumAllApprove},
					}},
				},
			},
		},
		next:    StepSpec{ID: "approval", Position: 2},
		applied: map[string]bool{},
	}
	participants := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "seat-reviewer", TaskID: "task-1", StepID: "backlog", Role: "reviewer", AgentProfileID: "agent-a", DecisionRequired: true},
		{ID: "seat-approver", TaskID: "task-1", StepID: "backlog", Role: "approver", AgentProfileID: "agent-a", DecisionRequired: true},
	}}
	eng := New(store, MapRegistry{}, WithParticipantStore(participants))

	role, participantID, err := eng.ResolveParticipantRole(context.Background(), "task-1", "review", "agent-a")
	if err != nil {
		t.Fatalf("ResolveParticipantRole: %v", err)
	}
	if role != "approver" || participantID != "seat-approver" {
		t.Errorf("role/participantID = %q/%q, want approver/seat-approver", role, participantID)
	}
}
