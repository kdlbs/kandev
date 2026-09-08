package engine

import (
	"context"
	"errors"
	"testing"
)

// fakeParticipantSeatWriter records HasRoleSeatForTaskWorkflow/EnsureRoleSeat
// calls for EnsureParticipantSeatCallback assertions.
type fakeParticipantSeatWriter struct {
	hasSeat      bool
	hasSeatErr   error
	hasSeatCalls int

	ensureCalls  []ensureRoleSeatCall
	ensureErr    error
	ensureResult ParticipantInfo
}

type ensureRoleSeatCall struct {
	workflowID     string
	stepID         string
	taskID         string
	role           string
	agentProfileID string
}

func (f *fakeParticipantSeatWriter) HasRoleSeatForTaskWorkflow(
	_ context.Context, _, _, _ string,
) (bool, error) {
	f.hasSeatCalls++
	return f.hasSeat, f.hasSeatErr
}

func (f *fakeParticipantSeatWriter) EnsureRoleSeat(
	_ context.Context, workflowID, stepID, taskID, role, agentProfileID string,
) (ParticipantInfo, error) {
	f.ensureCalls = append(f.ensureCalls, ensureRoleSeatCall{
		workflowID: workflowID, stepID: stepID, taskID: taskID,
		role: role, agentProfileID: agentProfileID,
	})
	if f.ensureErr != nil {
		return ParticipantInfo{}, f.ensureErr
	}
	return f.ensureResult, nil
}

// fakeParticipantSeatCaster records CastParticipantSeat calls for
// EnsureParticipantSeatCallback assertions.
type fakeParticipantSeatCaster struct {
	result         ParticipantSeatCastResult
	err            error
	calls          int
	lastWorkflowID string
	lastTaskID     string
	lastStepID     string
	lastRole       string
}

func (f *fakeParticipantSeatCaster) CastParticipantSeat(
	_ context.Context, workflowID, taskID, stepID, role string,
) (ParticipantSeatCastResult, error) {
	f.calls++
	f.lastWorkflowID = workflowID
	f.lastTaskID = taskID
	f.lastStepID = stepID
	f.lastRole = role
	return f.result, f.err
}

func newEnsureSeatInput(role string) ActionInput {
	return ActionInput{
		Trigger: TriggerOnEnter,
		State:   MachineState{TaskID: "task-1", WorkflowID: "wf-1"},
		Step:    StepSpec{ID: "step-1"},
		Action: Action{
			Kind:                  ActionEnsureParticipantSeat,
			EnsureParticipantSeat: &EnsureParticipantSeatAction{Role: role},
		},
		EntryID: "entry-1",
	}
}

func TestEnsureParticipantSeatCallback_NoOpWhenSeatAlreadyExists(t *testing.T) {
	writer := &fakeParticipantSeatWriter{hasSeat: true}
	caster := &fakeParticipantSeatCaster{}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster}

	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if writer.hasSeatCalls != 1 {
		t.Fatalf("expected exactly one existence check, got %d", writer.hasSeatCalls)
	}
	if caster.calls != 0 {
		t.Fatalf("expected caster not to be invoked when a seat already exists, got %d calls", caster.calls)
	}
	if len(writer.ensureCalls) != 0 {
		t.Fatalf("expected no write when a seat already exists, got %+v", writer.ensureCalls)
	}
}

func TestEnsureParticipantSeatCallback_MalformedRoleReportsWithoutWriting(t *testing.T) {
	for _, role := range []string{"", "not-a-role", "ceo"} {
		writer := &fakeParticipantSeatWriter{}
		caster := &fakeParticipantSeatCaster{}
		cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster}

		_, err := cb.Execute(context.Background(), newEnsureSeatInput(role))
		if !errors.Is(err, ErrMalformedParticipantRole) {
			t.Fatalf("role %q: expected ErrMalformedParticipantRole, got %v", role, err)
		}
		if writer.hasSeatCalls != 0 {
			t.Fatalf("role %q: expected no existence check for a malformed role, got %d", role, writer.hasSeatCalls)
		}
		if caster.calls != 0 {
			t.Fatalf("role %q: expected caster not to be invoked for a malformed role", role)
		}
		if len(writer.ensureCalls) != 0 {
			t.Fatalf("role %q: expected no seat written for a malformed role", role)
		}
	}
}

func TestEnsureParticipantSeatCallback_UnfillableCastReportsWithoutWriting(t *testing.T) {
	writer := &fakeParticipantSeatWriter{hasSeat: false}
	caster := &fakeParticipantSeatCaster{result: ParticipantSeatCastResult{Unfillable: true}}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster}

	_, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer"))
	if !errors.Is(err, ErrParticipantSeatUnfillable) {
		t.Fatalf("expected ErrParticipantSeatUnfillable, got %v", err)
	}
	if caster.calls != 1 {
		t.Fatalf("expected the caster to be invoked exactly once, got %d", caster.calls)
	}
	if len(writer.ensureCalls) != 0 {
		t.Fatalf("expected no seat written when the role is unfillable, got %+v", writer.ensureCalls)
	}
}

func TestEnsureParticipantSeatCallback_SuccessfulCastWritesSeat(t *testing.T) {
	writer := &fakeParticipantSeatWriter{hasSeat: false, ensureResult: ParticipantInfo{ID: "p-1", AgentProfileID: "agent-1"}}
	caster := &fakeParticipantSeatCaster{result: ParticipantSeatCastResult{
		AgentProfileID: "agent-1",
		Provenance:     SeatProvenanceEligiblePool,
	}}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster}

	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caster.calls != 1 || caster.lastWorkflowID != "wf-1" || caster.lastTaskID != "task-1" ||
		caster.lastStepID != "step-1" || caster.lastRole != "reviewer" {
		t.Fatalf("expected caster invoked once for (wf-1, task-1, step-1, reviewer), got calls=%d workflowID=%q taskID=%q stepID=%q role=%q",
			caster.calls, caster.lastWorkflowID, caster.lastTaskID, caster.lastStepID, caster.lastRole)
	}
	if len(writer.ensureCalls) != 1 {
		t.Fatalf("expected exactly one seat write, got %+v", writer.ensureCalls)
	}
	got := writer.ensureCalls[0]
	want := ensureRoleSeatCall{workflowID: "wf-1", stepID: "step-1", taskID: "task-1", role: "reviewer", agentProfileID: "agent-1"}
	if got != want {
		t.Fatalf("EnsureRoleSeat called with %+v, want %+v", got, want)
	}
}

func TestEnsureParticipantSeatCallback_NilWriterReportsNotYetWired(t *testing.T) {
	cb := EnsureParticipantSeatCallback{Writer: nil, Caster: &fakeParticipantSeatCaster{}}

	_, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer"))
	if !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestEnsureParticipantSeatCallback_NilCasterReportsNotYetWiredOnlyWhenSeatMissing(t *testing.T) {
	// A nil Caster must not surface an error when a seat already exists —
	// the callback never needs to cast in that path.
	writer := &fakeParticipantSeatWriter{hasSeat: true}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: nil}
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer")); err != nil {
		t.Fatalf("expected no error when a seat already exists despite nil Caster, got %v", err)
	}

	// When no seat exists, a nil Caster must surface ErrActionNotYetWired.
	writer = &fakeParticipantSeatWriter{hasSeat: false}
	cb = EnsureParticipantSeatCallback{Writer: writer, Caster: nil}
	_, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer"))
	if !errors.Is(err, ErrActionNotYetWired) {
		t.Fatalf("expected ErrActionNotYetWired, got %v", err)
	}
}

func TestEnsureParticipantSeatCallback_SatisfiesActionCallback(t *testing.T) {
	var _ ActionCallback = EnsureParticipantSeatCallback{}
}
