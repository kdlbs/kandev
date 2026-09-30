package coordinator

import (
	"context"
	"fmt"
	"time"
)

// SetOutcomeFenced writes outcome_json of an approving proposal only while
// token is still its claim token. It reports false when the claim was settled
// or re-issued, in which case nothing was written.
func (s *Store) SetOutcomeFenced(ctx context.Context, id, token, outcomeJSON string, now time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET outcome_json = ?, updated_at = ?
		WHERE id = ? AND status = ? AND claim_token = ?`),
		outcomeJSON, now.UTC(), id, string(ProposalStatusApproving), token)
	if err != nil {
		return false, fmt.Errorf("set proposal outcome: %w", err)
	}
	return matchedRow(res)
}

// CompleteKindTx marks an approving proposal (fenced by token) approved,
// recording the target task and, when non-empty, outcomeJSON. A nil outcome
// leaves a from_step_id written earlier in place.
func (s *Store) CompleteKindTx(ctx context.Context, exec coordinatorExec, id, token, taskID, outcomeJSON string, now time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, task_id = NULLIF(?, ''), outcome_json = CASE WHEN ? = '' THEN outcome_json ELSE ? END,
		    claim_token = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND claim_token = ?`),
		string(ProposalStatusApproved), taskID, outcomeJSON, outcomeJSON, now.UTC(),
		id, string(ProposalStatusApproving), token)
	if err != nil {
		return false, fmt.Errorf("complete proposal: %w", err)
	}
	return matchedRow(res)
}

// SettleStaleUnknownTx fails an approving proposal whose claim is older than
// staleBefore with error outcome_unknown, whatever its claim token. It never
// touches outcome_json.
func (s *Store) SettleStaleUnknownTx(ctx context.Context, exec coordinatorExec, id string, staleBefore, now time.Time) (bool, error) {
	res, err := exec.ExecContext(ctx, s.db.Rebind(`
		UPDATE coordinator_proposals
		SET status = ?, error = ?, claim_token = NULL, updated_at = ?
		WHERE id = ? AND status = ? AND claimed_at < ?`),
		string(ProposalStatusFailed), outcomeUnknownError, now.UTC(),
		id, string(ProposalStatusApproving), staleBefore.UTC())
	if err != nil {
		return false, fmt.Errorf("settle stale proposal: %w", err)
	}
	return matchedRow(res)
}
