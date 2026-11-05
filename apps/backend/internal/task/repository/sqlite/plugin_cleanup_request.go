package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordPluginExecutorCleanupRequested marks the envelope of one execution as
// owing a cleanup. It does not wait on the environment claim: the request is
// recorded precisely when another holder owns the claim, and the revision bump
// makes that holder's next compare-and-swap conflict so the cleanup wins. A
// request that is already recorded writes nothing.
func (r *Repository) RecordPluginExecutorCleanupRequested(ctx context.Context, sessionID, executionID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// The first read is unlocked: it only names the task and installation whose
	// locks must be taken before the execution row, the order a claim checkpoint
	// uses. The locked read below revalidates the execution.
	var taskID, metadataJSON string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id, metadata FROM executors_running WHERE session_id = ?`), sessionID).Scan(&taskID, &metadataJSON); errors.Is(err, sql.ErrNoRows) {
		return models.ErrExecutionRotated
	} else if err != nil {
		return err
	}
	unlocked := make(map[string]interface{})
	if metadataJSON != "" {
		if err := json.Unmarshal([]byte(metadataJSON), &unlocked); err != nil {
			return fmt.Errorf("decode existing executor inventory metadata: %w", err)
		}
	}
	if err := r.lockPluginEnvelopeWrite(ctx, tx, &models.ExecutorRunning{TaskID: taskID, Metadata: unlocked}); err != nil {
		return err
	}
	currentExecution, current, currentUpdatedAt, err := readPluginExecutorInventoryForUpdate(ctx, tx, sessionID)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && currentExecution != executionID) {
		return models.ErrExecutionRotated
	}
	if err != nil {
		return err
	}
	written, err := markPluginCleanupRequested(ctx, tx, r.db.Rebind, sessionID, executionID, current, currentUpdatedAt)
	if err != nil || !written {
		return err
	}
	return tx.Commit()
}

// markPluginCleanupRequested sets the marker and bumps the revision. written is
// false when the marker was already recorded.
func markPluginCleanupRequested(ctx context.Context, tx *sqlx.Tx, rebind func(string) string, sessionID, executionID string, current map[string]interface{}, currentUpdatedAt time.Time) (bool, error) {
	envelope, ok := current[pluginExecutorEnvelopeKey].(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("plugin executor envelope is missing: %w", models.ErrExecutionRotated)
	}
	if requested, _ := envelope["cleanup_requested"].(bool); requested {
		return false, nil
	}
	revision, err := pluginExecutorInventoryRevision(current)
	if err != nil {
		return false, err
	}
	envelope["cleanup_requested"] = true
	envelope["revision"] = revision + 1
	encoded, err := json.Marshal(current)
	if err != nil {
		return false, fmt.Errorf("encode plugin executor inventory metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, rebind(`
		UPDATE executors_running SET metadata = ?, updated_at = ?
		WHERE session_id = ? AND agent_execution_id = ? AND updated_at = ?
	`), string(encoded), time.Now().UTC(), sessionID, executionID, currentUpdatedAt)
	if err != nil {
		return false, err
	}
	if count, err := result.RowsAffected(); err != nil {
		return false, err
	} else if count != 1 {
		return false, models.ErrExecutionRotated
	}
	return true, nil
}
