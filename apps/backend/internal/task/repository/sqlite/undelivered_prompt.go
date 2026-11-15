package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// FailUnsentAgentDeliverySubmission settles only the observed dispatch reservation.
// The caller must prove that no transport write was attempted.
func (r *Repository) FailUnsentAgentDeliverySubmission(ctx context.Context, expected *models.AgentDeliverySubmission, at time.Time) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), expected.SessionID); err != nil {
		return false, err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{expected.SessionID}, nil); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE agent_delivery_submissions
 SET state = ?, outcome = ?, fully_settled_at = ?, updated_at = ?
 WHERE id = ? AND session_id = ? AND incarnation_id = ? AND harness_generation = ?
 AND owner_generation = ? AND payload_hash = ? AND state = ?
 AND terminal_stream_id = '' AND terminal_sequence = 0 AND terminal_turn_id = ''`),
		models.DeliverySubmissionFailed, "failed", at, at, expected.ID, expected.SessionID,
		expected.IncarnationID, expected.HarnessGeneration, expected.OwnerGeneration, expected.PayloadHash, models.DeliverySubmissionDispatching)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count == 1, nil
}
