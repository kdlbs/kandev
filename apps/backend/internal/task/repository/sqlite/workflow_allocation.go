package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// admitWorkflowAllocationTx fences profile-entry allocation before any seed,
// workspace or session mutation. An existing allocation is never evidence that
// its attach owner has stopped, including when its destination is terminal.
func (r *Repository) admitWorkflowAllocationTx(ctx context.Context, tx *sqlx.Tx, taskID string, route *models.WorkflowSessionRoute) error {
	if route == nil || route.TargetKind != "profile" {
		return nil
	}
	if route.OperationID == "" || route.EntryIdentity == "" {
		return fmt.Errorf("workflow allocation requires an accepted entry identity")
	}
	if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		return err
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "workflow-allocation:"+taskID); err != nil {
			return fmt.Errorf("lock workflow allocation: %w", err)
		}
	}
	metadata, err := r.workflowAllocationOwnerTx(ctx, tx, taskID, route)
	if err != nil {
		return err
	}
	if _, exists := metadata[models.MetaKeyWorkflowSessionRoute]; exists {
		previous, valid := models.LoadWorkflowSessionRoute(metadata)
		if !valid {
			return fmt.Errorf("recorded workflow allocation is invalid")
		}
		if previous.OperationID == route.OperationID || previous.EntryIdentity == route.EntryIdentity {
			return repoerrors.ErrWorkflowAllocationRecorded
		}
	}
	return nil
}

func (r *Repository) workflowAllocationOwnerTx(ctx context.Context, tx *sqlx.Tx, taskID string, route *models.WorkflowSessionRoute) (map[string]interface{}, error) {
	var stepID, raw string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workflow_step_id, ''), COALESCE(metadata, '{}') FROM tasks WHERE id = ?`), taskID).Scan(&stepID, &raw); err != nil {
		return nil, fmt.Errorf("read workflow allocation owner: %w", err)
	}
	if stepID != route.DestinationStepID {
		return nil, repoerrors.ErrWorkflowAllocationStaleEntry
	}
	if strings.HasPrefix(route.EntryIdentity, "entry:") {
		var entryID int64
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(MAX(id), 0) FROM task_step_transitions WHERE task_id = ?`), taskID).Scan(&entryID); err != nil {
			return nil, fmt.Errorf("read workflow allocation entry: %w", err)
		}
		if route.EntryIdentity != fmt.Sprintf("entry:%020d", entryID) {
			return nil, repoerrors.ErrWorkflowAllocationStaleEntry
		}
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode workflow allocation owner: %w", err)
	}
	return metadata, nil
}
