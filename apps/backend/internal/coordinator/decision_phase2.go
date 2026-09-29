package coordinator

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrUnknownProposalKind reports a stored proposal whose kind this build does
// not execute. It maps to a 500 with nothing written: no claim, no status
// change and no activity row.
var ErrUnknownProposalKind = errors.New("coordinator: unknown proposal kind")

func knownProposalKind(kind string) bool {
	switch kind {
	case ProposalKindCreateTask, ProposalKindMessage, ProposalKindMove, ProposalKindResume:
		return true
	}
	return false
}

// checkApprovable rejects a proposal this build cannot approve. Only
// create_task proposals have an executor here.
func (s *Service) checkApprovable(p *Proposal) error {
	if !s.phase2 || p.Kind == "" || p.Kind == ProposalKindCreateTask {
		return nil
	}
	return fmt.Errorf("%w: %q cannot be approved", ErrUnknownProposalKind, p.Kind)
}

// checkRejectable rejects a proposal whose kind is not one of the four.
func (s *Service) checkRejectable(p *Proposal) error {
	if !s.phase2 || p.Kind == "" || knownProposalKind(p.Kind) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrUnknownProposalKind, p.Kind)
}

// settleDecision applies a status write and, when it matched a row, records
// row in the same coordinator-locked transaction. A coordinator that is gone
// reports matched=false so the caller's race handling answers 404.
func (s *Service) settleDecision(ctx context.Context, coordinatorID string, row ActivityRow, apply func(tx coordinatorExec) (bool, error)) (bool, error) {
	matched := false
	err := s.store.withCoordinatorLock(ctx, coordinatorID, func(tx coordinatorExec) error {
		m, err := apply(tx)
		if err != nil || !m {
			return err
		}
		matched = true
		return s.Record(ctx, tx, row)
	})
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return matched, nil
}

// specEdited reports whether the approved final spec differs from the proposed
// spec on any editable field.
func specEdited(p *Proposal) bool {
	if p.FinalSpec == nil {
		return false
	}
	f, o := p.FinalSpec, p.Spec
	return f.Title != o.Title || f.Description != o.Description || f.WorkflowID != o.WorkflowID ||
		f.StepID != o.StepID || f.RepositoryID != o.RepositoryID
}

// approvalFailedCode is the reason code carried by a failed approval row.
const approvalFailedCode = "approval_failed"

func (s *Service) completeProposalStore(ctx context.Context, workspaceID, coordinatorID, proposalID, token, taskID string) (bool, error) {
	if !s.phase2 {
		return s.store.CompleteProposal(ctx, proposalID, token, taskID, time.Now())
	}
	current, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	row := ActivityRow{
		Edited:        specEdited(current),
		CoordinatorID: coordinatorID, WorkspaceID: workspaceID, ActionClass: ActionCreateTask,
		Outcome: ActivityApproved, Authorization: AuthRequiresApproval,
		TargetTaskID: &taskID, ProposalID: &proposalID, ActorUserID: optString(decidingUserID(ctx)),
	}
	return s.settleDecision(ctx, coordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.CompleteProposalTx(ctx, tx, proposalID, token, taskID, time.Now())
	})
}

func (s *Service) failProposalStore(ctx context.Context, workspaceID, coordinatorID, proposalID, token, errMsg string) (bool, error) {
	if !s.phase2 {
		return s.store.FailProposal(ctx, proposalID, token, errMsg, time.Now())
	}
	row := ActivityRow{
		CoordinatorID: coordinatorID, WorkspaceID: workspaceID, ActionClass: ActionCreateTask,
		Outcome: ActivityFailed, Authorization: AuthRequiresApproval, ReasonCode: optString(approvalFailedCode),
		ProposalID: &proposalID, ActorUserID: optString(decidingUserID(ctx)), Detail: errMsg,
	}
	return s.settleDecision(ctx, coordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.FailProposalTx(ctx, tx, proposalID, token, errMsg, time.Now())
	})
}

func (s *Service) rejectProposalStore(ctx context.Context, p *Proposal, reason, decidedBy string) (bool, error) {
	if !s.phase2 {
		return s.store.RejectProposal(ctx, p.ID, reason, decidedBy, time.Now())
	}
	class := ActionUnknown
	if a := Action(p.Kind); isPolicyAction(a) {
		class = a
	}
	row := ActivityRow{
		CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: class,
		Outcome: ActivityRejected, Authorization: AuthRequiresApproval, ReasonCode: optString(reason),
		TargetTaskID: p.TargetTaskID, ProposalID: &p.ID, ActorUserID: optString(decidedBy), Detail: reason,
	}
	return s.settleDecision(ctx, p.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.RejectProposalTx(ctx, tx, p.ID, reason, decidedBy, time.Now())
	})
}

func optString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
