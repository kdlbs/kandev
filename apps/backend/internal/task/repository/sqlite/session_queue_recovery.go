package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// ListQueueRecoverySessions deliberately omits names, paths, configuration and
// metadata. It rejects an oversized task rather than returning a partial receipt.
func (r *Repository) ListQueueRecoverySessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT id, task_id, queue_incarnation_id, state
		FROM task_sessions WHERE task_id = ? ORDER BY id LIMIT 257`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make([]*models.TaskSession, 0)
	for rows.Next() {
		if len(result) == 256 {
			return nil, messagequeue.ErrRecoveryLimit
		}
		session := &models.TaskSession{}
		if err := rows.Scan(&session.ID, &session.TaskID, &session.QueueIncarnationID, &session.State); err != nil {
			return nil, err
		}
		result = append(result, session)
	}
	return result, rows.Err()
}
