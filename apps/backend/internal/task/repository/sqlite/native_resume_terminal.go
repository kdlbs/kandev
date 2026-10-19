package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// Terminal SQL outcomes remain immutable. The retained peer retirement proof
// must still match their exact identity and projected terminal inside admission.
func (r *Repository) verifyNativeResumeTerminalsTx(ctx context.Context, tx *sqlx.Tx, proof *models.NativeResumeDeliveryEvidence) (int, bool, error) {
	count := 0
	for _, row := range proof.Submissions {
		switch row.State {
		case "", models.DeliverySubmissionInterruptedUnknown:
			continue
		case models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled:
		default:
			return 0, false, nil
		}
		if row.Outcome != string(row.State) || row.TerminalSequence <= 0 || row.TerminalStreamID != row.StreamID {
			return 0, false, nil
		}
		var valid bool
		err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
   SELECT 1 FROM agent_delivery_submissions s JOIN agent_delivery_cursors c ON c.stream_id = s.terminal_stream_id
   WHERE s.id = ? AND s.session_id = ? AND s.incarnation_id = ? AND s.harness_generation = ?
   AND s.payload_hash = ? AND s.state = ? AND s.outcome = ? AND s.terminal_stream_id = ? AND s.terminal_sequence = ?
   AND c.session_id = s.session_id AND c.incarnation_id = s.incarnation_id AND c.harness_generation = s.harness_generation
   AND c.received_sequence = c.projected_sequence AND c.projected_sequence >= s.terminal_sequence
   AND c.retired_sequence >= 0 AND c.retired_sequence <= c.projected_sequence
  )`), row.ID, proof.SessionID, proof.IncarnationID, row.HarnessGeneration, row.PayloadHash, row.State, row.Outcome, row.TerminalStreamID, row.TerminalSequence).Scan(&valid)
		if err != nil || !valid {
			return 0, false, err
		}
		count++
	}
	return count, true, nil
}
