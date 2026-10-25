package engine_adapters

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// SeatCasterWorkflowRepo captures the workflow-repo subset the seat caster
// needs: resolving the task's runner (REQ-OFFICE-REVIEW-SEATS-002's "the
// task's runner" fallback and self-review comparison), and listing every
// participant seat already recorded for the task's current workflow, for
// the cross-step exclusion in castFromCandidates. The caller supplies the
// immutable step that the task entered, so this adapter does not re-read
// mutable task state after the transition commits.
type SeatCasterWorkflowRepo interface {
	ResolveCurrentRunner(ctx context.Context, stepID, taskID string) (string, error)
	ListParticipantsForTaskWorkflow(ctx context.Context, taskID, workflowID string) ([]*wfmodels.WorkflowStepParticipant, error)
}

// seatRolePools maps a participant role to the set of agent roles eligible
// for its seat, per AC-OFFICE-REVIEW-SEATS-002.1: approver draws from `ceo`;
// reviewer draws from `ceo` ∪ `specialist` — and only those two. Any other
// participant role (watcher, collaborator, ...) keeps today's ceo-only
// behavior; the AC does not cover them, and the pool must not be widened
// without an amended AC.
var seatRolePools = map[string]map[models.AgentRole]bool{
	string(wfmodels.ParticipantRoleReviewer): {
		models.AgentRoleCEO:        true,
		models.AgentRoleSpecialist: true,
	},
	string(wfmodels.ParticipantRoleApprover): {
		models.AgentRoleCEO: true,
	},
}

// eligibleAgentRolesFor returns role's agent-role allowlist. It is always a
// positive match, never a denylist: eligibleCandidates lists agents with no
// Role filter (a union cannot be expressed as a single filter value), so
// this allowlist is what keeps worker/assistant/empty-role agent profiles
// from ever being seated.
func eligibleAgentRolesFor(role string) map[models.AgentRole]bool {
	if pool, ok := seatRolePools[role]; ok {
		return pool
	}
	return map[models.AgentRole]bool{models.AgentRoleCEO: true}
}

// SeatCasterAdapter implements engine.ParticipantSeatCaster by applying the
// casting resolution algorithm of REQ-OFFICE-REVIEW-SEATS-002 (system design
// "Casting resolution"):
//
//  1. List the task's workspace's Office agents eligible for role's seat
//     (eligibleAgentRolesFor) and whose status is neither `stopped` nor
//     `pending_approval`, ordered by `created_at` then agent profile
//     identifier.
//  2. Empty list: seat the task's runner (fallback provenance); no runner
//     resolves is unfillable.
//  3. Otherwise seat the first candidate that is neither the task's runner
//     nor already holding another seat on this task's current workflow
//     (best-effort cross-step exclusion, AC-OFFICE-REVIEW-SEATS-002.3) —
//     this is what lets a reviewer and an approver land on different agents
//     when the workspace shape allows it. If no such candidate exists, fall
//     back to the runner-exclusion-only rule (skip the runner if any other
//     candidate remains, otherwise seat the runner) so a seat is never left
//     empty.
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
	ctx context.Context, workflowID, taskID, stepID, role string,
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

	candidates, err := a.eligibleCandidates(ctx, fields.WorkspaceID, role)
	if err != nil {
		return engine.ParticipantSeatCastResult{}, fmt.Errorf("list eligible candidates: %w", err)
	}

	return castFromCandidates(candidates, runner, a.alreadySeatedAgents(ctx, taskID, workflowID), fields.WorkspaceID), nil
}

// eligibleCandidates returns the workspace's Office agents eligible for
// role's seat (eligibleAgentRolesFor) whose status is neither `stopped` nor
// `pending_approval`, ordered by `created_at` ascending then agent profile
// identifier ascending (AC-OFFICE-REVIEW-SEATS-002.1, -002.2).
//
// ListAgentInstancesFiltered's Role filter is a single string and cannot
// express the reviewer pool's ceo ∪ specialist union, so this lists with an
// empty Role filter and applies the allowlist here instead of pushing a
// union down to the shared listing method (AC-OFFICE-REVIEW-SEATS-002.2 —
// a shared listing whose behavior changes for everyone is how an unrelated
// Office surface silently loses agents).
func (a *SeatCasterAdapter) eligibleCandidates(
	ctx context.Context, workspaceID, role string,
) ([]*models.AgentInstance, error) {
	agents, err := a.Office.ListAgentInstancesFiltered(ctx, workspaceID, sqlite.AgentListFilter{})
	if err != nil {
		return nil, err
	}
	allowedRoles := eligibleAgentRolesFor(role)
	eligible := make([]*models.AgentInstance, 0, len(agents))
	for _, ag := range agents {
		if !allowedRoles[ag.Role] {
			continue
		}
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

// alreadySeatedAgents returns the set of agent profile ids already holding a
// participant seat anywhere on taskID's current workflow (workflowID), for
// castFromCandidates' best-effort cross-step exclusion. Scoping to workflowID
// keeps rows left over from a workflow the task has since switched away from
// (switch_workflow durably keeps them; see ListParticipantsForTaskWorkflow's
// doc comment) from ever counting as already seated. A read failure is
// non-fatal: the exclusion signal is optional and must never block casting a
// seat (AC-OFFICE-REVIEW-SEATS-002.3's best-effort clause), so this returns
// an empty set instead of propagating the error, and the caller casts
// without exclusion.
func (a *SeatCasterAdapter) alreadySeatedAgents(ctx context.Context, taskID, workflowID string) map[string]bool {
	seated := map[string]bool{}
	participants, err := a.Workflow.ListParticipantsForTaskWorkflow(ctx, taskID, workflowID)
	if err != nil {
		return seated
	}
	for _, p := range participants {
		if p.AgentProfileID != "" {
			seated[p.AgentProfileID] = true
		}
	}
	return seated
}

// castFromCandidates applies steps 2-5 of the casting resolution algorithm
// to an already-ordered, already-filtered candidate list, a resolved runner
// (which may be empty, meaning "does not resolve"), and the set of agent
// profile ids already seated elsewhere on this task's current workflow.
// workspaceID is stamped onto every result, including an Unfillable one, so
// the caller can emit AC-OFFICE-REVIEW-SEATS-004.1's warning record without
// a second lookup.
//
// The cross-step exclusion is best-effort and never blocking
// (AC-OFFICE-REVIEW-SEATS-002.3): if every candidate is either the runner or
// already seated, the runner-exclusion-only rule decides instead of leaving
// the seat empty. SelfReview is then true for either reason, since both mean
// the chosen agent is already participating in this task's review in some
// capacity.
func castFromCandidates(
	candidates []*models.AgentInstance, runner string, alreadySeated map[string]bool, workspaceID string,
) engine.ParticipantSeatCastResult {
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

	idx := -1
	for i, c := range candidates {
		if c.ID != runner && !alreadySeated[c.ID] {
			idx = i
			break
		}
	}
	if idx == -1 {
		idx = 0
		if candidates[0].ID == runner && len(candidates) > 1 {
			idx = 1
		}
	}
	chosen := candidates[idx]
	return engine.ParticipantSeatCastResult{
		AgentProfileID: chosen.ID,
		WorkspaceID:    workspaceID,
		Provenance:     engine.SeatProvenanceEligiblePool,
		SelfReview:     chosen.ID == runner || alreadySeated[chosen.ID],
	}
}

// Compile-time interface assertion.
var _ engine.ParticipantSeatCaster = (*SeatCasterAdapter)(nil)
