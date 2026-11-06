package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// VerifyUnprovenNativeResumeCompletion re-reads the historical completion
// predicate without changing SQL state. It validates SQL metadata only;
// admission separately requires the pinned per-stream cursor. Callers must
// already have authenticated the interrupted journal owner. The label is never
// accepted as terminal proof.
func (r *Repository) VerifyUnprovenNativeResumeCompletion(
	ctx context.Context,
	sessionID, incarnationID string,
	row models.NativeResumeSubmissionEvidence,
) (bool, error) {
	return r.verifyUnprovenNativeResumeCompletion(ctx, r.ro, sessionID, incarnationID, row)
}

func (r *Repository) verifyUnprovenNativeResumeCompletion(
	ctx context.Context,
	q sqlx.QueryerContext,
	sessionID, incarnationID string,
	row models.NativeResumeSubmissionEvidence,
) (bool, error) {
	if row.ID == "" || sessionID == "" || incarnationID == "" || row.HarnessGeneration <= 0 ||
		row.PayloadHash == "" || row.State != models.DeliverySubmissionCompleted ||
		row.Outcome != string(models.DeliverySubmissionCompleted) {
		return false, nil
	}
	var valid bool
	err := q.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
  SELECT 1 FROM agent_delivery_submissions s
  WHERE s.id = ? AND s.session_id = ? AND s.incarnation_id = ?
  AND s.harness_generation = ? AND s.payload_hash = ?
  AND s.state = ? AND s.outcome = ?
  AND s.terminal_stream_id = '' AND s.terminal_sequence = 0 AND s.terminal_turn_id = ''
  AND s.fully_settled_at IS NULL AND s.payload_pruned_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM agent_delivery_inbox i WHERE i.submission_id = s.id AND i.terminal <> 0)
  AND NOT EXISTS (SELECT 1 FROM agent_delivery_effects e WHERE e.submission_id = s.id AND e.effect_type = ?)
  )`), row.ID, sessionID, incarnationID, row.HarnessGeneration, row.PayloadHash,
		models.DeliverySubmissionCompleted, string(models.DeliverySubmissionCompleted),
		models.DeliveryTerminalEffectType,
	).Scan(&valid)
	return valid, err
}
