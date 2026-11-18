package store

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// writeBuiltinApplicationIntents shares the preference transaction. A saved
// target never changes an in-flight attempt's execution identity or stage.
func writeBuiltinApplicationIntents(ctx context.Context, tx *sqlx.Tx, current *mcpconfig.BuiltinPolicySnapshot, next *mcpconfig.BuiltinTaskPreferences, surface p.Surface) error {
	before, err := mcpconfig.ResolveBuiltinPackages(next.WorkspaceID, next.TaskID, surface, current.Policy, current.Preferences)
	if err != nil {
		return err
	}
	after, err := mcpconfig.ResolveBuiltinPackages(next.WorkspaceID, next.TaskID, surface, current.Policy, next)
	if err != nil {
		return err
	}
	changed := !slices.Equal(before.Packages, after.Packages)
	hash := mcpconfig.BuiltinSelectionHash(after.Packages)
	requestID := uuid.NewString()
	// Equivalent preference edits still update the target revision of existing
	// requests, but cannot create a request or reset a failed attempt.
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET
 target_revision = ?, policy_revision = ?, requested_by = ?, request_id = ?, updated_at = ?, row_version = row_version + 1,
 retry_of_operation_id = CASE WHEN stage='failed' AND attempt_source<>'' THEN operation_id ELSE retry_of_operation_id END,
 stage = CASE WHEN ? AND selection_hash <> ? AND stage IN ('pending','applied','failed','superseded') THEN 'pending' ELSE stage END,
 reason = CASE WHEN ? AND selection_hash <> ? AND stage IN ('pending','applied','failed','superseded') THEN '' ELSE reason END,
 selection_hash = ? WHERE task_id = ? AND workspace_id = ?`),
		next.Revision, current.Policy.Revision, next.UpdatedBy, requestID, next.UpdatedAt, changed, hash, changed, hash, hash, next.TaskID, next.WorkspaceID)
	if err != nil || !changed {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_builtin_mcp_applications
 (session_id, task_id, workspace_id, target_revision, policy_revision, selection_hash, requested_by, request_id, created_at, updated_at)
 SELECT s.id, s.task_id, t.workspace_id, ?, ?, ?, ?, ?, ?, ?
 FROM task_sessions s JOIN tasks t ON t.id = s.task_id
 WHERE s.task_id = ? AND t.workspace_id = ? AND t.archived_at IS NULL
 AND s.state IN ('STARTING','RUNNING','WAITING_FOR_INPUT')
 ON CONFLICT (session_id) DO UPDATE SET
 workspace_id = excluded.workspace_id, target_revision = excluded.target_revision,
 policy_revision = excluded.policy_revision, selection_hash = excluded.selection_hash,
 requested_by = excluded.requested_by, request_id = excluded.request_id,
 attempt_generation = task_builtin_mcp_applications.attempt_generation + 1, attempt_revision = 0,
 source_execution_id = '', replacement_execution_id = '', stage = 'pending', reason = '',
 row_version = task_builtin_mcp_applications.row_version + 1, updated_at = excluded.updated_at
 WHERE task_builtin_mcp_applications.workspace_id <> excluded.workspace_id`),
		next.Revision, current.Policy.Revision, hash, next.UpdatedBy, requestID, next.UpdatedAt, next.UpdatedAt, next.TaskID, next.WorkspaceID)
	return err
}
