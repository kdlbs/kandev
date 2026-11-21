package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// Ancestry is bounded by the current canonical generation. Every intervening
// native conversation must have an authorized, consumed context continuation.
func (r *Repository) hasCompletedContextAncestryTx(ctx context.Context, tx *sql.Tx, recovery models.AgentDeliveryRecovery, target int64) (bool, error) {
	if target <= recovery.HarnessGeneration || !contextContinuationNoticeMatches(recovery, recovery.SessionID, recovery.IncarnationID, recovery.HarnessGeneration) {
		return false, nil
	}
	return r.hasConsumedContextAncestryTx(ctx, tx, recovery.SessionID, recovery.IncarnationID, recovery.SubmissionID, recovery.HarnessGeneration, target)
}

func (r *Repository) hasConsumedContextAncestryTx(ctx context.Context, tx *sql.Tx, sessionID, incarnationID, submissionID string, source, target int64) (bool, error) {
	var sourceOwned bool
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT EXISTS (
 SELECT 1 FROM task_sessions ts JOIN harness_session_generations source ON source.session_id = ts.id AND source.incarnation_id = ts.queue_incarnation_id
 JOIN agent_delivery_submissions s ON s.session_id = ts.id AND s.incarnation_id = source.incarnation_id AND s.harness_generation = source.generation
 WHERE ts.id = ? AND source.incarnation_id = ? AND source.generation = ? AND source.native_session_id <> ''
 AND s.id = ? AND s.state IN ('dispatching', 'interrupted_unknown')
 AND ? = (SELECT MAX(generation) FROM harness_session_generations WHERE session_id = ts.id AND incarnation_id = source.incarnation_id)
 )`), sessionID, incarnationID, source, submissionID, target).Scan(&sourceOwned)
	if err != nil || !sourceOwned {
		return false, err
	}
	var count, invalid int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN
 g.predecessor_generation = g.generation - 1 AND g.creation_reason = 'context_continued' AND g.native_session_id <> ''
 AND EXISTS (SELECT 1 FROM session_restore_attempts a JOIN session_continuation_snapshots s ON s.attempt_id = a.id
 WHERE a.session_id = g.session_id AND a.incarnation_id = g.incarnation_id AND a.expected_generation = g.predecessor_generation
 AND a.action = 'continue_from_history' AND a.authorized = 1 AND a.outcome = 'context_continued' AND a.completed_at IS NOT NULL
 AND s.session_id = g.session_id AND s.target_generation = g.generation AND s.status = 'consumed')
 THEN 0 ELSE 1 END), 0)
 FROM harness_session_generations g WHERE g.session_id = ? AND g.incarnation_id = ? AND g.generation > ? AND g.generation <= ?`),
		sessionID, incarnationID, source, target).Scan(&count, &invalid)
	return count == target-source && invalid == 0, err
}

func (r *Repository) completeContextContinuationAttempt(ctx context.Context, id string, completedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var sessionID string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT session_id FROM session_restore_attempts WHERE id = ?`), id).Scan(&sessionID); err != nil {
		return err
	}
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return err
	}
	if err := lockAgentDeliveryWrites(ctx, tx, r.db.DriverName(), []string{sessionID}, nil); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE session_restore_attempts SET outcome = 'context_continued', completed_at = ? WHERE id = ?`), completedAt, id)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return sql.ErrNoRows
	}
	if err := r.retireCompletedContextNoticeTx(ctx, tx, sessionID, completedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) retireCompletedContextNoticeTx(ctx context.Context, tx *sql.Tx, sessionID string, completedAt time.Time) error {
	var raw sql.NullString
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT metadata FROM task_sessions WHERE id = ?`), sessionID).Scan(&raw); err != nil {
		return err
	}
	metadata, err := decodeAgentDeliveryMetadata(raw)
	if err != nil {
		return err
	}
	recovery, exists, err := agentDeliveryRecoveryFromMetadata(metadata)
	if err != nil || !exists {
		return err
	}
	if recovery.SessionID != sessionID {
		return nil
	}
	var target int64
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(MAX(generation), 0) FROM harness_session_generations WHERE session_id = ? AND incarnation_id = ?`), sessionID, recovery.IncarnationID).Scan(&target)
	if err != nil {
		return err
	}
	valid, err := r.hasCompletedContextAncestryTx(ctx, tx, recovery, target)
	if err != nil || !valid {
		return err
	}
	return r.writeContinuedRecoveryNoticeTx(ctx, tx, metadata, raw, recovery, completedAt)
}
