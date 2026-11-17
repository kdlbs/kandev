package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdateProviderMCPRuntimeProfile changes context only for the accepted scope.
// A delayed update cannot overwrite another provider or the initial binding.
func (r *Repository) UpdateProviderMCPRuntimeProfile(ctx context.Context, sessionID, executionID, bindingID string, value any) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), sessionID); err != nil {
		return err
	}
	if err := r.ensureExecutorRunningAvailableTx(ctx, tx, sessionID); err != nil {
		return err
	}
	metadata, binding, err := r.providerBindingMetadataTx(ctx, tx, sessionID, executionID, bindingID)
	if err != nil {
		return err
	}
	if !providerContextPending(value) {
		if _, err := p.ProviderRuntimeProfile(binding, value, true); err != nil {
			return err
		}
	}
	metadata[p.ProviderMCPRuntimeProfileMetadataKey] = value
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE executors_running SET metadata=?,updated_at=? WHERE session_id=? AND agent_execution_id=?`), string(encoded), time.Now().UTC(), sessionID, executionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return models.ErrExecutionRotated
	}
	return tx.Commit()
}

func (r *Repository) providerBindingMetadataTx(ctx context.Context, tx *sqlx.Tx, sessionID, executionID, bindingID string) (map[string]any, p.ProviderMCPBinding, error) {
	var raw, taskID, workspaceID string
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(er.metadata,'{}'),t.id,t.workspace_id FROM executors_running er
 JOIN task_sessions s ON s.id=er.session_id AND s.task_id=er.task_id JOIN tasks t ON t.id=s.task_id
 WHERE er.session_id=? AND er.agent_execution_id=? AND t.archived_at IS NULL`), sessionID, executionID).Scan(&raw, &taskID, &workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, p.ProviderMCPBinding{}, models.ErrExecutionRotated
	}
	if err != nil {
		return nil, p.ProviderMCPBinding{}, err
	}
	metadata := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, p.ProviderMCPBinding{}, err
	}
	binding, profile, err := p.ParseProviderMCPBinding(metadata[p.ProviderMCPBindingMetadataKey])
	if err != nil {
		return nil, binding, err
	}
	if binding.Scope.BindingID != bindingID || binding.Scope.ExecutionID != executionID || profile.BuiltinPackages.TaskID != taskID || profile.BuiltinPackages.WorkspaceID != workspaceID {
		return nil, binding, models.ErrExecutionRotated
	}
	return metadata, binding, nil
}

func providerContextPending(value any) bool {
	pending, ok := value.(map[string]any)
	return ok && len(pending) == 1 && pending["update_incomplete"] == true
}
