package coordinator

import (
	"context"
	"fmt"
	"time"
)

// RefuseOpenTx settles an open (pending or failed) proposal as refused: status
// rejected, error set to code, no decider. The update matches only the row
// state the caller observed (updated_at) and never a failed outcome_unknown
// row, so a refusal decided on a stale observation is a no-op. It reports
// whether a row matched.
func (s *Store) RefuseOpenTx(ctx context.Context, exec coordinatorExec, id string, observedUpdatedAt time.Time, code string, now time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, error = ?, decided_by = NULL, claim_token = NULL, updated_at = ?
		WHERE id = ? AND status IN (?, ?) AND updated_at = ? AND (error IS NULL OR error <> ?)`),
		string(ProposalStatusRejected), code, now.UTC(),
		id, string(ProposalStatusPending), string(ProposalStatusFailed), observedUpdatedAt, outcomeUnknownError)
	if err != nil {
		return false, fmt.Errorf("refuse open proposal: %w", err)
	}
	return matchedRow(res)
}

// RefuseClaimedTx settles an approving proposal as refused, fenced by the
// claim token.
func (s *Store) RefuseClaimedTx(ctx context.Context, exec coordinatorExec, id, token, code string, now time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, error = ?, decided_by = NULL, claim_token = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND claim_token = ?`),
		string(ProposalStatusRejected), code, now.UTC(),
		id, string(ProposalStatusApproving), token)
	if err != nil {
		return false, fmt.Errorf("refuse claimed proposal: %w", err)
	}
	return matchedRow(res)
}

// ClaimProposalFenced is ClaimProposalRaw for a kind whose claim must lose to
// any change since the approver read the row: it matches only the observed
// updated_at and never a failed outcome_unknown row.
func (s *Store) ClaimProposalFenced(ctx context.Context, id, token, finalJSON, decidedBy string, now, observedUpdatedAt time.Time) (bool, error) {
	now = now.UTC()
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, claimed_at = ?, claim_token = ?, final_spec_json = ?, decided_by = ?, error = NULL,
		    updated_at = ?, claimed_automatically = 0
		WHERE id = ? AND status IN (?, ?) AND updated_at = ? AND (error IS NULL OR error <> ?)`),
		string(ProposalStatusApproving), now, token, finalJSON, decidedBy, now,
		id, string(ProposalStatusPending), string(ProposalStatusFailed), observedUpdatedAt, outcomeUnknownError)
	if err != nil {
		return false, fmt.Errorf("claim proposal (fenced): %w", err)
	}
	return matchedRow(res)
}
