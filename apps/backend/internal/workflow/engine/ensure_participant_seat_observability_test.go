package engine

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func newObservedSeatLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zap.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	return log, logs
}

func readSeatEnsureCounter(t *testing.T, role, outcome string) int64 {
	t.Helper()
	m := expvar.Get("office_review_seat_ensure_total")
	if m == nil {
		t.Fatal("expvar \"office_review_seat_ensure_total\" not published")
	}
	kv := m.(*expvar.Map).Get(seatEnsureCounterKey(role, outcome))
	if kv == nil {
		return 0
	}
	iv, ok := kv.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// seatEnsureCounterKey mirrors quorummetrics.seatMetricLabel's "k=v;k=v"
// format for the office_review_seat_ensure_total dimensions this test file
// reads, without exporting the unexported helper across packages.
func seatEnsureCounterKey(role, outcome string) string {
	return "role=" + role + ";outcome=" + outcome
}

func readSeatProvenanceCounter(t *testing.T, role, provenance, selfReview string) int64 {
	t.Helper()
	m := expvar.Get("office_review_seat_provenance_total")
	if m == nil {
		t.Fatal("expvar \"office_review_seat_provenance_total\" not published")
	}
	kv := m.(*expvar.Map).Get("role=" + role + ";provenance=" + provenance + ";self_review=" + selfReview)
	if kv == nil {
		return 0
	}
	iv, ok := kv.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

func TestEnsureParticipantSeatCallback_AlreadySeatedRecordsCounterOnly(t *testing.T) {
	log, logs := newObservedSeatLogger(t)
	writer := &fakeParticipantSeatWriter{hasSeat: true}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: &fakeParticipantSeatCaster{}, Logger: log}

	before := readSeatEnsureCounter(t, "reviewer", SeatOutcomeAlreadySeated)
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("reviewer")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	after := readSeatEnsureCounter(t, "reviewer", SeatOutcomeAlreadySeated)
	if after-before != 1 {
		t.Fatalf("already_seated counter delta = %d, want 1", after-before)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no warning record for an already-seated role, got %d", logs.Len())
	}
}

func TestEnsureParticipantSeatCallback_MalformedRoleRecordsFixedLabelAndWarning(t *testing.T) {
	log, logs := newObservedSeatLogger(t)
	writer := &fakeParticipantSeatWriter{}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: &fakeParticipantSeatCaster{}, Logger: log}

	before := readSeatEnsureCounter(t, SeatRoleLabelInvalid, SeatOutcomeMalformedConfig)
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("not-a-real-role")); !errors.Is(err, ErrMalformedParticipantRole) {
		t.Fatalf("expected ErrMalformedParticipantRole, got %v", err)
	}
	after := readSeatEnsureCounter(t, SeatRoleLabelInvalid, SeatOutcomeMalformedConfig)
	if after-before != 1 {
		t.Fatalf("malformed_config counter delta = %d, want 1", after-before)
	}

	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record, got %d", logs.Len())
	}
	entry := logs.All()[0]
	fields := entry.ContextMap()
	if fields["declared_role"] != "not-a-real-role" {
		t.Fatalf("declared_role field = %v, want the raw operator-supplied role", fields["declared_role"])
	}
	if fields["task_id"] != "task-1" || fields["step_id"] != "step-1" {
		t.Fatalf("unexpected identifying fields: %+v", fields)
	}
}

func TestEnsureParticipantSeatCallback_UnfillableRecordsNoRunnerOutcomeAndWorkspace(t *testing.T) {
	log, logs := newObservedSeatLogger(t)
	writer := &fakeParticipantSeatWriter{hasSeat: false}
	caster := &fakeParticipantSeatCaster{result: ParticipantSeatCastResult{Unfillable: true, WorkspaceID: "ws-unfillable"}}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster, Logger: log}

	before := readSeatEnsureCounter(t, "approver", SeatOutcomeNoRunner)
	_, err := cb.Execute(context.Background(), newEnsureSeatInput("approver"))
	if !errors.Is(err, ErrParticipantSeatUnfillable) {
		t.Fatalf("expected ErrParticipantSeatUnfillable, got %v", err)
	}
	after := readSeatEnsureCounter(t, "approver", SeatOutcomeNoRunner)
	if after-before != 1 {
		t.Fatalf("no_runner counter delta = %d, want 1", after-before)
	}

	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fields["workspace_id"] != "ws-unfillable" {
		t.Fatalf("workspace_id field = %v, want ws-unfillable", fields["workspace_id"])
	}
	if fields["role"] != "approver" {
		t.Fatalf("role field = %v, want approver", fields["role"])
	}
}

func TestEnsureParticipantSeatCallback_WriterErrorRecordsDistinctOutcomeAndError(t *testing.T) {
	log, logs := newObservedSeatLogger(t)
	wantErr := errors.New("db unavailable")
	writer := &fakeParticipantSeatWriter{hasSeatErr: wantErr}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: &fakeParticipantSeatCaster{}, Logger: log}

	before := readSeatEnsureCounter(t, "watcher", SeatOutcomeError)
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("watcher")); !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
	after := readSeatEnsureCounter(t, "watcher", SeatOutcomeError)
	if after-before != 1 {
		t.Fatalf("error counter delta = %d, want 1", after-before)
	}

	if logs.Len() != 1 {
		t.Fatalf("expected exactly one warning record, got %d", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if fmt.Sprint(fields["error"]) != wantErr.Error() {
		t.Fatalf("error field = %v, want it to carry %v", fields["error"], wantErr)
	}
}

func TestEnsureParticipantSeatCallback_SuccessfulCastRecordsSeatedAndProvenance(t *testing.T) {
	log, logs := newObservedSeatLogger(t)
	writer := &fakeParticipantSeatWriter{hasSeat: false, ensureResult: ParticipantInfo{ID: "p-1", AgentProfileID: "agent-1"}}
	caster := &fakeParticipantSeatCaster{result: ParticipantSeatCastResult{
		AgentProfileID: "agent-1",
		Provenance:     SeatProvenanceRunnerFallback,
		SelfReview:     true,
	}}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: caster, Logger: log}

	beforeSeated := readSeatEnsureCounter(t, "collaborator", SeatOutcomeSeated)
	beforeProvenance := readSeatProvenanceCounter(t, "collaborator", string(SeatProvenanceRunnerFallback), "true")
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("collaborator")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	afterSeated := readSeatEnsureCounter(t, "collaborator", SeatOutcomeSeated)
	afterProvenance := readSeatProvenanceCounter(t, "collaborator", string(SeatProvenanceRunnerFallback), "true")

	if afterSeated-beforeSeated != 1 {
		t.Fatalf("seated counter delta = %d, want 1", afterSeated-beforeSeated)
	}
	if afterProvenance-beforeProvenance != 1 {
		t.Fatalf("provenance counter delta = %d, want 1", afterProvenance-beforeProvenance)
	}
	// A successful seating is not itself a failure or an empty result — no
	// warning record accompanies it (AC-OFFICE-REVIEW-SEATS-004.8 scopes the
	// warning requirement to the unfillable/malformed/error conditions).
	if logs.Len() != 0 {
		t.Fatalf("expected no warning record for a successful cast, got %d", logs.Len())
	}
}

func TestEnsureParticipantSeatCallback_NilLoggerSkipsWarningButStillCounts(t *testing.T) {
	writer := &fakeParticipantSeatWriter{}
	cb := EnsureParticipantSeatCallback{Writer: writer, Caster: &fakeParticipantSeatCaster{}}

	before := readSeatEnsureCounter(t, SeatRoleLabelInvalid, SeatOutcomeMalformedConfig)
	if _, err := cb.Execute(context.Background(), newEnsureSeatInput("")); !errors.Is(err, ErrMalformedParticipantRole) {
		t.Fatalf("expected ErrMalformedParticipantRole, got %v", err)
	}
	after := readSeatEnsureCounter(t, SeatRoleLabelInvalid, SeatOutcomeMalformedConfig)
	if after-before != 1 {
		t.Fatalf("malformed_config counter delta = %d, want 1 even with a nil Logger", after-before)
	}
}
