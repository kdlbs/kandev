package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Legacy resolutions lack a stream binding and predate the restored notice.
// They are historical proof only after the exact submission was settled unknown
// in the same saved conversation; current restored admission uses its strict verifier.
func (r *Repository) hasLegacyResolvedNativeResumeHistoryTx(ctx context.Context, tx *sqlx.Tx, recovery models.AgentDeliveryRecovery) (bool, error) {
	var valid bool
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM session_recovery_blocks b JOIN agent_delivery_submissions s
 ON s.id = b.delivery_submission_id AND s.session_id = b.session_id AND s.incarnation_id = b.incarnation_id AND s.harness_generation = b.expected_generation
 JOIN harness_session_generations g ON g.session_id = b.session_id AND g.incarnation_id = b.incarnation_id AND g.generation = b.expected_generation
 WHERE b.session_id = ? AND b.incarnation_id = ? AND b.expected_generation = ?
 AND b.consumer_reference = 'agent_delivery' AND b.state = 'resolved' AND b.authorized_action = 'resume'
 AND b.delivery_submission_id = ? AND b.delivery_stream_id = '' AND b.resolved_at <= ?
 AND s.state = 'interrupted_unknown' AND s.terminal_sequence = 0
 AND g.native_session_id <> '' AND g.native_session_id = (SELECT native_session_id FROM harness_session_generations WHERE session_id = g.session_id AND incarnation_id = g.incarnation_id ORDER BY generation DESC LIMIT 1)
 )`), recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration, recovery.SubmissionID, recovery.UpdatedAt).Scan(&valid)
	return valid, err
}

func (r *Repository) HasResolvedHistoricalNativeResumeRecovery(ctx context.Context, recovery models.AgentDeliveryRecovery) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	return r.hasResolvedNativeResumeRecoveryTx(ctx, tx, recovery)
}
