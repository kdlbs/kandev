package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

// VerifyCommittedRecoveryNoticeSource binds publication to the exact resolved
// incident. The submission label may advance after recovery commits.
func (r *Repository) VerifyCommittedRecoveryNoticeSource(
	ctx context.Context,
	source models.AgentDeliveryRecovery,
) (bool, error) {
	if source.SessionID == "" || source.IncarnationID == "" || source.HarnessGeneration <= 0 ||
		source.SubmissionID == "" {
		return false, nil
	}
	var committed bool
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT EXISTS (
		SELECT 1
		FROM task_sessions ts
		JOIN session_recovery_blocks b ON b.session_id = ts.id
		JOIN agent_delivery_submissions s ON s.id = b.delivery_submission_id
		WHERE ts.id = ? AND ts.queue_incarnation_id = ?
		  AND b.incarnation_id = ? AND b.expected_generation = ?
		  AND b.consumer_reference = 'agent_delivery'
		  AND b.reason IN ('unknown_prompt_outcome', 'unresolved_durable_work')
		  AND b.delivery_submission_id = ? AND b.delivery_stream_id <> ''
		  AND (? = '' OR b.delivery_stream_id = ?)
		  AND b.state = 'resolved' AND b.authorized_action IN ('resume', 'silent_restart_restore')
		  AND s.session_id = ts.id AND s.incarnation_id = b.incarnation_id
		  AND s.harness_generation = b.expected_generation
		  AND s.state IN ('interrupted_unknown', 'completed', 'failed', 'cancelled')
	)`), source.SessionID, source.IncarnationID, source.IncarnationID,
		source.HarnessGeneration, source.SubmissionID, source.StreamID, source.StreamID).Scan(&committed)
	return committed, err
}
