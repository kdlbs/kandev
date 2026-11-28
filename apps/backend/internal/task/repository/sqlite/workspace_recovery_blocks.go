package sqlite

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ListOpenWorkspaceRecoveryBlocks returns, read-only, the open session
// recovery blocks of one workspace that are waiting on an operator decision:
// the same blocks the task chat card projects (EnrichSessionRecoveryBlocks),
// i.e. state open on the session's current queue incarnation and its newest
// committed harness generation. Archived tasks and terminal sessions are
// excluded. Ordered oldest first; hasMore reports a further row beyond limit.
func (r *Repository) ListOpenWorkspaceRecoveryBlocks(
	ctx context.Context,
	workspaceID string,
	limit int,
) (blocks []models.WorkspaceRecoveryBlockSummary, hasMore bool, err error) {
	if limit < 1 {
		return nil, false, fmt.Errorf("ListOpenWorkspaceRecoveryBlocks: limit must be >= 1, got %d", limit)
	}
	query := fmt.Sprintf(`
		SELECT b.id, b.session_id, ts.state, ts.task_id, t.title, b.reason,
		       b.consumer_reference, b.created_at, b.updated_at
		FROM session_recovery_blocks b
		JOIN task_sessions ts ON ts.id = b.session_id
		JOIN tasks t ON t.id = ts.task_id
		WHERE t.workspace_id = ?
		  AND t.archived_at IS NULL
		  AND ts.state NOT IN ('%s', '%s', '%s')
		  AND b.state = ?
		  AND b.incarnation_id = COALESCE(NULLIF(ts.queue_incarnation_id, ''), ts.id)
		  AND b.expected_generation >= 1
		  AND b.expected_generation = (
			SELECT MAX(g.generation)
			FROM harness_session_generations g
			WHERE g.session_id = b.session_id AND g.incarnation_id = b.incarnation_id
		  )
		ORDER BY b.created_at ASC, b.id ASC
		LIMIT ?`,
		models.TaskSessionStateCompleted, models.TaskSessionStateFailed, models.TaskSessionStateCancelled,
	)
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(query), workspaceID, models.RecoveryBlockOpen, limit+1)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rows.Close() }()
	blocks = make([]models.WorkspaceRecoveryBlockSummary, 0)
	for rows.Next() {
		var block models.WorkspaceRecoveryBlockSummary
		if err := rows.Scan(&block.BlockID, &block.SessionID, &block.SessionState, &block.TaskID,
			&block.TaskTitle, &block.Reason, &block.ConsumerReference, &block.CreatedAt, &block.UpdatedAt); err != nil {
			return nil, false, err
		}
		blocks = append(blocks, block)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(blocks) > limit {
		return blocks[:limit], true, nil
	}
	return blocks, false, nil
}
