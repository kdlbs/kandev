package engine

import (
	"context"
	"errors"
	"testing"
)

// fakeDecisionStore lets tests pre-populate decisions and watch writes.
type fakeDecisionStore struct {
	byKey      map[string][]DecisionInfo
	cleared    map[string]int64
	recordErr  error
	clearedErr error
	listErr    error
}

func newFakeDecisionStore() *fakeDecisionStore {
	return &fakeDecisionStore{byKey: map[string][]DecisionInfo{}, cleared: map[string]int64{}}
}

func dkey(taskID, stepID string) string { return taskID + "|" + stepID }

func (f *fakeDecisionStore) ListStepDecisions(_ context.Context, taskID, stepID string) ([]DecisionInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.byKey[dkey(taskID, stepID)], nil
}

func (f *fakeDecisionStore) RecordStepDecision(_ context.Context, d DecisionInfo) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	k := dkey(d.TaskID, d.StepID)
	f.byKey[k] = append(f.byKey[k], d)
	return nil
}

func (f *fakeDecisionStore) ClearStepDecisions(_ context.Context, taskID, stepID string) (int64, error) {
	if f.clearedErr != nil {
		return 0, f.clearedErr
	}
	k := dkey(taskID, stepID)
	n := int64(len(f.byKey[k]))
	delete(f.byKey, k)
	f.cleared[k] += n
	return n, nil
}

// stepStoreForQuorum is a TransitionStore where the step holds a single
// transition action gated by a quorum guard.
type stepStoreForQuorum struct {
	state   MachineState
	step    StepSpec
	next    StepSpec
	applied map[string]bool
}

func (s *stepStoreForQuorum) LoadState(_ context.Context, _, _ string) (MachineState, error) {
	return s.state, nil
}
func (s *stepStoreForQuorum) LoadStep(_ context.Context, _, _ string) (StepSpec, error) {
	return s.step, nil
}
func (s *stepStoreForQuorum) LoadNextStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return s.next, nil
}
func (s *stepStoreForQuorum) LoadPreviousStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *stepStoreForQuorum) ApplyTransition(_ context.Context, _, _, _, _ string, _ Trigger) error {
	return nil
}
func (s *stepStoreForQuorum) PersistData(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *stepStoreForQuorum) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *stepStoreForQuorum) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

func quorumStore(guard *TransitionGuard) *stepStoreForQuorum {
	return &stepStoreForQuorum{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToNext, Guard: guard},
				},
			},
		},
		next:    StepSpec{ID: "approval", Position: 2},
		applied: map[string]bool{},
	}
}

func TestApplyThreshold_AllApprove(t *testing.T) {
	required := []ParticipantInfo{
		{ID: "p1"}, {ID: "p2"},
	}
	cases := []struct {
		name      string
		decisions []DecisionInfo
		want      bool
	}{
		{"all approved", []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionApproved}}, true},
		{"partial approved", []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}, false},
		{"none yet", nil, false},
		{"approve+reject", []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionRejected}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := applyThreshold(QuorumAllApprove, required, tc.decisions); got != tc.want {
				t.Fatalf("applyThreshold all_approve = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyThreshold_AllDecide(t *testing.T) {
	required := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}}
	d1 := []DecisionInfo{{ParticipantID: "p1", Decision: "approved"}, {ParticipantID: "p2", Decision: "rejected"}}
	if !applyThreshold(QuorumAllDecide, required, d1) {
		t.Fatalf("all_decide should be true when both decided")
	}
	d2 := []DecisionInfo{{ParticipantID: "p1", Decision: "approved"}}
	if applyThreshold(QuorumAllDecide, required, d2) {
		t.Fatalf("all_decide should be false when only one decided")
	}
}

func TestApplyThreshold_AnyReject(t *testing.T) {
	required := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}}
	d := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionRejected}}
	if !applyThreshold(QuorumAnyReject, required, d) {
		t.Fatalf("any_reject should be true when one rejected")
	}
	if applyThreshold(QuorumAnyReject, required, []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}) {
		t.Fatalf("any_reject should be false when no rejection")
	}
}

func TestApplyThreshold_MajorityApprove(t *testing.T) {
	required := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}
	// 2/3 approve => majority
	d := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionApproved}}
	if !applyThreshold(QuorumMajorityApprove, required, d) {
		t.Fatalf("majority_approve true expected for 2/3 approves")
	}
	// 1/3 approve, 1 reject => not majority
	d2 := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionRejected}}
	if applyThreshold(QuorumMajorityApprove, required, d2) {
		t.Fatalf("majority_approve false expected when not strictly more than half")
	}
}

func TestApplyThreshold_NApprove(t *testing.T) {
	required := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}
	d := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionApproved}}
	if !applyThreshold("n_approve:2", required, d) {
		t.Fatalf("n_approve:2 expected true")
	}
	if applyThreshold("n_approve:3", required, d) {
		t.Fatalf("n_approve:3 expected false")
	}
	if applyThreshold("n_approve:notanint", required, d) {
		t.Fatalf("malformed n_approve threshold should fail closed")
	}
	if applyThreshold("n_approve:0", required, d) {
		t.Fatalf("n_approve:0 should fail closed")
	}
}

func TestApplyThreshold_RemovedParticipantDropped(t *testing.T) {
	// p1 was required and approved, then removed. p2 still required and approved.
	required := []ParticipantInfo{{ID: "p2"}}
	decisions := []DecisionInfo{
		{ParticipantID: "p1", Decision: DecisionApproved},
		{ParticipantID: "p2", Decision: DecisionApproved},
	}
	if !applyThreshold(QuorumAllApprove, required, decisions) {
		t.Fatalf("all_approve should be true when removed participant's decision is ignored")
	}
}

func TestApplyThreshold_LatestDecisionPerParticipant(t *testing.T) {
	required := []ParticipantInfo{{ID: "p1"}}
	// p1 approved then rejected
	decisions := []DecisionInfo{
		{ParticipantID: "p1", Decision: DecisionApproved},
		{ParticipantID: "p1", Decision: DecisionRejected},
	}
	if applyThreshold(QuorumAllApprove, required, decisions) {
		t.Fatalf("expected latest reject to override earlier approve")
	}
	if !applyThreshold(QuorumAnyReject, required, decisions) {
		t.Fatalf("any_reject should be true given latest rejection")
	}
}

func TestApplyThreshold_NoRequiredParticipants_FailsClosedHandledByCaller(t *testing.T) {
	// applyThreshold itself, with empty required and a non-N threshold,
	// returns true for all_approve (vacuously true). Engine wraps this and
	// fails closed when len(required) == 0 — pinned in TestWaitForQuorum_*.
	if !applyThreshold(QuorumAllApprove, nil, nil) {
		t.Fatalf("documented behavior: applyThreshold returns true for vacuous all_approve")
	}
}

func TestEngine_WaitForQuorum_BlocksUntilSatisfied(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
		{ID: "p2", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-B"},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("expected guard to block transition with no decisions")
	}

	// Record one approval - still not enough for all_approve.
	if err := decisions.RecordStepDecision(context.Background(), DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	res, err = eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("expected guard to block transition with partial decisions")
	}

	// Record second approval - quorum reached.
	if err := decisions.RecordStepDecision(context.Background(), DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p2", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	res, err = eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Transitioned {
		t.Fatalf("expected transition once quorum satisfied")
	}
	if res.ToStepID != "approval" {
		t.Fatalf("expected target = approval, got %q", res.ToStepID)
	}
}

func TestEngine_WaitForQuorum_NoRequiredParticipants_FailsClosed(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	decisions := newFakeDecisionStore()
	// All participants present but DecisionRequired=false.
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: false, AgentProfileID: "rev-A"},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))
	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("expected guard to fail closed when no required participants exist")
	}
}

func TestEngine_WaitForQuorum_NoStoresWired_FailsClosed(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	eng := New(store, MapRegistry{}) // no DecisionStore / ParticipantStore
	res, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Transitioned {
		t.Fatalf("expected guard to fail closed when stores not wired")
	}
}

func TestClearDecisionsCallback_DeletesRows(t *testing.T) {
	decisions := newFakeDecisionStore()
	_ = decisions.RecordStepDecision(context.Background(), DecisionInfo{TaskID: "t", StepID: "s", ParticipantID: "p", Decision: "approved"})
	cb := ClearDecisionsCallback{Decisions: decisions}
	_, err := cb.Execute(context.Background(), ActionInput{
		State:  MachineState{TaskID: "t"},
		Step:   StepSpec{ID: "s"},
		Action: Action{Kind: ActionClearDecisions, ClearDecisions: &ClearDecisionsAction{}},
	})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got, _ := decisions.ListStepDecisions(context.Background(), "t", "s"); len(got) != 0 {
		t.Fatalf("expected decisions cleared, got %d", len(got))
	}
	if decisions.cleared[dkey("t", "s")] != 1 {
		t.Fatalf("expected cleared count for (t,s) to be 1, got %d", decisions.cleared[dkey("t", "s")])
	}
}

func TestClearDecisionsCallback_NoStore_Errors(t *testing.T) {
	cb := ClearDecisionsCallback{}
	_, err := cb.Execute(context.Background(), ActionInput{
		Action: Action{Kind: ActionClearDecisions, ClearDecisions: &ClearDecisionsAction{}},
	})
	if err == nil || !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestEngine_RecordParticipantDecision_PersistsAndReevaluates(t *testing.T) {
	store := quorumStore(&TransitionGuard{
		WaitForQuorum: &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove},
	})
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	if err := eng.RecordParticipantDecision(context.Background(), "task-1", "sess-1", "review", "p1", DecisionApproved, "lgtm"); err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	got, _ := decisions.ListStepDecisions(context.Background(), "task-1", "review")
	if len(got) != 1 {
		t.Fatalf("expected 1 recorded decision, got %d", len(got))
	}
	if got[0].Decision != DecisionApproved || got[0].Note != "lgtm" {
		t.Fatalf("unexpected decision shape: %#v", got[0])
	}
}

func TestEngine_RecordParticipantDecision_RequiresStore(t *testing.T) {
	store := quorumStore(nil)
	eng := New(store, MapRegistry{}) // no DecisionStore wired
	err := eng.RecordParticipantDecision(context.Background(), "task-1", "sess-1", "review", "p1", DecisionApproved, "")
	if err == nil {
		t.Fatalf("expected error when DecisionStore missing")
	}
}

func TestEngine_RecordParticipantDecision_RequiresIDs(t *testing.T) {
	decisions := newFakeDecisionStore()
	eng := New(quorumStore(nil), MapRegistry{}, WithDecisionStore(decisions))
	cases := []struct {
		name                    string
		task, step, participant string
		decision                string
		expectErr               bool
	}{
		{"missing task", "", "step", "p", "approved", true},
		{"missing step", "task", "", "p", "approved", true},
		{"missing participant", "task", "step", "", "approved", true},
		{"missing decision", "task", "step", "p", "", true},
		{"valid (no session => skip reeval)", "task", "step", "p", "approved", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Signature: (ctx, taskID, sessionID, stepID, participantID, decision, note)
			err := eng.RecordParticipantDecision(context.Background(), tc.task, "", tc.step, tc.participant, tc.decision, "")
			if (err != nil) != tc.expectErr {
				t.Fatalf("err=%v, expectErr=%v", err, tc.expectErr)
			}
		})
	}
}

func TestConfigTransitionGuard_ParsesWaitForQuorum(t *testing.T) {
	cfg := map[string]any{
		"wait_for_quorum": map[string]any{
			"role":      "reviewer",
			"threshold": "all_approve",
		},
	}
	g := ConfigTransitionGuard(cfg)
	if g == nil || g.WaitForQuorum == nil {
		t.Fatalf("expected guard, got %#v", g)
	}
	if g.WaitForQuorum.Role != "reviewer" || g.WaitForQuorum.Threshold != "all_approve" {
		t.Fatalf("unexpected guard: %#v", g.WaitForQuorum)
	}
	if got := ConfigTransitionGuard(nil); got != nil {
		t.Fatalf("expected nil for nil config")
	}
	if got := ConfigTransitionGuard(map[string]any{}); got != nil {
		t.Fatalf("expected nil when key absent")
	}
	// Missing fields => no guard.
	if got := ConfigTransitionGuard(map[string]any{"wait_for_quorum": map[string]any{"role": ""}}); got != nil {
		t.Fatalf("expected nil for malformed guard")
	}
}
