package engine

import (
	"context"
	"errors"
	"expvar"
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
func (s *stepStoreForQuorum) ApplyTransitionIfAtStep(
	_ context.Context, _, _, expectedStepID, toStepID string, _ Trigger,
) (bool, error) {
	if s.state.CurrentStepID != expectedStepID {
		return false, nil
	}
	s.state.CurrentStepID = toStepID
	return true, nil
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

// scopedParticipants distinguishes per-task rows from step-template rows,
// unlike the shared fakeParticipants (which returns the same static list
// for both). Required for tests that exercise AC-50's gather/canonicalize/
// collapse distinctions.
type scopedParticipants struct {
	perTask  []ParticipantInfo
	template []ParticipantInfo
	err      error
}

func (s scopedParticipants) ListTaskParticipants(_ context.Context, taskID string) ([]ParticipantInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	out := make([]ParticipantInfo, 0, len(s.perTask))
	for _, p := range s.perTask {
		if p.TaskID == taskID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s scopedParticipants) ListStepParticipants(_ context.Context, stepID, taskID string) ([]ParticipantInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	if taskID != "" {
		return nil, nil
	}
	out := make([]ParticipantInfo, 0, len(s.template))
	for _, p := range s.template {
		if p.StepID == stepID {
			out = append(out, p)
		}
	}
	return out, nil
}

func quorumEngine(decisions DecisionStore, participants ParticipantStore) *Engine {
	return New(quorumStore(nil), MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(participants))
}

// --- evaluateApproveStyle (AC-21/39/40/41/43a/53) ---

func TestEvaluateApproveStyle_AllApprove(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}}
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
			got := evaluateApproveStyle(QuorumAllApprove, seats, tc.decisions)
			if got.Satisfied != tc.want {
				t.Fatalf("all_approve satisfied = %v, want %v (reason=%s)", got.Satisfied, tc.want, got.Reason)
			}
			if !got.Satisfied && got.Reason != ReasonThresholdNotMet {
				t.Fatalf("expected reason %s, got %s", ReasonThresholdNotMet, got.Reason)
			}
		})
	}
}

func TestEvaluateApproveStyle_AllDecide(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}}
	d1 := []DecisionInfo{{ParticipantID: "p1", Decision: "approved"}, {ParticipantID: "p2", Decision: "rejected"}}
	if !evaluateApproveStyle(QuorumAllDecide, seats, d1).Satisfied {
		t.Fatalf("all_decide should be true when both decided")
	}
	d2 := []DecisionInfo{{ParticipantID: "p1", Decision: "approved"}}
	if evaluateApproveStyle(QuorumAllDecide, seats, d2).Satisfied {
		t.Fatalf("all_decide should be false when only one decided")
	}
}

func TestEvaluateApproveStyle_MajorityApprove(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}
	// 2/3 approve => majority
	d := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionApproved}}
	if !evaluateApproveStyle(QuorumMajorityApprove, seats, d).Satisfied {
		t.Fatalf("majority_approve true expected for 2/3 approves")
	}
	// 1/3 approve, 1 reject => not majority
	d2 := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionRejected}}
	if evaluateApproveStyle(QuorumMajorityApprove, seats, d2).Satisfied {
		t.Fatalf("majority_approve false expected when not strictly more than half")
	}
}

func TestEvaluateApproveStyle_NApprove(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}
	d := []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}, {ParticipantID: "p2", Decision: DecisionApproved}}
	if !evaluateApproveStyle("n_approve:2", seats, d).Satisfied {
		t.Fatalf("n_approve:2 expected true")
	}
	if evaluateApproveStyle("n_approve:3", seats, d).Satisfied {
		t.Fatalf("n_approve:3 expected false")
	}
	if got := evaluateApproveStyle("n_approve:notanint", seats, d); got.Satisfied || got.Reason != ReasonThresholdUnrecognized {
		t.Fatalf("malformed n_approve threshold should be threshold_unrecognized, got %+v", got)
	}
	if got := evaluateApproveStyle("n_approve:0", seats, d); got.Satisfied || got.Reason != ReasonThresholdUnrecognized {
		t.Fatalf("n_approve:0 should be threshold_unrecognized, got %+v", got)
	}
}

func TestEvaluateApproveStyle_NApprove_ExceedsSlate_Unsatisfiable(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}, {ID: "p2"}}
	got := evaluateApproveStyle("n_approve:5", seats, nil)
	if got.Satisfied || got.Reason != ReasonThresholdUnsatisfiable {
		t.Fatalf("n_approve:5 over a 2-seat slate should be threshold_unsatisfiable, got %+v", got)
	}
	if got.RequiredCount != 5 {
		t.Fatalf("expected RequiredCount=5, got %d", got.RequiredCount)
	}
}

func TestEvaluateApproveStyle_UnrecognizedThreshold(t *testing.T) {
	got := evaluateApproveStyle("quorum_of_the_moon", []ParticipantInfo{{ID: "p1"}}, nil)
	if got.Satisfied || got.Reason != ReasonThresholdUnrecognized {
		t.Fatalf("expected threshold_unrecognized, got %+v", got)
	}
}

func TestEvaluateApproveStyle_UnmappedDecisionDropped(t *testing.T) {
	// p1 is not (or no longer) a seat; its decision does not count.
	seats := []ParticipantInfo{{ID: "p2"}}
	decisions := []DecisionInfo{
		{ParticipantID: "p1", Decision: DecisionApproved},
		{ParticipantID: "p2", Decision: DecisionApproved},
	}
	if !evaluateApproveStyle(QuorumAllApprove, seats, decisions).Satisfied {
		t.Fatalf("all_approve should be true when the unmapped decision is ignored")
	}
}

func TestEvaluateApproveStyle_LatestDecisionPerSeat(t *testing.T) {
	seats := []ParticipantInfo{{ID: "p1"}}
	// p1 approved then rejected — AC-26 ordering means the later row wins.
	decisions := []DecisionInfo{
		{ParticipantID: "p1", Decision: DecisionApproved},
		{ParticipantID: "p1", Decision: DecisionRejected},
	}
	if evaluateApproveStyle(QuorumAllApprove, seats, decisions).Satisfied {
		t.Fatalf("expected latest reject to override earlier approve")
	}
}

func TestEvaluateApproveStyle_MapsByRoleAndAgentWhenParticipantIDUnset(t *testing.T) {
	// AC-51: an agent decider maps to its seat by (role, decider_id) when the
	// decision carries no seat id.
	seats := []ParticipantInfo{{ID: "p1", Role: "reviewer", AgentProfileID: "rev-A"}}
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "reviewer", Decision: DecisionApproved},
	}
	if !evaluateApproveStyle(QuorumAllApprove, seats, decisions).Satisfied {
		t.Fatalf("expected decider-identity mapping to satisfy all_approve")
	}
}

// --- evaluateAnyReject (AC-43/43a/43b/43c/58/59) ---

func TestEvaluateAnyReject_VetoOnRejection(t *testing.T) {
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "reviewer", Decision: DecisionApproved},
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-B", Role: "reviewer", Decision: DecisionRejected},
	}
	satisfied, received := evaluateAnyReject(decisions, "reviewer")
	if !satisfied || received != 1 {
		t.Fatalf("expected veto satisfied with 1 rejecter, got satisfied=%v received=%d", satisfied, received)
	}
}

func TestEvaluateAnyReject_NoRejection(t *testing.T) {
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "reviewer", Decision: DecisionApproved},
	}
	if satisfied, _ := evaluateAnyReject(decisions, "reviewer"); satisfied {
		t.Fatalf("any_reject should be false when no rejection")
	}
}

func TestEvaluateAnyReject_LatestDecisionOverrides(t *testing.T) {
	// Same decider rejects then later approves — AC-26 ordering means the
	// later row (approve) is the decider's current stance, so no veto.
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "reviewer", Decision: DecisionRejected},
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "reviewer", Decision: DecisionApproved},
	}
	if satisfied, _ := evaluateAnyReject(decisions, "reviewer"); satisfied {
		t.Fatalf("expected latest approve to clear the earlier veto")
	}
}

func TestEvaluateAnyReject_RoleScopedForAgent(t *testing.T) {
	// AC-42: an agent decider's rejection under a different role does not
	// veto this guard's role.
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeAgent, DeciderID: "rev-A", Role: "qa", Decision: DecisionRejected},
	}
	if satisfied, _ := evaluateAnyReject(decisions, "reviewer"); satisfied {
		t.Fatalf("agent rejection under a different role must not veto")
	}
}

func TestEvaluateAnyReject_RoleAgnosticForUser(t *testing.T) {
	// AC-58: a human decider's rejection vetoes regardless of stored role.
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeUser, DeciderID: "user-1", Role: "qa", Decision: DecisionRejected},
	}
	if satisfied, _ := evaluateAnyReject(decisions, "reviewer"); !satisfied {
		t.Fatalf("user rejection should veto regardless of role")
	}
}

func TestEvaluateAnyReject_ChangesRequestedCountsAsRejection(t *testing.T) {
	decisions := []DecisionInfo{
		{DeciderType: DeciderTypeUser, DeciderID: "user-1", Role: "reviewer", Decision: DecisionChangesRequested},
	}
	if satisfied, _ := evaluateAnyReject(decisions, "reviewer"); !satisfied {
		t.Fatalf("changes_requested should be treated as a rejection veto")
	}
}

func TestEvaluateAnyReject_EmptyDecisions(t *testing.T) {
	if satisfied, received := evaluateAnyReject(nil, "reviewer"); satisfied || received != 0 {
		t.Fatalf("expected no veto with no decisions, got satisfied=%v received=%d", satisfied, received)
	}
}

// --- requiredSeats / canonicalize / collapse (AC-20/44/49/50) ---

func TestRequiredSeats_FiltersByRoleAndDecisionRequired(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		{ID: "p2", StepID: "review", Role: "reviewer", AgentProfileID: "rev-B", DecisionRequired: false},
		{ID: "p3", StepID: "review", Role: "qa", AgentProfileID: "qa-A", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 1 || seats[0].ID != "p1" {
		t.Fatalf("expected only p1 to survive role+decision_required filtering, got %+v", seats)
	}
}

func TestRequiredSeats_PerTaskWinsOverTemplateOnCollapse(t *testing.T) {
	// A template seat for rev-A at the evaluating step, and a per-task
	// override for rev-A recorded at a different step (AC-49: per-task
	// participation is not step-scoped). Collapse must produce exactly one
	// seat, carrying the per-task row.
	parts := scopedParticipants{
		template: []ParticipantInfo{
			{ID: "tmpl-1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		},
		perTask: []ParticipantInfo{
			{ID: "task-row-1", TaskID: "task-1", StepID: "approval", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		},
	}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 1 {
		t.Fatalf("expected exactly one collapsed seat, got %d: %+v", len(seats), seats)
	}
	if seats[0].ID != "task-row-1" {
		t.Fatalf("expected the per-task row to win collapse, got %+v", seats[0])
	}
}

func TestRequiredSeats_CanonicalizePrefersEvaluatingStep(t *testing.T) {
	// Two per-task rows for the same (task, role, agent) at different steps
	// — canonicalize must keep the one at the evaluating step.
	parts := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "row-other", TaskID: "task-1", StepID: "other", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		{ID: "row-review", TaskID: "task-1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 1 || seats[0].ID != "row-review" {
		t.Fatalf("expected canonicalize to prefer the evaluating-step row, got %+v", seats)
	}
}

func TestRequiredSeats_CanonicalizeFallsBackToLowestID(t *testing.T) {
	// Neither duplicate row is at the evaluating step — fall back to the
	// lowest id in ASCII order.
	parts := scopedParticipants{perTask: []ParticipantInfo{
		{ID: "row-z", TaskID: "task-1", StepID: "other-b", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		{ID: "row-a", TaskID: "task-1", StepID: "other-a", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 1 || seats[0].ID != "row-a" {
		t.Fatalf("expected canonicalize to fall back to lowest id, got %+v", seats)
	}
}

func TestRequiredSeats_EmptyAgentProfileNeverGrouped(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "", DecisionRequired: true},
		{ID: "p2", StepID: "review", Role: "reviewer", AgentProfileID: "", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 2 {
		t.Fatalf("expected empty-agent_profile_id rows to remain distinct seats, got %d: %+v", len(seats), seats)
	}
}

func TestRequiredSeats_EmptySlate(t *testing.T) {
	parts := scopedParticipants{}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	seats, err := eng.requiredSeats(context.Background(), "review", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeats: %v", err)
	}
	if len(seats) != 0 {
		t.Fatalf("expected empty slate, got %+v", seats)
	}
}

// --- computeGuardOutcome / evaluateTransitionGuard precedence (AC-23/52) ---

func TestEvaluateTransitionGuard_NilGuardPermits(t *testing.T) {
	eng := quorumEngine(newFakeDecisionStore(), scopedParticipants{})
	got := eng.evaluateTransitionGuard(context.Background(), MachineState{}, Action{Kind: ActionMoveToNext})
	if !got.Satisfied {
		t.Fatalf("nil guard should always permit the transition")
	}
}

func TestEvaluateTransitionGuard_UnrecognizedGuardVariant(t *testing.T) {
	eng := quorumEngine(newFakeDecisionStore(), scopedParticipants{})
	got := eng.evaluateTransitionGuard(context.Background(), MachineState{}, Action{
		Kind:  ActionMoveToNext,
		Guard: &TransitionGuard{},
	})
	if got.Satisfied || got.Reason != ReasonGuardVariantUnrecognized {
		t.Fatalf("expected guard_variant_unrecognized, got %+v", got)
	}
}

func TestComputeGuardOutcome_DecisionStoreUnwired(t *testing.T) {
	eng := New(quorumStore(nil), MapRegistry{}, WithParticipantStore(scopedParticipants{}))
	got := eng.computeGuardOutcome(context.Background(), MachineState{}, &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if got.Satisfied || got.Reason != ReasonDecisionStoreUnwired {
		t.Fatalf("expected decision_store_unwired, got %+v", got)
	}
}

func TestComputeGuardOutcome_ParticipantStoreUnwired(t *testing.T) {
	eng := New(quorumStore(nil), MapRegistry{}, WithDecisionStore(newFakeDecisionStore()))
	got := eng.computeGuardOutcome(context.Background(), MachineState{}, &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if got.Satisfied || got.Reason != ReasonParticipantStoreUnwired {
		t.Fatalf("expected participant_store_unwired, got %+v", got)
	}
}

func TestComputeGuardOutcome_SlateEmpty(t *testing.T) {
	eng := quorumEngine(newFakeDecisionStore(), scopedParticipants{})
	state := MachineState{TaskID: "task-1", CurrentStepID: "review"}
	got := eng.computeGuardOutcome(context.Background(), state, &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if got.Satisfied || got.Reason != ReasonSlateEmpty {
		t.Fatalf("expected slate_empty, got %+v", got)
	}
}

func readQuorumSlateEmptyCounter(role string) int64 {
	m := expvar.Get("workflow_quorum_slate_empty_total")
	if m == nil {
		return 0
	}
	kv := m.(*expvar.Map).Get(role)
	if kv == nil {
		return 0
	}
	iv, ok := kv.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// TestComputeGuardOutcome_MalformedRoleSanitizesSlateEmptyCounterLabel covers
// AC-OFFICE-REVIEW-SEATS-004.6/.11 on the slate_empty path: guard.Role is
// operator-editable step config, and a value outside the fixed
// ParticipantRole set must fold to the SeatRoleLabelInvalid sentinel on the
// counter rather than growing its label cardinality with an arbitrary
// string.
func TestComputeGuardOutcome_MalformedRoleSanitizesSlateEmptyCounterLabel(t *testing.T) {
	const malformedRole = "not-a-real-role"
	eng := quorumEngine(newFakeDecisionStore(), scopedParticipants{})
	state := MachineState{TaskID: "task-1", CurrentStepID: "review"}

	before := readQuorumSlateEmptyCounter(SeatRoleLabelInvalid)
	got := eng.computeGuardOutcome(context.Background(), state, &WaitForQuorumGuard{Role: malformedRole, Threshold: QuorumAllApprove})
	if got.Satisfied || got.Reason != ReasonSlateEmpty {
		t.Fatalf("expected slate_empty, got %+v", got)
	}
	after := readQuorumSlateEmptyCounter(SeatRoleLabelInvalid)
	if after-before != 1 {
		t.Fatalf("%s counter delta = %d, want 1", SeatRoleLabelInvalid, after-before)
	}
	if got := readQuorumSlateEmptyCounter(malformedRole); got != 0 {
		t.Fatalf("expected no counter entry under the raw operator string %q, got %d", malformedRole, got)
	}
}

func TestComputeGuardOutcome_EvaluationError(t *testing.T) {
	parts := scopedParticipants{err: errors.New("boom")}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	state := MachineState{TaskID: "task-1", CurrentStepID: "review"}
	got := eng.computeGuardOutcome(context.Background(), state, &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if got.Satisfied || got.Reason != ReasonEvaluationError || got.Err == nil {
		t.Fatalf("expected evaluation_error with a wrapped error, got %+v", got)
	}
}

func TestComputeGuardOutcome_AnyReject_NoSlateNeeded(t *testing.T) {
	// AC-59: any_reject must not short-circuit on an empty seat slate.
	decisions := newFakeDecisionStore()
	if err := decisions.RecordStepDecision(context.Background(), DecisionInfo{
		TaskID: "task-1", StepID: "review",
		DeciderType: DeciderTypeUser, DeciderID: "user-1", Role: "reviewer",
		Decision: DecisionRejected,
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	eng := quorumEngine(decisions, scopedParticipants{}) // no seats configured at all
	state := MachineState{TaskID: "task-1", CurrentStepID: "review"}
	got := eng.computeGuardOutcome(context.Background(), state, &WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAnyReject})
	if !got.Satisfied {
		t.Fatalf("expected any_reject veto to fire despite an empty slate, got %+v", got)
	}
}

func TestComputeGuardOutcome_ThresholdUnrecognizedTakesPrecedenceOverThresholdNotMet(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)
	state := MachineState{TaskID: "task-1", CurrentStepID: "review"}
	got := eng.computeGuardOutcome(context.Background(), state, &WaitForQuorumGuard{Role: "reviewer", Threshold: "not_a_real_threshold"})
	if got.Satisfied || got.Reason != ReasonThresholdUnrecognized {
		t.Fatalf("expected threshold_unrecognized, got %+v", got)
	}
}

// --- Engine-level integration (unchanged from the pre-rewrite suite) ---

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

	result, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved, Note: "lgtm",
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if result.DecisionID == "" {
		t.Fatalf("expected a generated decision id")
	}
	if result.DecidedAt.IsZero() {
		t.Fatalf("expected DecidedAt to be stamped")
	}
	if !result.Transitioned || result.TransitionAbandoned {
		t.Fatalf("expected the sole reviewer's approval to satisfy all_approve and transition: %#v", result)
	}
	if result.FromStepID != "review" || result.ToStepID != "approval" {
		t.Fatalf("unexpected transition endpoints: %#v", result)
	}
	if len(result.Guards) != 1 || !result.Guards[0].Satisfied || result.Guards[0].TargetStepID != "approval" {
		t.Fatalf("expected the validated guard snapshot on transition, got %#v", result.Guards)
	}
	if store.state.CurrentStepID != "approval" {
		t.Fatalf("expected store to have moved to 'approval', got %q", store.state.CurrentStepID)
	}

	got, _ := decisions.ListStepDecisions(context.Background(), "task-1", "review")
	if len(got) != 1 {
		t.Fatalf("expected 1 recorded decision, got %d", len(got))
	}
	if got[0].Decision != DecisionApproved || got[0].Note != "lgtm" {
		t.Fatalf("unexpected decision shape: %#v", got[0])
	}
	if got[0].ID != result.DecisionID || !got[0].DecidedAt.Equal(result.DecidedAt) {
		t.Fatalf("persisted decision does not carry the id/DecidedAt returned to the caller: %#v", got[0])
	}
}

func TestEngine_RecordParticipantDecision_RequiresStore(t *testing.T) {
	store := quorumStore(nil)
	eng := New(store, MapRegistry{}) // no DecisionStore wired
	_, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	})
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
			result, err := eng.RecordParticipantDecision(context.Background(), "", DecisionInfo{
				TaskID: tc.task, StepID: tc.step, ParticipantID: tc.participant, Decision: tc.decision,
			})
			if (err != nil) != tc.expectErr {
				t.Fatalf("err=%v, expectErr=%v", err, tc.expectErr)
			}
			if err == nil && !result.ReevaluationSkipped {
				t.Fatalf("expected ReevaluationSkipped with a blank sessionID: %#v", result)
			}
			if err == nil && result.SkipReason != ReasonSessionUnresolvable {
				t.Fatalf("expected SkipReason=%s, got %q", ReasonSessionUnresolvable, result.SkipReason)
			}
		})
	}
}

func TestConfigTransitionGuard_ParsesWaitForQuorum(t *testing.T) {
	cfg := map[string]any{
		"if": map[string]any{
			"wait_for_quorum": map[string]any{
				"role":      "reviewer",
				"threshold": "all_approve",
			},
		},
	}
	g := ConfigTransitionGuard(cfg)
	if g == nil || g.WaitForQuorum == nil {
		t.Fatalf("expected wait_for_quorum guard")
	}
	if g.WaitForQuorum.Role != "reviewer" || g.WaitForQuorum.Threshold != "all_approve" {
		t.Fatalf("unexpected guard: %+v", g.WaitForQuorum)
	}
}

func TestConfigTransitionGuard_ParsesLegacyTopLevelWaitForQuorum(t *testing.T) {
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
