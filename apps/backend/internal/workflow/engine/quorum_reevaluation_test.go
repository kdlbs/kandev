package engine

import (
	"context"
	"expvar"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// reevalFakeStore is a TransitionStore whose ApplyTransitionIfAtStep call
// count and outcome are test-controllable, for AC-46/47/48/65/66 coverage
// that stepStoreForQuorum (single guarded action only) cannot express.
type reevalFakeStore struct {
	state   MachineState
	step    StepSpec
	applied map[string]bool

	casCalls   int
	casApplied func(expectedStepID, toStepID string) (bool, error)
}

func (s *reevalFakeStore) LoadState(_ context.Context, _, _ string) (MachineState, error) {
	return s.state, nil
}
func (s *reevalFakeStore) LoadStep(_ context.Context, _, _ string) (StepSpec, error) {
	return s.step, nil
}
func (s *reevalFakeStore) LoadNextStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *reevalFakeStore) LoadPreviousStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *reevalFakeStore) ApplyTransition(_ context.Context, _, _, _, _ string, _ Trigger) error {
	return nil
}
func (s *reevalFakeStore) ApplyTransitionIfAtStep(
	_ context.Context, _, _, expectedStepID, toStepID string, _ Trigger,
) (bool, error) {
	s.casCalls++
	if s.casApplied != nil {
		return s.casApplied(expectedStepID, toStepID)
	}
	if s.state.CurrentStepID != expectedStepID {
		return false, nil
	}
	s.state.CurrentStepID = toStepID
	return true, nil
}
func (s *reevalFakeStore) PersistData(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *reevalFakeStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *reevalFakeStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

func approvedGuard(role string) *TransitionGuard {
	return &TransitionGuard{WaitForQuorum: &WaitForQuorumGuard{Role: role, Threshold: QuorumAllApprove}}
}

// --- AC-46/48: lost CAS race is reported as abandoned, not an error ---

func TestReevaluateGuardedTransitions_AbandonsOnLostCASRace(t *testing.T) {
	store := &reevalFakeStore{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToStep, Guard: approvedGuard("reviewer"), MoveToStep: &MoveToStepAction{StepID: "approval"}},
				},
			},
		},
		applied:    map[string]bool{},
		casApplied: func(string, string) (bool, error) { return false, nil }, // simulate a concurrent mover winning the race
	}
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	result, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if !result.TransitionAbandoned || result.Transitioned {
		t.Fatalf("expected TransitionAbandoned=true, Transitioned=false, got %#v", result)
	}
	if result.FromStepID != "review" || result.ToStepID != "approval" {
		t.Fatalf("unexpected abandoned transition endpoints: %#v", result)
	}
	if store.casCalls != 1 {
		t.Fatalf("expected exactly one CAS attempt, got %d", store.casCalls)
	}
}

// --- AC-30/66: re-evaluation of the same decision is idempotent ---

func TestReevaluateGuardedTransitions_IdempotentOnRepeatedDecisionID(t *testing.T) {
	store := &reevalFakeStore{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToStep, Guard: approvedGuard("reviewer"), MoveToStep: &MoveToStepAction{StepID: "approval"}},
				},
			},
		},
		applied: map[string]bool{},
	}
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	if err := decisions.RecordStepDecision(context.Background(), DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	first, err := eng.reevaluateGuardedTransitions(context.Background(), "task-1", "sess-1", "review", "decision-1")
	if err != nil {
		t.Fatalf("first reevaluate: %v", err)
	}
	if !first.Transitioned {
		t.Fatalf("expected first call to transition, got %#v", first)
	}
	if store.casCalls != 1 {
		t.Fatalf("expected 1 CAS call after first reevaluate, got %d", store.casCalls)
	}

	second, err := eng.reevaluateGuardedTransitions(context.Background(), "task-1", "sess-1", "review", "decision-1")
	if err != nil {
		t.Fatalf("second reevaluate: %v", err)
	}
	if !second.Idempotent {
		t.Fatalf("expected second call with the same decision id to be idempotent, got %#v", second)
	}
	if store.casCalls != 1 {
		t.Fatalf("expected no additional CAS call on the idempotent replay, got %d calls", store.casCalls)
	}
}

// --- AC-65: only guarded transition actions are in scope for re-evaluation ---

func TestApplyFirstSatisfiedGuardedTransition_SkipsNonTransitionAndUngatedActions(t *testing.T) {
	store := &reevalFakeStore{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					// Non-transition action: must never be inspected or executed
					// by the re-evaluation path (it already ran once for the
					// turn; MapRegistry{} below has no callback wired for it,
					// so executing it would panic/error on lookup failure).
					{Kind: ActionSetWorkflowData, SetWorkflowData: &SetWorkflowDataAction{Key: "k", Value: "v"}},
					// Ungated transition: out of AC-65 scope, must not re-fire.
					{Kind: ActionMoveToStep, MoveToStep: &MoveToStepAction{StepID: "ungated-target"}},
					// The only in-scope action.
					{Kind: ActionMoveToStep, Guard: approvedGuard("reviewer"), MoveToStep: &MoveToStepAction{StepID: "guarded-target"}},
				},
			},
		},
		applied: map[string]bool{},
	}
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	result, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if !result.Transitioned || result.ToStepID != "guarded-target" {
		t.Fatalf("expected transition to the guarded target only, got %#v", result)
	}
	if store.casCalls != 1 {
		t.Fatalf("expected exactly one CAS attempt (the guarded action), got %d", store.casCalls)
	}
}

// --- AC-24/24a: guard-not-fired and session-unresolvable both log and count ---

func newObservedEngine(store TransitionStore, decisions DecisionStore, participants ParticipantStore) (*Engine, *observer.ObservedLogs) {
	core, logs := observer.New(zap.WarnLevel)
	zapLogger := zap.New(core)
	log, err := logger.NewFromZap(zapLogger)
	if err != nil {
		panic(err)
	}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(participants), WithLogger(log))
	return eng, logs
}

func readGuardCounter(reason string) int64 {
	m := expvar.Get("workflow_quorum_guard_not_fired_total")
	if m == nil {
		return 0
	}
	kv := m.(*expvar.Map).Get(reason)
	if kv == nil {
		return 0
	}
	iv, ok := kv.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

func TestGuardNotFired_LogsAndCounts_OnOrdinaryHandleTriggerPath(t *testing.T) {
	store := quorumStore(approvedGuard("reviewer"))
	decisions := newFakeDecisionStore() // no decisions recorded => threshold not met
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	eng, logs := newObservedEngine(store, decisions, parts)

	before := readGuardCounter(ReasonThresholdNotMet)
	_, err := eng.HandleTrigger(context.Background(), HandleInput{
		TaskID: "task-1", SessionID: "sess-1", Trigger: TriggerOnTurnComplete,
	})
	if err != nil {
		t.Fatalf("HandleTrigger: %v", err)
	}
	if got := readGuardCounter(ReasonThresholdNotMet); got != before+1 {
		t.Fatalf("expected threshold_not_met counter to increment by 1, got %d -> %d", before, got)
	}
	entries := logs.FilterMessage("workflow quorum guard did not fire").All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one guard-not-fired log entry, got %d", len(entries))
	}
}

func TestGuardNotFired_LogsAndCounts_OnReevaluationPath(t *testing.T) {
	store := &reevalFakeStore{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					// Requires a second reviewer role that never decides, so this
					// guard evaluates and does not fire during re-evaluation.
					{Kind: ActionMoveToStep, Guard: approvedGuard("approver"), MoveToStep: &MoveToStepAction{StepID: "elsewhere"}},
				},
			},
		},
		applied: map[string]bool{},
	}
	decisions := newFakeDecisionStore()
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "approver", DecisionRequired: true, AgentProfileID: "app-A"},
		{ID: "p2", Role: "approver", DecisionRequired: true, AgentProfileID: "app-B"},
	}}
	eng, logs := newObservedEngine(store, decisions, parts)

	before := readGuardCounter(ReasonThresholdNotMet)
	result, err := eng.RecordParticipantDecision(context.Background(), "sess-1", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if result.Transitioned || result.TransitionAbandoned {
		t.Fatalf("expected no transition (only one of two approvers decided), got %#v", result)
	}
	if got := readGuardCounter(ReasonThresholdNotMet); got != before+1 {
		t.Fatalf("expected threshold_not_met counter to increment by 1 on the reevaluation path, got %d -> %d", before, got)
	}
	entries := logs.FilterMessage("workflow quorum guard did not fire").All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one guard-not-fired log entry from reevaluation, got %d", len(entries))
	}
}

func TestRecordReevaluationSkip_LogsAndCountsSessionUnresolvable(t *testing.T) {
	store := quorumStore(nil)
	decisions := newFakeDecisionStore()
	eng, logs := newObservedEngine(store, decisions, fakeParticipants{})

	before := readGuardCounter(ReasonSessionUnresolvable)
	result, err := eng.RecordParticipantDecision(context.Background(), "", DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	})
	if err != nil {
		t.Fatalf("RecordParticipantDecision: %v", err)
	}
	if !result.ReevaluationSkipped || result.SkipReason != ReasonSessionUnresolvable {
		t.Fatalf("expected a session_unresolvable skip, got %#v", result)
	}
	if got := readGuardCounter(ReasonSessionUnresolvable); got != before+1 {
		t.Fatalf("expected session_unresolvable counter to increment by 1, got %d -> %d", before, got)
	}
	entries := logs.FilterMessage("workflow quorum reevaluation skipped: session unresolvable").All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one reevaluation-skip log entry, got %d", len(entries))
	}
}

// --- AC-54/57d/60-62: EvaluateStepQuorum read-only diagnostic snapshot ---

func TestEvaluateStepQuorum_NoBoundStepReturnsEmptySnapshot(t *testing.T) {
	noStepStore := &reevalFakeStore{state: MachineState{WorkflowID: "wf"}, applied: map[string]bool{}}
	eng := New(noStepStore, MapRegistry{}, WithDecisionStore(newFakeDecisionStore()), WithParticipantStore(fakeParticipants{}))
	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil || snap.StepID != "" || len(snap.Guards) != 0 || snap.ReevaluationBlocked {
		t.Fatalf("expected empty snapshot with no error for blank CurrentStepID, got %#v, err=%v", snap, err)
	}
}

// TestEvaluateStepQuorum_BlankSessionStillReflectsReevaluationBlocked is
// AC-62: a task that has never had a session (sessionID == "", the F38/
// dispatcher case for a task with zero task_sessions rows) must still
// compute ReevaluationBlocked live from ListStepDecisions, not
// short-circuit to false before ever consulting decisions. stepStoreForQuorum's
// LoadState ignores its sessionID argument and returns the task's real
// CurrentStepID regardless, mirroring production: CurrentStepID is derived
// from the task row, not the session.
func TestEvaluateStepQuorum_BlankSessionStillReflectsReevaluationBlocked(t *testing.T) {
	store := quorumStore(approvedGuard("reviewer"))
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if snap.StepID != "review" {
		t.Fatalf("StepID = %q, want review", snap.StepID)
	}
	if !snap.ReevaluationBlocked {
		t.Fatalf("ReevaluationBlocked = false, want true: a decision is on record at the current step")
	}
}

func TestEvaluateStepQuorum_PerGuardEvaluationErrorDoesNotFailTheMethod(t *testing.T) {
	store := &stepStoreForQuorum{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					// move_to_step with no target: resolveTransitionTarget errors,
					// which must surface as this guard's evaluation_error entry,
					// not a method-level error.
					{Kind: ActionMoveToStep, Guard: approvedGuard("reviewer")},
				},
			},
		},
		applied: map[string]bool{},
	}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil {
		t.Fatalf("expected no method-level error, got %v", err)
	}
	if len(snap.Guards) != 1 {
		t.Fatalf("expected exactly one guard entry, got %d", len(snap.Guards))
	}
	if snap.Guards[0].Reason != ReasonEvaluationError || snap.Guards[0].Error == nil {
		t.Fatalf("expected the guard entry to carry evaluation_error, got %#v", snap.Guards[0])
	}
}

func TestEvaluateStepQuorum_ReevaluationBlockedReflectsOnlyDecisionsNonEmpty(t *testing.T) {
	store := quorumStore(approvedGuard("reviewer"))
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	decisions := newFakeDecisionStore()
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if snap.ReevaluationBlocked {
		t.Fatalf("expected ReevaluationBlocked=false with no decisions recorded")
	}

	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}
	snap2, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if !snap2.ReevaluationBlocked {
		t.Fatalf("expected ReevaluationBlocked=true once a decision exists at the current step")
	}
}

func TestEvaluateStepQuorum_GuardsOrderedByConfiguredActionOrderAndSatisfiedOmitsReason(t *testing.T) {
	store := &stepStoreForQuorum{
		state: MachineState{TaskID: "task-1", SessionID: "sess-1", WorkflowID: "wf", CurrentStepID: "review"},
		step: StepSpec{
			ID: "review", WorkflowID: "wf", Position: 1,
			Events: map[Trigger][]Action{
				TriggerOnTurnComplete: {
					{Kind: ActionMoveToStep, Guard: approvedGuard("approver"), MoveToStep: &MoveToStepAction{StepID: "second"}},
					{Kind: ActionMoveToStep, Guard: approvedGuard("reviewer"), MoveToStep: &MoveToStepAction{StepID: "first"}},
				},
			},
		},
		applied: map[string]bool{},
	}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
		{ID: "p2", Role: "approver", DecisionRequired: true, AgentProfileID: "app-A"},
	}}
	decisions := newFakeDecisionStore()
	decisions.byKey[dkey("task-1", "review")] = []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}
	eng := New(store, MapRegistry{}, WithDecisionStore(decisions), WithParticipantStore(parts))

	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if len(snap.Guards) != 2 {
		t.Fatalf("expected 2 guard entries, got %d", len(snap.Guards))
	}
	// Configured order: approver (unsatisfied, p2 never decided) then reviewer (satisfied).
	if snap.Guards[0].Role != "approver" || snap.Guards[0].Satisfied {
		t.Fatalf("expected first entry to be the unsatisfied approver guard, got %#v", snap.Guards[0])
	}
	if snap.Guards[0].Reason != ReasonThresholdNotMet {
		t.Fatalf("expected unsatisfied entry to carry threshold_not_met, got %q", snap.Guards[0].Reason)
	}
	if snap.Guards[1].Role != "reviewer" || !snap.Guards[1].Satisfied {
		t.Fatalf("expected second entry to be the satisfied reviewer guard, got %#v", snap.Guards[1])
	}
	if snap.Guards[1].Reason != "" {
		t.Fatalf("expected a satisfied guard entry to omit Reason (AC-60), got %q", snap.Guards[1].Reason)
	}
}

// TestEvaluateStepQuorum_DoesNotRecordSlateEmptySideEffects is the
// AC-004.10/AC-24b regression: EvaluateStepQuorum's doc comment promises "no
// AC-24a counter, no AC-24 log" for its read-only diagnostic snapshot, but a
// dashboard poll hits the same computeGuardOutcome/recordQuorumSlateEmpty
// code the committing HandleTrigger and reevaluation paths use. A guard
// whose required slate is empty must surface ReasonSlateEmpty in the
// snapshot entry without incrementing workflow_quorum_slate_empty_total or
// emitting the "workflow quorum required slate empty" warning log — in
// contrast to TestComputeGuardOutcome_SlateEmpty's committing-path sibling,
// which does record both.
func TestEvaluateStepQuorum_DoesNotRecordSlateEmptySideEffects(t *testing.T) {
	store := quorumStore(approvedGuard("reviewer"))
	decisions := newFakeDecisionStore()
	eng, logs := newObservedEngine(store, decisions, fakeParticipants{})

	before := readQuorumSlateEmptyCounter("reviewer")
	snap, err := eng.EvaluateStepQuorum(context.Background(), "task-1", "sess-1")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum: %v", err)
	}
	if len(snap.Guards) != 1 || snap.Guards[0].Reason != ReasonSlateEmpty {
		t.Fatalf("expected a single slate_empty guard entry, got %#v", snap.Guards)
	}
	if after := readQuorumSlateEmptyCounter("reviewer"); after != before {
		t.Fatalf("expected no slate_empty counter delta from a read-only snapshot, got %d -> %d", before, after)
	}
	if entries := logs.FilterMessage("workflow quorum required slate empty").All(); len(entries) != 0 {
		t.Fatalf("expected no slate-empty warning log from a read-only snapshot, got %d: %+v", len(entries), entries)
	}
}

// --- B1 companion: EvaluateStepQuorum must not depend on WHICH session ---

// sessionScopedStateStore mirrors production assembleMachineState
// (orchestrator/event_handlers_workflow.go): CurrentStepID and WorkflowID are
// derived from the TASK row and are therefore identical for every session,
// while SessionID, SessionState, Data and IsPassthrough are genuinely
// per-session — Data being loaded from that session's own
// metadata["workflow_data"] bag.
type sessionScopedStateStore struct {
	step              StepSpec
	dataBySess        map[string]map[string]any
	stateBySess       map[string]string
	passthroughBySess map[string]bool
	loadedData        []map[string]any
	loadedStates      []string
	loadedPassthrough []bool
	applied           map[string]bool
}

func (s *sessionScopedStateStore) LoadState(_ context.Context, taskID, sessionID string) (MachineState, error) {
	data := s.dataBySess[sessionID]
	sessionState := s.stateBySess[sessionID]
	isPassthrough := s.passthroughBySess[sessionID]
	s.loadedData = append(s.loadedData, data)
	s.loadedStates = append(s.loadedStates, sessionState)
	s.loadedPassthrough = append(s.loadedPassthrough, isPassthrough)
	return MachineState{
		TaskID:        taskID,
		SessionID:     sessionID,
		WorkflowID:    "wf",
		CurrentStepID: "review",
		SessionState:  sessionState,
		IsPassthrough: isPassthrough,
		Data:          data,
	}, nil
}
func (s *sessionScopedStateStore) LoadStep(_ context.Context, _, _ string) (StepSpec, error) {
	return s.step, nil
}
func (s *sessionScopedStateStore) LoadNextStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{ID: "approval", Position: 2}, nil
}
func (s *sessionScopedStateStore) LoadPreviousStep(_ context.Context, _ string, _ int) (StepSpec, error) {
	return StepSpec{}, nil
}
func (s *sessionScopedStateStore) ApplyTransition(_ context.Context, _, _, _, _ string, _ Trigger) error {
	return nil
}
func (s *sessionScopedStateStore) ApplyTransitionIfAtStep(
	_ context.Context, _, _, _, _ string, _ Trigger,
) (bool, error) {
	return false, nil
}
func (s *sessionScopedStateStore) PersistData(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (s *sessionScopedStateStore) IsOperationApplied(_ context.Context, op string) (bool, error) {
	return s.applied[op], nil
}
func (s *sessionScopedStateStore) MarkOperationApplied(_ context.Context, op string) error {
	s.applied[op] = true
	return nil
}

// TestEvaluateStepQuorum_InsensitiveToWhichLiveSessionIsNewest pins the
// invariant that lets engine_dispatcher's resolveLatestSessionID stay
// task-scoped ("the task's most recent session, any state") once
// features.officeSessionIdentity gives each participant agent its own
// session per task and a task therefore has SEVERAL live sessions.
//
// Today EvaluateStepQuorum's guard evaluation reads TaskID, CurrentStepID
// and WorkflowID off the loaded MachineState, all task-derived, so which
// session the resolver happens to pick cannot change the answer. SessionID,
// SessionState, Data and IsPassthrough are the session-derived fields, and
// none of them is read by guard evaluation.
//
// If a guard ever starts reading SessionID, SessionState, Data or
// IsPassthrough, that
// stops being true silently: the resolver would hand the engine whichever
// agent's state happened to be newest, and a decision could be evaluated
// against another agent's state with nothing failing. This test makes that
// regression loud. The fix at that point is to session-scope the CALL SITE
// — pass the deciding session, as RecordDecision does via
// resolveDeciderSessionID — not to add a defensive check inside the
// task-scoped resolver, which would only make a wrong-scoped read look
// safe.
func TestEvaluateStepQuorum_InsensitiveToWhichLiveSessionIsNewest(t *testing.T) {
	newStore := func() *sessionScopedStateStore {
		return &sessionScopedStateStore{
			step: StepSpec{
				ID: "review", WorkflowID: "wf", Position: 1,
				Events: map[Trigger][]Action{
					TriggerOnTurnComplete: {
						{Kind: ActionMoveToNext, Guard: approvedGuard("reviewer")},
					},
				},
			},
			// The two live sessions carry DIFFERENT per-session bags and
			// states, so a guard that started reading either would diverge
			// between them.
			dataBySess: map[string]map[string]any{
				"sess-runner":   {"owner": "runner", "attempts": 3},
				"sess-reviewer": {"owner": "reviewer", "attempts": 1},
			},
			stateBySess: map[string]string{
				"sess-runner":   "RUNNING",
				"sess-reviewer": "WAITING_FOR_INPUT",
			},
			passthroughBySess: map[string]bool{
				"sess-runner":   true,
				"sess-reviewer": false,
			},
			applied: map[string]bool{},
		}
	}
	parts := fakeParticipants{list: []ParticipantInfo{
		{ID: "p1", Role: "reviewer", DecisionRequired: true, AgentProfileID: "rev-A"},
	}}
	newDecisions := func() *fakeDecisionStore {
		d := newFakeDecisionStore()
		d.byKey[dkey("task-1", "review")] = []DecisionInfo{{ParticipantID: "p1", Decision: DecisionApproved}}
		return d
	}

	runnerStore := newStore()
	fromRunner, err := New(runnerStore, MapRegistry{},
		WithDecisionStore(newDecisions()), WithParticipantStore(parts)).
		EvaluateStepQuorum(context.Background(), "task-1", "sess-runner")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum(sess-runner): %v", err)
	}

	reviewerStore := newStore()
	fromReviewer, err := New(reviewerStore, MapRegistry{},
		WithDecisionStore(newDecisions()), WithParticipantStore(parts)).
		EvaluateStepQuorum(context.Background(), "task-1", "sess-reviewer")
	if err != nil {
		t.Fatalf("EvaluateStepQuorum(sess-reviewer): %v", err)
	}

	// Guard against a vacuous pass: if the fake ever stops handing the two
	// runs genuinely different per-session bags and states, the comparison
	// below proves nothing. Assert the divergence actually reached the engine.
	if len(runnerStore.loadedData) != 1 || len(reviewerStore.loadedData) != 1 {
		t.Fatalf("expected exactly one LoadState per run, got %d and %d",
			len(runnerStore.loadedData), len(reviewerStore.loadedData))
	}
	if reflect.DeepEqual(runnerStore.loadedData[0], reviewerStore.loadedData[0]) {
		t.Fatalf("test is vacuous: both sessions loaded the same Data %v", runnerStore.loadedData[0])
	}
	if len(runnerStore.loadedStates) != 1 || len(reviewerStore.loadedStates) != 1 {
		t.Fatalf("expected exactly one LoadState per run, got %d and %d states",
			len(runnerStore.loadedStates), len(reviewerStore.loadedStates))
	}
	if runnerStore.loadedStates[0] == reviewerStore.loadedStates[0] {
		t.Fatalf("test is vacuous: both sessions loaded the same SessionState %q", runnerStore.loadedStates[0])
	}
	if len(runnerStore.loadedPassthrough) != 1 || len(reviewerStore.loadedPassthrough) != 1 {
		t.Fatalf("expected exactly one LoadState per run, got %d and %d passthrough values",
			len(runnerStore.loadedPassthrough), len(reviewerStore.loadedPassthrough))
	}
	if runnerStore.loadedPassthrough[0] == reviewerStore.loadedPassthrough[0] {
		t.Fatalf("test is vacuous: both sessions loaded the same IsPassthrough value %v",
			runnerStore.loadedPassthrough[0])
	}

	// The snapshot must be identical despite that divergence.
	if fromRunner.StepID != fromReviewer.StepID {
		t.Errorf("StepID differs by session: %q vs %q", fromRunner.StepID, fromReviewer.StepID)
	}
	if fromRunner.ReevaluationBlocked != fromReviewer.ReevaluationBlocked {
		t.Errorf("ReevaluationBlocked differs by session: %v vs %v",
			fromRunner.ReevaluationBlocked, fromReviewer.ReevaluationBlocked)
	}
	if len(fromRunner.Guards) != len(fromReviewer.Guards) {
		t.Fatalf("guard count differs by session: %d vs %d",
			len(fromRunner.Guards), len(fromReviewer.Guards))
	}
	for i := range fromRunner.Guards {
		if !reflect.DeepEqual(fromRunner.Guards[i], fromReviewer.Guards[i]) {
			t.Errorf("guard %d differs by session:\n  runner:   %#v\n  reviewer: %#v",
				i, fromRunner.Guards[i], fromReviewer.Guards[i])
		}
	}

	// Sanity: the snapshot is non-trivial, so the equality above has content.
	if fromRunner.StepID != "review" || len(fromRunner.Guards) != 1 {
		t.Fatalf("expected a one-guard snapshot at step review, got %#v", fromRunner)
	}
	if !fromRunner.Guards[0].Satisfied {
		t.Fatalf("expected the quorum guard to be satisfied, got %#v", fromRunner.Guards[0])
	}
}
