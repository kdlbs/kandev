package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ListAutomaticExecutionCandidates reads primary conversations; the consumer
// revalidates current policy and authority before any action.
func (r *Repository) ListAutomaticExecutionCandidates(ctx context.Context, after string, limit int) ([]models.SilentRestoreCandidate, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("automatic execution candidate limit must be between 1 and 200")
	}
	id := dialect.ByteOrderedText(r.db.DriverName(), "ts.id")
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT ts.task_id, ts.id, t.workspace_id, ts.state
		FROM task_sessions ts JOIN tasks t ON t.id = ts.task_id
		WHERE %s > ? AND ts.is_primary = 1 AND t.archived_at IS NULL
		AND (ts.state IN ('CREATED', 'FAILED') OR (ts.state = 'WAITING_FOR_INPUT' AND EXISTS (
			SELECT 1 FROM session_recovery_blocks b WHERE b.session_id = ts.id
			AND b.state = 'open' AND b.incarnation_id = COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id)
			AND b.reason IN ('native_state_missing', 'native_resume_unsupported')
		))) ORDER BY %s LIMIT ?`, id, id)), after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var candidates []models.SilentRestoreCandidate
	for rows.Next() {
		var candidate models.SilentRestoreCandidate
		if err := rows.Scan(&candidate.TaskID, &candidate.SessionID, &candidate.WorkspaceID, &candidate.State); err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}
