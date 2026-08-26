package engine_adapters

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// SeatCasterWorkflowRepo captures the workflow-repo subset the seat caster
// needs to resolve the task's runner (REQ-OFFICE-REVIEW-SEATS-002's "the
// task's runner" fallback and self-review comparison). The caller supplies
// the immutable step that the task entered, so this adapter does not re-read
// mutable task state after the transition commits.
type SeatCasterWorkflowRepo interface {
	ResolveCurrentRunner(ctx context.Context, stepID, taskID string) (string, error)
}

// SeatCasterAdapter implements engine.ParticipantSeatCaster by applying the
// casting resolution algorithm of REQ-OFFICE-REVIEW-SEATS-002 (system design
// "Casting resolution"):
//
//  1. List the task's workspace's Office agents whose role is `ceo` and whose
//     status is neither `stopped` nor `pending_approval`, ordered by
//     `created_at` then agent profile identifier.
//  2. Empty list: seat the task's runner (fallback provenance); no runner
//     resolves is unfillable.
//  3. Otherwise seat the candidate list's first member unless that member is
//     the task's runner and an alternative exists, in which case seat the
//     second member instead — this is what keeps an ordinary cast from
//     being a self-review when a different reviewer is available.
//
// The status exclusion is applied here, over ListAgentInstancesFiltered's
// result, rather than by adding a filter to that shared method or changing
// its default (AC-OFFICE-REVIEW-SEATS-002.2) — a shared listing whose
// behavior changes for everyone is how an unrelated Office surface silently
// loses agents.
type SeatCasterAdapter struct {
	Office   OfficeRepo
	Workflow SeatCasterWorkflowRepo
}

// NewSeatCasterAdapter builds a SeatCasterAdapter wrapping the office and
// workflow repos.
func NewSeatCasterAdapter(office OfficeRepo, workflow SeatCasterWorkflowRepo) *SeatCasterAdapter {
	return &SeatCasterAdapter{Office: office, Workflow: workflow}
}

// CastParticipantSeat satisfies engine.ParticipantSeatCaster.
func (a *SeatCasterAdapter) CastParticipantSeat(
	ctx context.Context, taskID, stepID, role string,
) (engine.ParticipantSeatCastResult, error) {
	if taskID == "" {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("task_id is required to cast a participant seat")
	}
	if stepID == "" {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("step_id is required to cast a participant seat")
	}
	fields, err := a.Office.GetTaskExecutionFields(ctx, taskID)
	if err != nil {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("get task workspace: %w", err)
	}
	if fields.WorkspaceID == "" {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("task %s has no workspace", taskID)
	}

	runner, err := a.Workflow.ResolveCurrentRunner(ctx, stepID, taskID)
	if err != nil {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("resolve task runner: %w", err)
	}

	candidates, err := a.eligibleCEOCandidates(ctx, fields.WorkspaceID)
	if err != nil {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("list eligible candidates: %w", err)
	}

	return castFromCandidates(candidates, runner, fields.WorkspaceID), nil
}

// eligibleCEOCandidates returns the workspace's Office agents whose role is
// `ceo` and whose status is neither `stopped` nor `pending_approval`,
// ordered by `created_at` ascending then agent profile identifier ascending
// (AC-OFFICE-REVIEW-SEATS-002.1, -002.2).
func (a *SeatCasterAdapter) eligibleCEOCandidates(
	ctx context.Context, workspaceID string,
) ([]*models.AgentInstance, error) {
	agents, err := a.Office.ListAgentInstancesFiltered(ctx, workspaceID, sqlite.AgentListFilter{
		Role: string(models.AgentRoleCEO),
	})
	if err != nil {
		return nil, err
	}
	eligible := make([]*models.AgentInstance, 0, len(agents))
	for _, ag := range agents {
		if ag.Status == models.AgentStatusStopped || ag.Status == models.AgentStatusPendingApproval {
			continue
		}
		eligible = append(eligible, ag)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		ti, tj := eligible[i].CreatedAt, eligible[j].CreatedAt
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return eligible[i].ID < eligible[j].ID
	})
	return eligible, nil
}

// castFromCandidates applies steps 2-5 of the casting resolution algorithm
// to an already-ordered, already-filtered candidate list and a resolved
// runner (which may be empty, meaning "does not resolve"). workspaceID is
// stamped onto every result, including an Unfillable one, so the caller can
// emit AC-OFFICE-REVIEW-SEATS-004.1's warning record without a second
// lookup.
func castFromCandidates(candidates []*models.AgentInstance, runner, workspaceID string) engine.ParticipantSeatCastResult {
	if len(candidates) == 0 {
		if runner == "" {
			return engine.ParticipantSeatCastResult{Unfillable: true, WorkspaceID: workspaceID}
		}
		return engine.ParticipantSeatCastResult{
			AgentProfileID: runner,
			WorkspaceID:    workspaceID,
			Provenance:     engine.SeatProvenanceRunnerFallback,
			SelfReview:     true,
		}
	}
	idx := 0
	if candidates[0].ID == runner && len(candidates) > 1 {
		idx = 1
	}
	chosen := candidates[idx]
	return engine.ParticipantSeatCastResult{
		AgentProfileID: chosen.ID,
		WorkspaceID:    workspaceID,
		Provenance:     engine.SeatProvenanceEligiblePool,
		SelfReview:     chosen.ID == runner,
	}
}

// Compile-time interface assertion.
var _ engine.ParticipantSeatCaster = (*SeatCasterAdapter)(nil)
