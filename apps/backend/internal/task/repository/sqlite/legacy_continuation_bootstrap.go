package sqlite

import "context"

// HasPreparedLegacyContinuationBootstrap authorizes setup replay only while an
// explicit prepared continuation owns a session's first canonical generation.
// It creates no checkpoint, receipt, generation, or submission.
func (r *Repository) HasPreparedLegacyContinuationBootstrap(ctx context.Context, sessionID, incarnationID, executionID string) (bool, error) {
	var valid bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT EXISTS (
 SELECT 1 FROM task_sessions ts
 JOIN session_restore_attempts a ON a.session_id = ts.id AND a.incarnation_id = ts.queue_incarnation_id
 JOIN session_continuation_snapshots s ON s.attempt_id = a.id AND s.session_id = ts.id
 JOIN executors_running e ON e.session_id = ts.id AND e.agent_execution_id = ?
 WHERE ts.id = ? AND ts.queue_incarnation_id = ? AND ts.state <> 'CANCELLED'
 AND a.action = 'continue_from_history' AND a.authorized = 1 AND a.expected_generation = 0
 AND a.outcome = 'blocked' AND a.completed_at IS NULL
 AND s.target_generation = 1 AND s.status = 'prepared' AND s.submission_id <> '' AND s.byte_count > 0 AND s.content <> ''
 AND NOT EXISTS (SELECT 1 FROM harness_session_generations g WHERE g.session_id = ts.id AND g.incarnation_id = ts.queue_incarnation_id)
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_submissions d WHERE d.session_id = ts.id AND d.incarnation_id = ts.queue_incarnation_id)
 AND NOT EXISTS (SELECT 1 FROM agent_delivery_cursors c WHERE c.session_id = ts.id AND c.incarnation_id = ts.queue_incarnation_id)
 )`), executionID, sessionID, incarnationID).Scan(&valid)
	return valid, err
}
