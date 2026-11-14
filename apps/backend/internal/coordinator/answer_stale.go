package coordinator

import (
	"context"
	"encoding/json"
	"time"

	"go.uber.org/zap"
)

const supersedeAttempts = 3

func refusedRow(p *Proposal, code string) ActivityRow {
	reason := code
	id := p.ID
	return ActivityRow{
		CoordinatorID: p.CoordinatorID, WorkspaceID: p.WorkspaceID, ActionClass: ActionAnswer,
		Outcome: ActivityRefused, Authorization: AuthRequiresApproval, ReasonCode: &reason,
		TargetTaskID: p.TargetTaskID, ProposalID: &id,
	}
}

// settleRefusedTx refuses an open proposal inside tx and, only when a row
// matched, records the refusal. A second call matches nothing and records
// nothing.
func (s *Service) settleRefusedTx(ctx context.Context, tx coordinatorExec, p *Proposal, code string) (bool, error) {
	matched, err := s.store.RefuseOpenTx(ctx, tx, p.ID, p.UpdatedAt, code, time.Now())
	if err != nil || !matched {
		return false, err
	}
	return true, s.Record(ctx, tx, refusedRow(p, code))
}

// refuseKind settles a claimed proposal as refused. A claim another path
// already settled returns the current row.
func (s *Service) refuseKind(ctx context.Context, proposal *Proposal, token, code string) (*Proposal, error) {
	matched, err := s.settleDecision(ctx, proposal.CoordinatorID, refusedRow(proposal, code), func(tx coordinatorExec) (bool, error) {
		return s.store.RefuseClaimedTx(ctx, tx, proposal.ID, token, code, time.Now())
	})
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.fencedResult(ctx, proposal)
	}
	s.publishCoordinatorUpdated(ctx, proposal.WorkspaceID, proposal.CoordinatorID)
	s.logger.Info("proposal refused", zap.String("proposal_id", proposal.ID), zap.String("kind", proposal.Kind), zap.String("code", code))
	return s.store.GetProposal(ctx, proposal.WorkspaceID, proposal.CoordinatorID, proposal.ID, s.phase2)
}

// isRefusedAnswer reports whether p is a refused answer proposal: rejected
// with a refusal code and no decider, never a person's reject.
func isRefusedAnswer(p *Proposal) bool {
	return p != nil && p.Kind == ProposalKindAnswer && p.Status == ProposalStatusRejected && p.Error != nil &&
		(*p.Error == refuseStale || *p.Error == refuseTaskArchived)
}

// Supersede decides what an open answer proposal means for a new call, inside
// the propose transaction.
func (k *answerKind) Supersede(ctx context.Context, tx coordinatorExec, open *Proposal, args json.RawMessage) (SupersedeVerdict, error) {
	scope := proposeScopeFrom(ctx)
	for i := 0; i < supersedeAttempts && open != nil; i++ {
		if k.OpenMatches(open, args) {
			return SupersedeDedupe, nil
		}
		settle, err := k.decideOpen(open, scope)
		if err != nil {
			return SupersedeProceed, err
		}
		if settle {
			matched, err := k.svc.settleRefusedTx(ctx, tx, open, refuseStale)
			if err != nil || matched {
				return SupersedeProceed, err
			}
		}
		taskID := ""
		if open.TargetTaskID != nil {
			taskID = *open.TargetTaskID
		}
		var rerr error
		if open, rerr = k.svc.store.OpenTargetProposal(ctx, tx, open.CoordinatorID, ProposalKindAnswer, taskID); rerr != nil {
			return SupersedeProceed, rerr
		}
	}
	if open != nil {
		return SupersedeProceed, &ProposeRefusal{Code: refusalAnswerPending, Field: fieldTaskID}
	}
	return SupersedeProceed, nil
}

// decideOpen returns true when the open proposal is superseded and must be
// refused stale, nil when the call is refused with a typed error.
func (k *answerKind) decideOpen(open *Proposal, scope *proposeScope) (bool, error) {
	pending := &ProposeRefusal{Code: refusalAnswerPending, Field: fieldTaskID}
	switch {
	case open.Status == ProposalStatusApproving:
		return false, &ProposeRefusal{Code: refusalApprovalInProgress, Field: fieldTaskID}
	case open.Error != nil && *open.Error == outcomeUnknownError:
		return false, pending
	case scope == nil || !scope.TargetsRead || !open.CreatedAt.Before(scope.ReadStartedAt):
		return false, pending
	}
	var stored answerSpec
	if json.Unmarshal([]byte(open.RawSpec), &stored) != nil {
		return false, pending
	}
	if targetIdentityMatches(scope.Targets, stored.Target, stored.PendingID, stored.RequestID) {
		return false, pending
	}
	return true, nil
}
