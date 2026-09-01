package engine_adapters

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// fakeSeatCasterWorkflowRepo is a minimal SeatCasterWorkflowRepo fake for
// SeatCasterAdapter tests.
type fakeSeatCasterWorkflowRepo struct {
	runner    string
	runnerErr error

	seatedAgentIDs       []string
	participantsErr      error
	gotParticipantsFor   string
	gotParticipantsForWF string

	gotRunnerStepID string
	gotRunnerTaskID string
}

func (f *fakeSeatCasterWorkflowRepo) ResolveCurrentRunner(_ context.Context, stepID, taskID string) (string, error) {
	f.gotRunnerStepID = stepID
	f.gotRunnerTaskID = taskID
	return f.runner, f.runnerErr
}

func (f *fakeSeatCasterWorkflowRepo) ListParticipantsForTaskWorkflow(
	_ context.Context, taskID, workflowID string,
) ([]*wfmodels.WorkflowStepParticipant, error) {
	f.gotParticipantsFor = taskID
	f.gotParticipantsForWF = workflowID
	if f.participantsErr != nil {
		return nil, f.participantsErr
	}
	participants := make([]*wfmodels.WorkflowStepParticipant, 0, len(f.seatedAgentIDs))
	for _, id := range f.seatedAgentIDs {
		participants = append(participants, &wfmodels.WorkflowStepParticipant{AgentProfileID: id})
	}
	return participants, nil
}

func TestSeatCasterAdapter_EmptyCandidateListFallsBackToRunner(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
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

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "agent-B" {
		t.Errorf("agent = %q, want agent-B (earliest created_at, id tiebreak)", got.AgentProfileID)
	}
}

func TestSeatCasterAdapter_RejectsEmptyTaskID(t *testing.T) {
	a := NewSeatCasterAdapter(&fakeOfficeRepo{}, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error for empty task id")
	}
}

func TestSeatCasterAdapter_PropagatesWorkspaceResolutionError(t *testing.T) {
	office := &fakeOfficeRepo{fieldsErr: errors.New("boom")}
	a := NewSeatCasterAdapter(office, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSeatCasterAdapter_PropagatesTaskWithNoWorkspaceAsError(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1"}}
	a := NewSeatCasterAdapter(office, &fakeSeatCasterWorkflowRepo{})
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error for a task with no workspace")
	}
}

func TestSeatCasterAdapter_RejectsEmptyStepID(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{}
	a := NewSeatCasterAdapter(office, wf)
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "", "reviewer"); err == nil {
		t.Fatal("expected error for empty step id")
	}
}

func TestSeatCasterAdapter_PropagatesRunnerResolutionError(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runnerErr: errors.New("boom")}
	a := NewSeatCasterAdapter(office, wf)
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer"); err == nil {
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
	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", "reviewer"); err == nil {
		t.Fatal("expected error")
	}
}

func TestSeatCasterAdapter_UsesEnteredStepToResolveRunner(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	if _, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-42", "reviewer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.gotRunnerStepID != "step-42" || wf.gotRunnerTaskID != "t-1" {
		t.Errorf("ResolveCurrentRunner called with step=%q task=%q, want step-42/t-1",
			wf.gotRunnerStepID, wf.gotRunnerTaskID)
	}
}

// TestSeatCasterAdapter_ScopesExclusionReadToCallersWorkflow covers the P1
// fix: the cross-step exclusion read must scope to the workflowID the
// caller passes, not read task-wide across every workflow the task has ever
// been on. A workflow the task switched away from durably keeps its
// workflow_step_participants rows (switch_workflow), so an unscoped read
// would count a stale row from that abandoned workflow as "already seated"
// for the current one.
func TestSeatCasterAdapter_ScopesExclusionReadToCallersWorkflow(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	if _, err := a.CastParticipantSeat(context.Background(), "wf-current", "t-1", "step-1", "reviewer"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if wf.gotParticipantsFor != "t-1" || wf.gotParticipantsForWF != "wf-current" {
		t.Errorf("ListParticipantsForTaskWorkflow called with task=%q workflow=%q, want t-1/wf-current",
			wf.gotParticipantsFor, wf.gotParticipantsForWF)
	}
}

// TestSeatCasterAdapter_ReviewerPoolIncludesSpecialists is proof shape (a)
// from the task plan: 2 ceos + specialists. Reviewer is cast first with no
// exclusions in play, so it seats the earliest candidate; approver's
// narrower ceo-only pool then excludes that agent, landing on the other
// ceo. Fails with the change reverted, since the old code seats ceo-1 for
// both roles.
func TestSeatCasterAdapter_ReviewerPoolIncludesSpecialists(t *testing.T) {
	tCEO1 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	tCEO2 := tCEO1.Add(time.Minute)
	tSpec := tCEO1.Add(2 * time.Minute)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "ceo-1", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tCEO1},
			{ID: "ceo-2", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tCEO2},
			{ID: "spec-1", Role: models.AgentRoleSpecialist, Status: models.AgentStatusIdle, CreatedAt: tSpec},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	reviewer, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-review", "reviewer")
	if err != nil {
		t.Fatalf("reviewer cast: %v", err)
	}
	if reviewer.AgentProfileID != "ceo-1" {
		t.Fatalf("reviewer = %q, want ceo-1 (earliest created_at)", reviewer.AgentProfileID)
	}
	// Pins the regression a reverted seat_caster.go:132 (Role: ceo) would
	// reintroduce: the pool union can only be expressed by listing with no
	// Role filter and applying the allowlist in Go (AC-OFFICE-REVIEW-SEATS-002.2).
	if office.gotFilter.Role != "" {
		t.Errorf("reviewer cast: filter role = %q, want empty (pool allowlist is applied in Go, not pushed to the shared listing)", office.gotFilter.Role)
	}

	wf.seatedAgentIDs = []string{reviewer.AgentProfileID}
	approver, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("approver cast: %v", err)
	}
	if approver.AgentProfileID != "ceo-2" {
		t.Fatalf("approver = %q, want ceo-2 (ceo-1 excluded; the approver pool never includes spec-1)", approver.AgentProfileID)
	}
	if office.gotFilter.Role != "" {
		t.Errorf("approver cast: filter role = %q, want empty (pool allowlist is applied in Go, not pushed to the shared listing)", office.gotFilter.Role)
	}
	if reviewer.AgentProfileID == approver.AgentProfileID {
		t.Fatal("expected reviewer and approver to be different agents")
	}
}

// TestSeatCasterAdapter_ReviewerWidensToOlderSpecialist is proof shape (b):
// 1 ceo + 1 specialist, with the specialist older. Reviewer's widened pool
// picks the specialist over the ceo by created_at; approver's ceo-only pool
// never sees the specialist at all. Fails with the change reverted, since
// the old code never considers the specialist for either role.
func TestSeatCasterAdapter_ReviewerWidensToOlderSpecialist(t *testing.T) {
	tSpec := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	tCEO := tSpec.Add(time.Minute)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "spec-1", Role: models.AgentRoleSpecialist, Status: models.AgentStatusIdle, CreatedAt: tSpec},
			{ID: "ceo-1", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: tCEO},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	reviewer, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-review", "reviewer")
	if err != nil {
		t.Fatalf("reviewer cast: %v", err)
	}
	if reviewer.AgentProfileID != "spec-1" {
		t.Fatalf("reviewer = %q, want spec-1 (earliest created_at across ceo ∪ specialist)", reviewer.AgentProfileID)
	}
	// Pins the regression a reverted seat_caster.go:132 (Role: ceo) would
	// reintroduce: the pool union can only be expressed by listing with no
	// Role filter and applying the allowlist in Go (AC-OFFICE-REVIEW-SEATS-002.2).
	if office.gotFilter.Role != "" {
		t.Errorf("reviewer cast: filter role = %q, want empty (pool allowlist is applied in Go, not pushed to the shared listing)", office.gotFilter.Role)
	}

	wf.seatedAgentIDs = []string{reviewer.AgentProfileID}
	approver, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("approver cast: %v", err)
	}
	if approver.AgentProfileID != "ceo-1" {
		t.Fatalf("approver = %q, want ceo-1 (the approver pool is ceo-only)", approver.AgentProfileID)
	}
	if office.gotFilter.Role != "" {
		t.Errorf("approver cast: filter role = %q, want empty (pool allowlist is applied in Go, not pushed to the shared listing)", office.gotFilter.Role)
	}
	if reviewer.AgentProfileID == approver.AgentProfileID {
		t.Fatal("expected reviewer and approver to be different agents")
	}
}

// TestSeatCasterAdapter_ApproverPoolStaysCEOOnly pins the D-B ruling's
// accepted non-divergence: a workspace with exactly one ceo (plus any
// number of specialists) seats the same agent for both reviewer and
// approver, because the approver pool never widens to specialists. This is
// ruled behavior, not a regression — do not "fix" it by widening the
// approver pool.
func TestSeatCasterAdapter_ApproverPoolStaysCEOOnly(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	agents := []*models.AgentInstance{
		{ID: "ceo-1", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t0},
	}
	for i := 0; i < 4; i++ {
		agents = append(agents, &models.AgentInstance{
			ID:        fmt.Sprintf("spec-%d", i),
			Role:      models.AgentRoleSpecialist,
			Status:    models.AgentStatusIdle,
			CreatedAt: t0.Add(time.Duration(i+1) * time.Minute),
		})
	}
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: agents,
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: ""}
	a := NewSeatCasterAdapter(office, wf)

	reviewer, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-review", "reviewer")
	if err != nil {
		t.Fatalf("reviewer cast: %v", err)
	}
	if reviewer.AgentProfileID != "ceo-1" {
		t.Fatalf("reviewer = %q, want ceo-1 (earliest created_at)", reviewer.AgentProfileID)
	}

	wf.seatedAgentIDs = []string{reviewer.AgentProfileID}
	approver, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("approver cast: %v", err)
	}
	if approver.AgentProfileID != "ceo-1" {
		t.Fatalf("approver = %q, want ceo-1 (the sole ceo; must not widen to specialists)", approver.AgentProfileID)
	}
	if !approver.SelfReview {
		t.Error("expected SelfReview when the only approver candidate is already seated as reviewer")
	}
}

// TestSeatCasterAdapter_ApproverPoolExcludesSpecialistsEvenWhenCEOPoolEmpty
// covers AC-OFFICE-REVIEW-SEATS-002.5/002.6 for the narrower approver pool:
// a workspace with specialists but no ceo still falls back to the runner
// for an approver seat, rather than reaching into the specialist pool that
// only reviewer is entitled to.
func TestSeatCasterAdapter_ApproverPoolExcludesSpecialistsEvenWhenCEOPoolEmpty(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "spec-1", Role: models.AgentRoleSpecialist, Status: models.AgentStatusIdle, CreatedAt: t0},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.AgentProfileID != "runner-agent" {
		t.Errorf("agent = %q, want runner-agent (approver pool is ceo-only and has no members here)", got.AgentProfileID)
	}
	if got.Provenance != engine.SeatProvenanceRunnerFallback {
		t.Errorf("provenance = %q, want %q", got.Provenance, engine.SeatProvenanceRunnerFallback)
	}
}

// TestSeatCasterAdapter_ExclusionNeverEmptiesASeat covers
// AC-OFFICE-REVIEW-SEATS-002.3's never-blocking guarantee: the sole
// candidate is already seated at another step on this task, and must be
// seated anyway rather than left unfillable.
func TestSeatCasterAdapter_ExclusionNeverEmptiesASeat(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "ceo-1", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t0},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "", seatedAgentIDs: []string{"ceo-1"}}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Unfillable {
		t.Fatal("expected the sole already-seated candidate to be seated anyway, not left unfillable")
	}
	if got.AgentProfileID != "ceo-1" {
		t.Errorf("agent = %q, want ceo-1", got.AgentProfileID)
	}
	if !got.SelfReview {
		t.Error("expected SelfReview when the only candidate already holds another seat on this task")
	}
}

// TestSeatCasterAdapter_ExclusionReadFailureIsNonFatal covers
// AC-OFFICE-REVIEW-SEATS-002.3's best-effort clause: a failure reading the
// cross-step exclusion signal must not fail the cast — it degrades to
// casting without exclusion.
func TestSeatCasterAdapter_ExclusionReadFailureIsNonFatal(t *testing.T) {
	t1 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Minute)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "ceo-1", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t1},
			{ID: "ceo-2", Role: models.AgentRoleCEO, Status: models.AgentStatusIdle, CreatedAt: t2},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "", participantsErr: errors.New("boom")}
	a := NewSeatCasterAdapter(office, wf)

	got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-approve", "approver")
	if err != nil {
		t.Fatalf("expected the seat to still be cast when the exclusion read fails: %v", err)
	}
	if got.AgentProfileID != "ceo-1" {
		t.Errorf("agent = %q, want ceo-1 (cast without exclusion when the read errors)", got.AgentProfileID)
	}
}

// TestSeatCasterAdapter_PoolAllowlistIsPositiveNotADenylist covers the
// guard called out in the task plan: the empty-Role listing this adapter
// now issues also returns worker/assistant/qa/security/devops and
// empty-role agent profiles, and the Go allowlist must exclude them by not
// naming their roles, not by naming roles to reject.
func TestSeatCasterAdapter_PoolAllowlistIsPositiveNotADenylist(t *testing.T) {
	t0 := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	office := &fakeOfficeRepo{
		fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"},
		agents: []*models.AgentInstance{
			{ID: "worker-1", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, CreatedAt: t0},
			{ID: "assistant-1", Role: models.AgentRoleAssistant, Status: models.AgentStatusIdle, CreatedAt: t0},
			{ID: "qa-1", Role: models.AgentRoleQA, Status: models.AgentStatusIdle, CreatedAt: t0},
			{ID: "security-1", Role: models.AgentRoleSecurity, Status: models.AgentStatusIdle, CreatedAt: t0},
			{ID: "devops-1", Role: models.AgentRoleDevOps, Status: models.AgentStatusIdle, CreatedAt: t0},
			{ID: "no-role-1", Role: "", Status: models.AgentStatusIdle, CreatedAt: t0},
		},
	}
	wf := &fakeSeatCasterWorkflowRepo{runner: "runner-agent"}
	a := NewSeatCasterAdapter(office, wf)

	for _, role := range []string{"reviewer", "approver"} {
		got, err := a.CastParticipantSeat(context.Background(), "wf-1", "t-1", "step-1", role)
		if err != nil {
			t.Fatalf("role %q: unexpected error: %v", role, err)
		}
		if got.AgentProfileID != "runner-agent" {
			t.Errorf("role %q: agent = %q, want runner-agent — none of worker/assistant/qa/security/devops/empty-role may be seated",
				role, got.AgentProfileID)
		}
		if got.Provenance != engine.SeatProvenanceRunnerFallback {
			t.Errorf("role %q: provenance = %q, want %q", role, got.Provenance, engine.SeatProvenanceRunnerFallback)
		}
	}
}

func TestSeatCasterAdapter_SatisfiesParticipantSeatCaster(t *testing.T) {
	var _ engine.ParticipantSeatCaster = (*SeatCasterAdapter)(nil)
}
