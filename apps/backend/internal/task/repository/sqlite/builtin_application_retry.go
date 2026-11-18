package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

// RetryBuiltinMCPApplication admits one human retry of the current saved request.
// It preserves the failed operation as closed-source evidence for the helper.
func (r *Repository) RetryBuiltinMCPApplication(ctx context.Context, sessionID, actor, requestID string, version int64, target models.BuiltinMCPRetryTarget) (bool, error) {
	if actor == "" || requestID == "" || version <= 0 || target.SelectionHash == "" {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET stage='pending',reason='',policy_revision=?,selection_hash=?,
 retry_of_operation_id=CASE WHEN attempt_source<>'' THEN operation_id ELSE '' END,
 row_version=row_version+1,updated_at=? WHERE session_id=? AND request_id=? AND row_version=? AND (stage='failed' OR (stage='accepting' AND reason='acceptance_unconfirmed'))
 AND EXISTS (SELECT 1 FROM task_sessions s JOIN tasks t ON t.id=s.task_id WHERE s.id=task_builtin_mcp_applications.session_id AND s.task_id=task_builtin_mcp_applications.task_id AND t.workspace_id=task_builtin_mcp_applications.workspace_id AND t.archived_at IS NULL AND s.state='WAITING_FOR_INPUT')
 AND target_revision=? AND EXISTS (SELECT 1 FROM task_builtin_mcp_preferences pref WHERE pref.task_id=task_builtin_mcp_applications.task_id AND pref.workspace_id=task_builtin_mcp_applications.workspace_id AND pref.revision=?)
 AND COALESCE((SELECT revision FROM workspace_builtin_mcp_policies WHERE workspace_id=task_builtin_mcp_applications.workspace_id),0)=?`), target.PolicyRevision, target.SelectionHash, time.Now().UTC(), sessionID, requestID, version, target.TaskRevision, target.TaskRevision, target.PolicyRevision)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, nil
	}
	return true, tx.Commit()
}
