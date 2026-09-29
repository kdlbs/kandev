package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// approveKind is the approve path of a non-create kind: the status decision,
// edit validation, the policy re-check, the claim, then Execute. Nothing
// re-runs Execute on a claim that has gone stale (see settleStaleKind).
func (s *Service) approveKind(ctx context.Context, exec KindExecutor, proposal *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	switch proposal.Status {
	case ProposalStatusApproved, ProposalStatusRejected, ProposalStatusReturned:
		return nil, &ProposalConflictError{Proposal: proposal}
	case ProposalStatusApproving:
		return s.approveApprovingKind(ctx, proposal, edits)
	case ProposalStatusPending, ProposalStatusFailed:
		return s.claimKind(ctx, exec, proposal, edits)
	default:
		return nil, errUnknownStatus(proposal)
	}
}

func (s *Service) approveApprovingKind(ctx context.Context, proposal *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	if carriesKindEdits(edits) {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	cutoff := time.Now().UTC().Add(-staleApproveWindow)
	if proposal.ClaimedAt == nil || !proposal.ClaimedAt.Before(cutoff) {
		return nil, &ProposalConflictError{Proposal: proposal}
	}
	return s.reclaimStaleAndProceed(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, cutoff)
}

// claimKind validates the edits, re-checks policy, claims and executes.
func (s *Service) claimKind(ctx context.Context, exec KindExecutor, proposal *Proposal, edits ApproveProposalRequest) (*Proposal, error) {
	base := json.RawMessage(proposal.RawSpec)
	if proposal.RawFinalSpec != "" {
		base = json.RawMessage(proposal.RawFinalSpec)
	}
	body, err := json.Marshal(edits)
	if err != nil {
		return nil, err
	}
	final, err := exec.ValidateEdits(base, body)
	if err != nil {
		return nil, err
	}
	if err := s.recheckPolicy(ctx, proposal.CoordinatorID, proposal); err != nil {
		return nil, err
	}
	decidedBy := decidingUserID(ctx)
	token := uuid.New().String()
	matched, err := s.store.ClaimProposalRaw(ctx, proposal.ID, token, string(final), decidedBy, time.Now())
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.claimRaceResult(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID)
	}
	s.publishCoordinatorUpdated(ctx, proposal.WorkspaceID, proposal.CoordinatorID)
	s.logger.Info("proposal claimed", zap.String("proposal_id", proposal.ID), zap.String("kind", proposal.Kind))

	coordinator, err := s.store.GetCoordinatorByID(ctx, proposal.CoordinatorID)
	if err != nil {
		return s.failKind(ctx, exec, proposal, token, err.Error())
	}
	claim := Claim{
		ProposalID: proposal.ID, Token: token, WorkspaceID: proposal.WorkspaceID, Coordinator: coordinator,
		Spec: final, StartsAgent: proposal.StartsAgent, ApprovedBy: decidedBy,
		TextEdited: !sameSpec(final, json.RawMessage(proposal.RawSpec)),
	}
	if proposal.TargetTaskID != nil {
		claim.TargetTaskID = *proposal.TargetTaskID
	}
	return s.executeKind(ctx, exec, proposal, claim)
}

// executeKind runs Execute under the deadline and settles the row on a
// context that outlives it. A nil error settles approved even when the
// deadline fired during the call; an indefinite error after the deadline
// settles failed outcome_unknown.
func (s *Service) executeKind(ctx context.Context, exec KindExecutor, proposal *Proposal, claim Claim) (*Proposal, error) {
	ectx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.executeTimeout)
	out, err := exec.Execute(ectx, claim)
	deadlineFired := errors.Is(ectx.Err(), context.DeadlineExceeded)
	cancel()

	sctx, scancel := context.WithTimeout(context.WithoutCancel(ctx), settleBound)
	defer scancel()
	var failure *execFailure
	switch {
	case errors.Is(err, errSettleFenced):
		return s.fencedResult(sctx, proposal)
	case err == nil:
		return s.completeKind(sctx, exec, proposal, claim, out)
	case errors.As(err, &failure):
		return s.failKind(sctx, exec, proposal, claim.Token, failure.text)
	case deadlineFired || errors.Is(err, context.DeadlineExceeded):
		return s.failKind(sctx, exec, proposal, claim.Token, outcomeUnknownError)
	default:
		return s.failKind(sctx, exec, proposal, claim.Token, err.Error())
	}
}

// fencedResult returns the current row after a settle that another path won.
func (s *Service) fencedResult(ctx context.Context, proposal *Proposal) (*Proposal, error) {
	s.logger.Warn("execute_settle_fenced", zap.String("proposal_id", proposal.ID), zap.String("kind", proposal.Kind))
	return s.settleWriteRace(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, "")
}

func (s *Service) completeKind(ctx context.Context, exec KindExecutor, proposal *Proposal, claim Claim, out Outcome) (*Proposal, error) {
	target := out.TaskID
	row := ActivityRow{
		Edited: claim.TextEdited, CoordinatorID: proposal.CoordinatorID, WorkspaceID: proposal.WorkspaceID,
		ActionClass: exec.Action(), Outcome: ActivityApproved, Authorization: AuthRequiresApproval,
		TargetTaskID: optString(target), ProposalID: &proposal.ID, ActorUserID: optString(claim.ApprovedBy), Detail: out.Detail,
	}
	matched, err := s.settleDecision(ctx, proposal.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.CompleteKindTx(ctx, tx, proposal.ID, claim.Token, target, out.OutcomeJSON, time.Now())
	})
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.fencedResult(ctx, proposal)
	}
	s.publishCoordinatorUpdated(ctx, proposal.WorkspaceID, proposal.CoordinatorID)
	s.logger.Info("proposal approved", zap.String("proposal_id", proposal.ID), zap.String("kind", proposal.Kind), zap.String("task_id", target))
	return s.store.GetProposal(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, s.phase2)
}

func (s *Service) failKind(ctx context.Context, exec KindExecutor, proposal *Proposal, token, text string) (*Proposal, error) {
	row := s.failedKindRow(exec, proposal, text, decidingUserID(ctx))
	matched, err := s.settleDecision(ctx, proposal.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.FailProposalTx(ctx, tx, proposal.ID, token, text, time.Now())
	})
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.fencedResult(ctx, proposal)
	}
	s.publishCoordinatorUpdated(ctx, proposal.WorkspaceID, proposal.CoordinatorID)
	s.logger.Info("proposal approval failed", zap.String("proposal_id", proposal.ID), zap.String("kind", proposal.Kind), zap.String("error", text))
	return s.store.GetProposal(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, s.phase2)
}

func (s *Service) failedKindRow(exec KindExecutor, proposal *Proposal, text, actor string) ActivityRow {
	return ActivityRow{
		CoordinatorID: proposal.CoordinatorID, WorkspaceID: proposal.WorkspaceID, ActionClass: exec.Action(),
		Outcome: ActivityFailed, Authorization: AuthRequiresApproval, ReasonCode: optString(approvalFailedCode),
		TargetTaskID: proposal.TargetTaskID, ProposalID: &proposal.ID, ActorUserID: optString(actor), Detail: text,
	}
}

// settleStaleKind settles a stale non-create claim failed with outcome_unknown
// without running Execute: the side effect may already have happened, and the
// kind is not safe to run twice. The conditional update and its activity row
// commit together; a claim another path settled first is a conflict.
func (s *Service) settleStaleKind(ctx context.Context, current *Proposal, exec KindExecutor, cutoff time.Time) (*Proposal, error) {
	row := s.failedKindRow(exec, current, outcomeUnknownError, "")
	matched, err := s.settleDecision(ctx, current.CoordinatorID, row, func(tx coordinatorExec) (bool, error) {
		return s.store.SettleStaleUnknownTx(ctx, tx, current.ID, cutoff, time.Now())
	})
	if err != nil {
		return nil, err
	}
	if !matched {
		latest, err := s.store.GetProposal(ctx, current.WorkspaceID, current.CoordinatorID, current.ID, true)
		if err != nil {
			return nil, err
		}
		return nil, &ProposalConflictError{Proposal: latest}
	}
	s.publishCoordinatorUpdated(ctx, current.WorkspaceID, current.CoordinatorID)
	s.logger.Info("stale claim settled outcome_unknown", zap.String("proposal_id", current.ID), zap.String("kind", current.Kind))
	return s.store.GetProposal(ctx, current.WorkspaceID, current.CoordinatorID, current.ID, true)
}

func errUnknownStatus(p *Proposal) error {
	return fmt.Errorf("coordinator: proposal %s has unknown status %q", p.ID, p.Status)
}

// sameSpec reports whether two spec documents hold the same fields, ignoring
// key order.
func sameSpec(a, b json.RawMessage) bool {
	var x, y map[string]any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return string(a) == string(b)
	}
	return reflect.DeepEqual(x, y)
}
