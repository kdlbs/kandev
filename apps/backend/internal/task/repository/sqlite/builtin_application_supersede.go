package sqlite

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

// SupersedeBuiltinMCPApplication settles a reverted selection without replacing
// its provider. The caller has verified the same live helper at a safe boundary.
func (r *Repository) SupersedeBuiltinMCPApplication(ctx context.Context, sessionID, executionID, sourceDigest string, version int64) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return false, err
	}
	// Pending rows have no reserved source execution yet.
	var raw string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(er.metadata,'{}') FROM executors_running er JOIN task_sessions s ON s.id=er.session_id AND s.task_id=er.task_id JOIN tasks t ON t.id=s.task_id WHERE er.session_id=? AND er.agent_execution_id=? AND er.status='running' AND s.state='WAITING_FOR_INPUT' AND t.archived_at IS NULL`), sessionID, executionID).Scan(&raw); err != nil {
		return false, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return false, err
	}
	current, err := builtinSupersedeProfile(metadata)
	if err != nil || current == nil {
		return false, err
	}

	frozenDigest, err := p.BuiltinBindingDigest(*current)
	if err != nil {
		return false, err
	}
	if frozenDigest != sourceDigest {
		return false, models.ErrExecutionRotated
	}
	hash := mcpconfig.BuiltinSelectionHash(current.BuiltinPackages.Packages)
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_builtin_mcp_applications SET stage='superseded',reason='already_current',row_version=row_version+1,updated_at=? WHERE session_id=? AND row_version=? AND stage='pending' AND retry_of_operation_id='' AND selection_hash=? AND task_id=? AND workspace_id=? AND EXISTS (SELECT 1 FROM task_builtin_mcp_preferences pref WHERE pref.task_id=task_builtin_mcp_applications.task_id AND pref.revision=task_builtin_mcp_applications.target_revision AND pref.workspace_id=task_builtin_mcp_applications.workspace_id) AND COALESCE((SELECT revision FROM workspace_builtin_mcp_policies WHERE workspace_id=task_builtin_mcp_applications.workspace_id),0)=policy_revision`), time.Now().UTC(), sessionID, version, hash, current.BuiltinPackages.TaskID, current.BuiltinPackages.WorkspaceID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return false, err
	}
	return true, tx.Commit()
}

func builtinSupersedeProfile(metadata map[string]any) (*p.Context, error) {
	if closed, exists := metadata[p.ProviderMCPEffectsClosedMetadataKey]; exists && closed != false {
		return nil, nil
	}
	current, err := p.ParseBuiltinBindingMetadata(metadata[p.BuiltinBindingMetadataKey])
	if value, ok := metadata[p.ProviderMCPBindingMetadataKey]; ok {
		_, current, err = p.ParseProviderMCPBinding(value)
	}
	if err != nil || current == nil {
		return nil, err
	}
	return current, nil
}
