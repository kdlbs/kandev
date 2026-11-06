package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdateBuiltinMCPBinding changes only the binding of the expected execution.
// It cannot replace a successor or overwrite a concurrent resume-token update.
func (r *Repository) UpdateBuiltinMCPBinding(ctx context.Context, sessionID, executionID string, value any) error {
	if sessionID == "" || executionID == "" {
		return errors.New("session and execution are required for built-in MCP binding")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
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
	query := `UPDATE executors_running SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?, json(?)), updated_at = ? WHERE session_id = ? AND agent_execution_id = ?`
	path := `$."` + p.BuiltinBindingMetadataKey + `"`
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `UPDATE executors_running SET metadata = jsonb_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END, ARRAY[?]::text[], ?::jsonb, true)::text, updated_at = ? WHERE session_id = ? AND agent_execution_id = ?`
		path = p.BuiltinBindingMetadataKey
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), path, string(payload), time.Now().UTC(), sessionID, executionID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return models.ErrExecutionRotated
	}
	return tx.Commit()
}

// retainBuiltinBindingTx runs after the session write lock. Status snapshots
// may lag a control update, so only the narrow binding writer can change the
// binding of an existing execution, including an explicit unknown value.
func (r *Repository) retainBuiltinBindingTx(ctx context.Context, tx *sqlx.Tx, running *models.ExecutorRunning, candidate string) (string, error) {
	var executionID, stored string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT agent_execution_id, COALESCE(metadata,'{}') FROM executors_running WHERE session_id = ?`), running.SessionID).Scan(&executionID, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return candidate, nil
	}
	if err != nil {
		return "", err
	}
	if executionID != running.AgentExecutionID {
		return candidate, nil
	}
	prior := map[string]json.RawMessage{}
	if stored != "" {
		if err := json.Unmarshal([]byte(stored), &prior); err != nil {
			return "", err
		}
	}
	next := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(candidate), &next); err != nil {
		return "", err
	}
	delete(next, p.BuiltinBindingMetadataKey)
	if value, exists := prior[p.BuiltinBindingMetadataKey]; exists {
		if next == nil {
			next = make(map[string]json.RawMessage)
		}
		next[p.BuiltinBindingMetadataKey] = value
	}
	encoded, err := json.Marshal(next)
	return string(encoded), err
}
