package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

// ClaimBuiltinMCPApplication reserves a saved target. Runtime admission must
// still establish settled activity. This reservation is not stop authority.
// The coordinator must revalidate policy, actor access, delivery, and helper resources.
func (r *Repository) ClaimBuiltinMCPApplication(ctx context.Context, sessionID, executionID string, version int64) (*models.BuiltinMCPApplication, error) {
	if sessionID == "" || executionID == "" || version <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET
 stage='preparing', reason='', source_execution_id=?, replacement_execution_id='',
 operation_id=?,attempt_source='',attempt_actor_id=requested_by,attempt_selection_hash=selection_hash,attempt_policy_revision=policy_revision,target_binding_id='',target_binding_json='',verification_deadline=?,
 attempt_generation=attempt_generation+1, attempt_revision=target_revision,
 row_version=row_version+1, updated_at=?
 WHERE session_id=? AND row_version=? AND stage='pending'
 AND EXISTS (SELECT 1 FROM task_sessions s JOIN tasks t ON t.id=s.task_id
 JOIN executors_running er ON er.session_id=s.id AND er.task_id=t.id
 JOIN task_builtin_mcp_preferences pref ON pref.task_id=t.id AND pref.workspace_id=t.workspace_id
 WHERE s.id=task_builtin_mcp_applications.session_id AND s.task_id=task_builtin_mcp_applications.task_id
 AND t.workspace_id=task_builtin_mcp_applications.workspace_id AND t.archived_at IS NULL
 AND s.state='WAITING_FOR_INPUT' AND er.agent_execution_id=? AND er.status='running'
 AND er.resumable=TRUE AND er.resume_token<>'' AND pref.revision=task_builtin_mcp_applications.target_revision)`),
		executionID, uuid.NewString(), time.Now().UTC().Add(2*time.Minute), time.Now().UTC(), sessionID, version, executionID)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows != 1 {
		return nil, nil
	}
	var application models.BuiltinMCPApplication
	err = tx.GetContext(ctx, &application, tx.Rebind(`SELECT * FROM task_builtin_mcp_applications WHERE session_id=?`), sessionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &application, nil
}
