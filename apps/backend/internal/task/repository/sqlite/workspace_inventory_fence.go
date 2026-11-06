package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
)

type inventoryFenceReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (r *Repository) ReadWorkspaceInventoryRepairFence(ctx context.Context, taskID, sessionID string) (*models.WorkspaceInventoryRepairFence, error) {
	return r.readInventoryFence(ctx, r.db, taskID, sessionID, "")
}

func (r *Repository) readInventoryFence(ctx context.Context, reader inventoryFenceReader, taskID, sessionID, suffix string) (*models.WorkspaceInventoryRepairFence, error) {
	fence := &models.WorkspaceInventoryRepairFence{}
	query := `SELECT t.updated_at, COALESCE(t.workflow_step_id, ''), s.updated_at, s.state
        FROM tasks t JOIN task_sessions s ON s.task_id = t.id WHERE t.id = ? AND s.id = ?` + suffix
	if err := reader.QueryRowContext(ctx, r.db.Rebind(query), taskID, sessionID).Scan(
		&fence.TaskUpdatedAt, &fence.WorkflowStepID, &fence.SessionUpdatedAt, &fence.SessionState); err != nil {
		return nil, err
	}
	err := reader.QueryRowContext(ctx, r.db.Rebind(`SELECT route_generation, state, updated_at
        FROM dynamic_route_states WHERE session_id = ?`+suffix), sessionID).Scan(&fence.RouteGeneration, &fence.RouteState, &fence.RouteUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return fence, nil
	}
	if err != nil {
		return nil, err
	}
	fence.RouteKnown = true
	return fence, nil
}

func (r *Repository) validateInventoryFenceTx(ctx context.Context, tx *sqlx.Tx, repair *models.WorkspaceInventoryRepair) error {
	if repair.Preservation.Fence == nil {
		return nil
	}
	actual, err := r.readInventoryFence(ctx, tx, repair.TaskID, repair.SessionID, r.workspaceInventorySourceLockSuffix())
	if err != nil || !repair.Preservation.Fence.Equal(actual) {
		return models.ErrWorkspaceInventoryRecoveryConflict
	}
	return nil
}
