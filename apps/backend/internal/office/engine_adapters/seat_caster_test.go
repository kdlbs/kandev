package engine_adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// fakeSeatCasterWorkflowRepo is a minimal SeatCasterWorkflowRepo fake for
// SeatCasterAdapter tests.
type fakeSeatCasterWorkflowRepo struct {
	runner    string
	runnerErr error

	gotRunnerStepID string
	gotRunnerTaskID string
}

func (f *fakeSeatCasterWorkflowRepo) ResolveCurrentRunner(_ context.Context, stepID, taskID string) (string, error) {
	f.gotRunnerStepID = stepID
	f.gotRunnerTaskID = taskID
	return f.runner, f.runnerErr
}

func TestSeatCasterAdapter_EmptyCandidateListFallsBackToRunner(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Unfillable {
		t.Fatal("expected a fallback seat, not unfillable")
	}
	if got.AgentProfileID != "runner-agent" {
		t.Errorf("agent = %q, want runner-agent", got.AgentProfileID)
	}
	if got.Provenance != engine.SeatProvenanceRunnerFallback {
		t.Errorf("provenance = %q, want %q", got.Provenance, engine.SeatProvenanceRunnerFallback)
	}
	if !got.SelfReview {
		t.Error("expected self-review to be recorded for a runner-fallback seat")
	}
	if got.WorkspaceID != "ws-1" {
		t.Errorf("workspace_id = %q, want ws-1", got.WorkspaceID)
	}
}

func TestSeatCasterAdapter_EmptyCandidateListAndNoRunnerIsUnfillable(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Unfillable {
		t.Fatal("expected unfillable when the candidate list is empty and the runner does not resolve")
	}
	if got.AgentProfileID != "" {
		t.Errorf("expected no agent profile id, got %q", got.AgentProfileID)
	}
	if got.WorkspaceID != "ws-1" {
		t.Errorf("workspace_id = %q, want ws-1 (needed for the AC-004.1 warning record even when unfillable)", got.WorkspaceID)
	}
}

func TestSeatCasterAdapter_SingleCandidateIsRunnerRecordsSelfReview(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "agent-A", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: now},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "agent-A"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-A" {
		t.Errorf("agent = %q, want agent-A", got.AgentProfileID)
	}
	if got.Provenance != engine.SeatProvenanceEligiblePool {
		t.Errorf("provenance = %q, want %q", got.Provenance, engine.SeatProvenanceEligiblePool)
	}
	if !got.SelfReview {
		t.Error("expected self-review when the sole eligible candidate is the runner")
	}
}

func TestSeatCasterAdapter_FirstCandidateIsRunnerSeatsSecond(t *testing.T) {
	t1 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "agent-A", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t1},
			{ID: "agent-B", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t2},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "agent-A"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-B" {
		t.Errorf("agent = %q, want agent-B (the second member, to avoid self-review)", got.AgentProfileID)
	}
	if got.Provenance != engine.SeatProvenanceEligiblePool {
		t.Errorf("provenance = %q, want %q", got.Provenance, engine.SeatProvenanceEligiblePool)
	}
	if got.SelfReview {
		t.Error("expected no self-review when an alternative candidate exists")
	}
}

func TestSeatCasterAdapter_FirstCandidateNotRunnerSeatsFirstAsEligiblePool(t *testing.T) {
	t1 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "agent-A", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t1},
			{ID: "agent-B", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t2},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "someone-else"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-A" {
		t.Errorf("agent = %q, want agent-A", got.AgentProfileID)
	}
	if got.Provenance != engine.SeatProvenanceEligiblePool {
		t.Errorf("provenance = %q, want %q", got.Provenance, engine.SeatProvenanceEligiblePool)
	}
	if got.SelfReview {
		t.Error("expected no self-review when the first candidate is not the runner")
	}
}

func TestSeatCasterAdapter_ExcludesStoppedAndPendingApprovalCandidates(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "agent-stopped", Role: models.AgentRoleCEO, Status: models.AgentStatusStopped, CreatedAt: t0},
			{ID: "agent-pending", Role: models.AgentRoleCEO, Status: models.AgentStatusPendingApproval, CreatedAt: t0.Add(time.Minute)},
			{ID: "agent-ok", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t0.Add(2 * time.Minute)},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-ok" {
		t.Errorf("agent = %q, want agent-ok (the only non-excluded candidate)", got.AgentProfileID)
	}
	// AC-OFFICE-REVIEW-SEATS-002.2: the exclusion must be applied over the
	// listing's result, not by pushing a status filter down to the shared
	// listing method.
	if office.gotFilter.Status != "" {
		t.Errorf("expected no status filter passed to the shared listing method, got %q", office.gotFilter.Status)
	}
}

func TestSeatCasterAdapter_OrdersCandidatesByCreatedAtThenIdentifier(t *testing.T) {
	tEarly := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	tLate := tEarly.Add(time.Minute)
	// agent-Z and agent-B share the same created_at: the identifier
	// tiebreak must place agent-B first. agent-A has a later created_at
	// and must sort last despite its earlier-alphabetical identifier.
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "agent-Z", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tEarly},
			{ID: "agent-A", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tLate},
			{ID: "agent-B", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tEarly},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-B" {
		t.Errorf("agent = %q, want agent-B (earliest created_at, id tiebreak)", got.AgentProfileID)
	}
}

func TestSeatCasterAdapter_RejectsEmptyTaskID(t *testing.T) {
	a := NewSeatCasterAdapter(&fakeOfficeRepo{}, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error for empty task id")
	}
}

func TestSeatCasterAdapter_PropagatesWorkspaceResolutionError(t *testing.T) {
	office := &fakeOfficeRepo{fieldsErr: errors.New("boom")}
	a := NewSeatCasterAdapter(office, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSeatCasterAdapter_PropagatesTaskWithNoWorkspaceAsError(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1"}}
	a := NewSeatCasterAdapter(office, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error for a task with no workspace")
	}
}

func TestSeatCasterAdapter_RejectsEmptyStepID(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{}
	a := NewSeatCasterAdapter(office, wf)
	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "", "reviewer"); err == nil {
		t.Fatal("expected error for empty step id")
	}
}

func TestSeatCasterAdapter_PropagatesRunnerResolutionError(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runnerErr: errors.New("boom")}
	a := NewSeatCasterAdapter(office, wf)
	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSeatCasterAdapter_PropagatesCandidateListingError(t *testing.T) {
	office := &fakeOfficeRepo{
		fields:  &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		listErr: errors.New("boom"),
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)
	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSeatCasterAdapter_UsesEnteredStepToResolveRunner(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	if _, err := a.CastParticipantSeat(context.Background(), "t-1", "step-42", "reviewer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.gotRunnerStepID != "step-42" || wf.gotRunnerTaskID != "t-1" {
		t.Errorf("ResolveCurrentRunner called with step=%q task=%q, want step-42/t-1",
			wf.gotRunnerStepID, wf.gotRunnerTaskID)
	}
}

func TestSeatCasterAdapter_SatisfiesParticipantSeatCaster(t *testing.T) {
	var _ engine.ParticipantSeatCaster = (*SeatCasterAdapter)(nil)
}
