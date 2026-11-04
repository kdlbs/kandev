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
		if err == nil && !valid {
			valid, err = r.verifyPendingNativeResumeTerminal(ctx, tx, proof.SessionID, proof.IncarnationID, row)
		}
		if err != nil || !valid {
			return 0, false, err
		}
		count++
	}
	return count, true, nil
}

// VerifyPendingNativeResumeTerminal reads terminal authority without running
// settlement, which can release recovery blocks before peer authentication.
func (r *Repository) VerifyPendingNativeResumeTerminal(ctx context.Context, sessionID, incarnationID string, row models.NativeResumeSubmissionEvidence) (bool, error) {
	return r.verifyPendingNativeResumeTerminal(ctx, r.ro, sessionID, incarnationID, row)
}

func (r *Repository) verifyPendingNativeResumeTerminal(ctx context.Context, q sqlx.QueryerContext, sessionID, incarnationID string, row models.NativeResumeSubmissionEvidence) (bool, error) {
	if !validDeliveryTerminalOutcome(row.State) || row.Outcome != string(row.State) || row.PayloadHash == "" || row.TerminalSequence <= 0 || row.TerminalStreamID == "" || row.TerminalStreamID != row.StreamID {
		return false, nil
	}
	var valid bool
	err := q.QueryRowxContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM agent_delivery_submissions s
 JOIN agent_delivery_effects e ON e.submission_id = s.id
 JOIN agent_delivery_cursors c ON c.stream_id = e.stream_id
 WHERE s.id = ? AND s.session_id = ? AND s.incarnation_id = ? AND s.harness_generation = ?
 AND s.payload_hash = ? AND s.state = ? AND s.outcome = ?
 AND s.terminal_stream_id = '' AND s.terminal_sequence = 0 AND s.terminal_turn_id = ''
 AND s.fully_settled_at IS NULL AND s.payload_pruned_at IS NULL
 AND e.effect_type = ? AND e.state IN (?, ?)
 AND e.session_id = s.session_id AND e.incarnation_id = s.incarnation_id
 AND e.harness_generation = s.harness_generation AND e.outcome = s.outcome
 AND e.stream_id = ? AND e.sequence = ?
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_effects newer
   WHERE newer.submission_id = s.id AND newer.stream_id = e.stream_id
   AND newer.effect_type = e.effect_type AND newer.sequence > e.sequence)
 AND c.session_id = s.session_id AND c.incarnation_id = s.incarnation_id
 AND c.harness_generation = s.harness_generation
 AND c.received_sequence = c.projected_sequence AND c.projected_sequence >= e.sequence
 AND c.retired_sequence >= 0 AND c.retired_sequence <= c.projected_sequence
)`), row.ID, sessionID, incarnationID, row.HarnessGeneration, row.PayloadHash, row.State, row.Outcome,
		models.DeliveryTerminalEffectType, models.DeliveryEffectBlockResolved, models.DeliveryEffectCompleted,
		row.TerminalStreamID, row.TerminalSequence).Scan(&valid)
	return valid, err
}
