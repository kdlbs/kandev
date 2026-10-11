package sqlite

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// A generic fence has no submission binding. Native resume must have retired
// authenticated journal work before this transaction checks SQL admission.
func (r *Repository) canResolveGenericNativeResumeTx(ctx context.Context, tx *sqlx.Tx, blockID string, recovery models.AgentDeliveryRecovery, exists bool) (bool, error) {
	var valid bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks b JOIN task_sessions ts ON ts.id = b.session_id
 JOIN harness_session_generations g ON g.session_id = ts.id AND g.incarnation_id = ts.queue_incarnation_id
 WHERE b.id = ? AND b.state = 'open' AND b.reason = 'unresolved_durable_work'
 AND b.consumer_reference = 'agent_delivery' AND b.delivery_submission_id = '' AND b.delivery_stream_id = ''
 AND b.delivery_sequence = 0 AND b.delivery_turn_id = '' AND b.delivery_outcome = ''
 AND b.incarnation_id = ts.queue_incarnation_id AND b.expected_generation = g.generation AND g.native_session_id <> ''
 AND g.generation = (SELECT MAX(generation) FROM harness_session_generations WHERE session_id = ts.id AND incarnation_id = ts.queue_incarnation_id)
 AND 1 = (SELECT COUNT(*) FROM session_recovery_blocks WHERE session_id = ts.id AND incarnation_id = b.incarnation_id AND expected_generation = b.expected_generation AND state = 'open')
 AND EXISTS (SELECT 1 FROM agent_delivery_submissions WHERE session_id = ts.id AND incarnation_id = b.incarnation_id AND harness_generation = b.expected_generation AND state = 'interrupted_unknown' AND terminal_sequence = 0)
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_submissions WHERE session_id = ts.id AND state IN ('prepared', 'accepted', 'dispatching'))
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_cursors WHERE session_id = ts.id AND incarnation_id = b.incarnation_id AND harness_generation = b.expected_generation AND received_sequence <> projected_sequence)
 AND NOT EXISTS (SELECT 1 FROM session_restore_attempts WHERE session_id = ts.id AND completed_at IS NULL)
 )`), blockID).Scan(&valid)
	if err != nil || !valid || !exists {
		return valid, err
	}
	var sameOwner bool
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
		SELECT 1 FROM session_recovery_blocks WHERE id = ? AND session_id = ? AND incarnation_id = ? AND expected_generation >= ?)`),
		blockID, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration).Scan(&sameOwner)
	if err != nil || !sameOwner {
		return false, err
	}
	return r.hasResolvedNativeResumeRecoveryTx(ctx, tx, recovery)
}

func (r *Repository) hasResolvedNativeResumeRecoveryTx(ctx context.Context, tx *sqlx.Tx, recovery models.AgentDeliveryRecovery) (bool, error) {
	if recovery.Phase != models.AgentDeliveryRecoveryRestored {
		return false, nil
	}
	var valid bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks WHERE session_id = ? AND incarnation_id = ? AND expected_generation = ?
 AND consumer_reference = 'agent_delivery' AND state = 'resolved' AND authorized_action = 'resume'
 AND delivery_submission_id = ? AND delivery_stream_id = ? AND resolved_at = ?)`),
		recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration, recovery.SubmissionID, recovery.StreamID, recovery.UpdatedAt).Scan(&valid)
	if err != nil || valid {
		return valid, err
	}
	return r.hasLegacyResolvedNativeResumeHistoryTx(ctx, tx, recovery)
}
