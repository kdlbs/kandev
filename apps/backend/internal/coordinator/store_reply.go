package coordinator

import (
	"context"
	"fmt"
	"time"
)

// ReturnProposalTx moves a pending proposal to returned, storing the reply
// text and the replying manager. It matches no row unless the proposal is
// still pending.
func (s *Store) ReturnProposalTx(ctx context.Context, exec coordinatorExec, id, replyText, decidedBy string, now time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, reply_text = ?, decided_by = ?, updated_at = ?
		WHERE id = ? AND status = ?`),
		string(ProposalStatusReturned), replyText, decidedBy, now.UTC(), id, string(ProposalStatusPending))
	if err != nil {
		return false, fmt.Errorf("return proposal: %w", err)
	}
	return matchedRow(res)
}

// ClaimReplyDelivery stamps the diagnostic delivery claim of a returned,
// undelivered reply. It gates nothing: matching no row means there is nothing
// left to deliver.
func (s *Store) ClaimReplyDelivery(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET reply_delivery_claimed_at = ?
		WHERE id = ? AND status = ? AND reply_delivered_at IS NULL`),
		now.UTC(), id, string(ProposalStatusReturned))
	if err != nil {
		return false, fmt.Errorf("claim reply delivery: %w", err)
	}
	return matchedRow(res)
}

// FinaliseReplyDelivery records that the reply reached the conversation. It
// reports whether this call changed the row.
func (s *Store) FinaliseReplyDelivery(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET reply_delivered_at = ?, reply_delivery_claimed_at = NULL
		WHERE id = ? AND reply_delivered_at IS NULL`),
		now.UTC(), id)
	if err != nil {
		return false, fmt.Errorf("finalise reply delivery: %w", err)
	}
	return matchedRow(res)
}

// ReleaseReplyClaim clears the delivery claim of a reply that was not
// delivered, leaving reply_delivered_at null.
func (s *Store) ReleaseReplyClaim(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET reply_delivery_claimed_at = NULL
		WHERE id = ? AND reply_delivered_at IS NULL`), id); err != nil {
		return fmt.Errorf("release reply claim: %w", err)
	}
	return nil
}
