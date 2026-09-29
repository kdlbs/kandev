package coordinator

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// proposalRejectReasonMaxRunes is RejectProposalRequest.Reason's length
// limit, in Unicode code points after trimming
// (docs/specs/coordinator/system-design/proposals.md#reject).
const proposalRejectReasonMaxRunes = 500

// RejectProposal implements the reject route
// (docs/specs/coordinator/system-design/proposals.md#reject): read the
// proposal, a status other than pending or failed is a 409 conflict, the
// reason is trimmed and length-checked, then the store's CAS UPDATE moves
// pending or failed to rejected. A write that matches no row shares
// claimRaceResult with approve, since both share the same race semantics
// (404 when the row is gone, 409 with the current row otherwise) over the
// same `status IN ('pending','failed')` condition.
func (s *Service) RejectProposal(ctx context.Context, workspaceID, coordinatorID, proposalID string, req RejectProposalRequest) (*Proposal, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return nil, err
	}
	proposal, err := s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
	if err != nil {
		return nil, err
	}
	if err := s.checkRejectable(proposal); err != nil {
		return nil, err
	}
	if proposal.Status != ProposalStatusPending && proposal.Status != ProposalStatusFailed {
		return nil, &ProposalConflictError{Proposal: proposal}
	}

	reason := ""
	if req.Reason != nil {
		reason = *req.Reason
	}
	trimmed := strings.TrimSpace(reason)
	if n := utf8.RuneCountInString(trimmed); n > proposalRejectReasonMaxRunes {
		return nil, &FieldError{
			Field:   "reason",
			Message: fmt.Sprintf("reason must be at most %d characters", proposalRejectReasonMaxRunes),
		}
	}

	decidedBy := decidingUserID(ctx)
	matched, err := s.rejectProposalStore(ctx, proposal, trimmed, decidedBy)
	if err != nil {
		return nil, err
	}
	if !matched {
		return s.claimRaceResult(ctx, workspaceID, coordinatorID, proposalID)
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, coordinatorID)
	s.logger.Info("proposal rejected",
		zap.String("proposal_id", proposalID), zap.String("coordinator_id", coordinatorID), zap.String("workspace_id", workspaceID))
	return s.store.GetProposal(ctx, workspaceID, coordinatorID, proposalID, s.phase2)
}
