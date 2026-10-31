package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordTaskTurnError writes a newer task turn-error pointer under the task
// row lock. A duplicate stamp and an older occurrence are no-ops.
func (r *Repository) RecordTaskTurnError(
	ctx context.Context,
	taskID string,
	value models.TaskTurnError,
) (bool, error) {
	value = models.NormalizeTaskTurnError(value)
	if !value.Valid() {
		return false, nil
	}
	return r.mutateTaskTurnError(ctx, taskID, func(metadata map[string]interface{}) (bool, error) {
		return models.SetTaskTurnError(metadata, value), nil
	})
}

// AcknowledgeTaskTurnErrorIfStamp retires exactly the observed record. A
// mismatched newer stamp never mutates the row.
func (r *Repository) AcknowledgeTaskTurnErrorIfStamp(
	ctx context.Context,
	taskID, stamp string,
	acknowledgedAt time.Time,
	reason string,
) (models.TurnErrorAcknowledgementResult, error) {
	result := models.TurnErrorAcknowledgementMissing
	_, err := r.mutateTaskTurnError(ctx, taskID, func(metadata map[string]interface{}) (bool, error) {
		current, ok := models.LoadTaskTurnError(metadata)
		if !ok {
			result = models.TurnErrorAcknowledgementMissing
			return false, nil
		}
		if current.StampValue != stamp {
			result = models.TurnErrorAcknowledgementStale
			return false, nil
		}
		if current.Acknowledged() {
			result = models.TurnErrorAcknowledgementNoOp
			return false, nil
		}
		if !models.AcknowledgeTaskTurnError(metadata, stamp, acknowledgedAt, reason) {
			result = models.TurnErrorAcknowledgementStale
			return false, nil
		}
		result = models.TurnErrorAcknowledgementStored
		return true, nil
	})
	return result, err
}

func (r *Repository) mutateTaskTurnError(
	ctx context.Context,
	taskID string,
	mutate func(metadata map[string]interface{}) (bool, error),
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockTaskTurnErrorRow(ctx, tx, r.db.DriverName(), taskID); err != nil {
		return false, err
	}
	raw, err := r.lockMetadataRow(ctx, tx, "tasks", "task", taskID)
	if err != nil {
		return false, err
	}
	metadata := make(map[string]interface{})
	if raw != "" && raw != jsonNull {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return false, fmt.Errorf("failed to parse task metadata: %w", err)
		}
	}
	changed, err := mutate(metadata)
	if err != nil {
		return false, err
	}
	if !changed {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	payload, err := json.Marshal(metadata[models.MetaKeyLastTurnError])
	if err != nil {
		return false, fmt.Errorf("failed to serialize task turn error: %w", err)
	}
	result, err := tx.ExecContext(
		ctx,
		r.db.Rebind(metadataKeyUpdateQuery("tasks", r.db.DriverName())),
		metadataKeyUpdateArgs(r.db.DriverName(), models.MetaKeyLastTurnError, string(payload), r.nowUTC(), taskID)...,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 0 {
		return false, fmt.Errorf("task not found: %s", taskID)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func lockTaskTurnErrorRow(ctx context.Context, tx *sqlx.Tx, driver, taskID string) error {
	if !dialect.IsPostgres(driver) {
		return nil
	}
	lockKey := "task-turn-error:" + taskID
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return fmt.Errorf("lock task turn error: %w", err)
	}
	return nil
}
