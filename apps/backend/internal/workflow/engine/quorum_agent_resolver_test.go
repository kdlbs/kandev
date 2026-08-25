package engine

import (
	"context"
	"errors"
	"expvar"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// fakeAgentProfileResolver lets tests control which agent profile ids
// resolve, or fail resolution outright with an error.
type fakeAgentProfileResolver struct {
	unresolved map[string]bool
	err        error

	calls []string
}

func (f *fakeAgentProfileResolver) AgentProfileExists(_ context.Context, agentProfileID string) (bool, error) {
	f.calls = append(f.calls, agentProfileID)
	if f.err != nil {
		return false, f.err
	}
	return !f.unresolved[agentProfileID], nil
}

func quorumEngineWithResolver(
	decisions DecisionStore, participants ParticipantStore, resolver AgentProfileResolver,
) *Engine {
	return New(quorumStore(nil), MapRegistry{},
		WithDecisionStore(decisions), WithParticipantStore(participants), WithAgentProfileResolver(resolver))
}

func readParticipantAgentUnresolvedCounter(role string) int64 {
	m := expvar.Get("workflow_participant_agent_unresolved_total")
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

func TestRequiredSeats_DropsSeatWithUnresolvedAgentProfile(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		{ID: "p2", StepID: "review", Role: "reviewer", AgentProfileID: "rev-B", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{unresolved: map[string]bool{"rev-B": true}}
	eng := quorumEngineWithResolver(newFakeDecisionStore(), parts, resolver)

	seats, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeatsForWorkflow: %v", err)
	}
	if len(seats) != 1 || seats[0].ID != "p1" {
		t.Fatalf("expected only p1 (resolvable agent) to survive, got %+v", seats)
	}
}

func TestRequiredSeats_UnresolvedAgentIncrementsCounterAndLogs(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-gone", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{unresolved: map[string]bool{"rev-gone": true}}
	core, logs := observer.New(zap.WarnLevel)
	zapLogger := zap.New(core)
	log, err := logger.NewFromZap(zapLogger)
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}
	eng := New(quorumStore(nil), MapRegistry{},
		WithDecisionStore(newFakeDecisionStore()), WithParticipantStore(parts),
		WithAgentProfileResolver(resolver), WithLogger(log))

	before := readParticipantAgentUnresolvedCounter("reviewer")
	seats, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeatsForWorkflow: %v", err)
	}
	if len(seats) != 0 {
		t.Fatalf("expected the unresolved seat to be dropped, got %+v", seats)
	}
	after := readParticipantAgentUnresolvedCounter("reviewer")
	if after-before != 1 {
		t.Fatalf("counter delta = %d, want 1", after-before)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one warning log, got %d: %+v", len(entries), entries)
	}
	entry := entries[0]
	if entry.Message != "workflow quorum participant agent profile unresolved" {
		t.Errorf("log message = %q, unexpected", entry.Message)
	}
	fields := entry.ContextMap()
	if fields["task_id"] != "task-1" || fields["step_id"] != "review" ||
		fields["role"] != "reviewer" || fields["agent_profile_id"] != "rev-gone" {
		t.Errorf("unexpected log fields: %+v", fields)
	}
}

// TestRequiredSeats_MalformedRoleSanitizesCounterLabel covers
// AC-OFFICE-REVIEW-SEATS-004.6/.11: a guard.Role outside the fixed
// ParticipantRole set must not become an expvar counter label verbatim — it
// folds to the SeatRoleLabelInvalid sentinel, while the raw operator string
// still reaches the paired warning record as a typed field.
func TestRequiredSeats_MalformedRoleSanitizesCounterLabel(t *testing.T) {
	const malformedRole = "not-a-real-role"
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: malformedRole, AgentProfileID: "rev-gone", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{unresolved: map[string]bool{"rev-gone": true}}
	core, logs := observer.New(zap.WarnLevel)
	zapLogger := zap.New(core)
	log, err := logger.NewFromZap(zapLogger)
	if err != nil {
		t.Fatalf("build logger: %v", err)
	}
	eng := New(quorumStore(nil), MapRegistry{},
		WithDecisionStore(newFakeDecisionStore()), WithParticipantStore(parts),
		WithAgentProfileResolver(resolver), WithLogger(log))

	before := readParticipantAgentUnresolvedCounter(SeatRoleLabelInvalid)
	seats, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", malformedRole)
	if err != nil {
		t.Fatalf("requiredSeatsForWorkflow: %v", err)
	}
	if len(seats) != 0 {
		t.Fatalf("expected the unresolved seat to be dropped, got %+v", seats)
	}
	after := readParticipantAgentUnresolvedCounter(SeatRoleLabelInvalid)
	if after-before != 1 {
		t.Fatalf("%s counter delta = %d, want 1", SeatRoleLabelInvalid, after-before)
	}
	if got := readParticipantAgentUnresolvedCounter(malformedRole); got != 0 {
		t.Fatalf("expected no counter entry under the raw operator string %q, got %d", malformedRole, got)
	}

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected exactly one warning log, got %d: %+v", len(entries), entries)
	}
	if fields := entries[0].ContextMap(); fields["role"] != malformedRole {
		t.Errorf("expected the warning record to carry the raw role %q, got %+v", malformedRole, fields)
	}
}

func TestRequiredSeats_KeepsSeatWhenResolverNotWired(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	eng := quorumEngine(newFakeDecisionStore(), parts)

	seats, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeatsForWorkflow: %v", err)
	}
	if len(seats) != 1 || seats[0].ID != "p1" {
		t.Fatalf("expected the seat to survive when no resolver is wired, got %+v", seats)
	}
}

func TestRequiredSeats_EmptyAgentProfileIDSkipsResolverCall(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{}
	eng := quorumEngineWithResolver(newFakeDecisionStore(), parts, resolver)

	seats, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", "reviewer")
	if err != nil {
		t.Fatalf("requiredSeatsForWorkflow: %v", err)
	}
	if len(seats) != 1 {
		t.Fatalf("expected the empty-agent seat to be kept, got %+v", seats)
	}
	if len(resolver.calls) != 0 {
		t.Errorf("expected the resolver not to be called for an empty agent profile id, got calls %+v", resolver.calls)
	}
}

func TestRequiredSeats_PropagatesResolverError(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	boom := errors.New("boom")
	resolver := &fakeAgentProfileResolver{err: boom}
	eng := quorumEngineWithResolver(newFakeDecisionStore(), parts, resolver)

	_, err := eng.requiredSeatsForWorkflow(context.Background(), "review", "task-1", "", "reviewer")
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want wrapping %v", err, boom)
	}
}

func TestComputeGuardOutcome_ResolverErrorSurfacesAsEvaluationError(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{err: errors.New("boom")}
	eng := quorumEngineWithResolver(newFakeDecisionStore(), parts, resolver)

	outcome := eng.computeGuardOutcome(context.Background(), MachineState{TaskID: "task-1", CurrentStepID: "review"},
		&WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if outcome.Reason != ReasonEvaluationError {
		t.Fatalf("reason = %q, want %q", outcome.Reason, ReasonEvaluationError)
	}
	if outcome.Err == nil {
		t.Errorf("expected outcome.Err to carry the resolver failure")
	}
}

// TestComputeGuardOutcome_ContinuesEvaluatingAfterDroppingUnresolvedSeat is
// the end-to-end AC-OFFICE-REVIEW-SEATS-004.3 assertion: a guard whose slate
// held one resolvable and one unresolved-agent seat still fires once the
// resolvable seat approves, instead of waiting forever on a seat that can
// never decide.
func TestComputeGuardOutcome_ContinuesEvaluatingAfterDroppingUnresolvedSeat(t *testing.T) {
	parts := scopedParticipants{template: []ParticipantInfo{
		{ID: "p1", StepID: "review", Role: "reviewer", AgentProfileID: "rev-A", DecisionRequired: true},
		{ID: "p2", StepID: "review", Role: "reviewer", AgentProfileID: "rev-gone", DecisionRequired: true},
	}}
	resolver := &fakeAgentProfileResolver{unresolved: map[string]bool{"rev-gone": true}}
	decisions := newFakeDecisionStore()
	if err := decisions.RecordStepDecision(context.Background(), DecisionInfo{
		TaskID: "task-1", StepID: "review", ParticipantID: "p1", Decision: DecisionApproved,
	}); err != nil {
		t.Fatalf("seed decision: %v", err)
	}
	eng := quorumEngineWithResolver(decisions, parts, resolver)

	outcome := eng.computeGuardOutcome(context.Background(), MachineState{TaskID: "task-1", CurrentStepID: "review"},
		&WaitForQuorumGuard{Role: "reviewer", Threshold: QuorumAllApprove})
	if !outcome.Satisfied {
		t.Fatalf("expected the guard to fire once the only counted seat approved, got %+v", outcome)
	}
	if outcome.RequiredCount != 1 {
		t.Errorf("required count = %d, want 1 (the unresolved seat must not count)", outcome.RequiredCount)
	}
}
