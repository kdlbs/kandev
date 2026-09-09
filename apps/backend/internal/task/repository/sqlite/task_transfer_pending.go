package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) resolveTransferPendingState(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	placement *transferTaskPlacement,
	inventory transferRelationInventory,
) (string, map[string]string, error) {
	mapped := make(map[string]string)
	mapStep := func(sourceStepID string) (string, error) {
		if sourceStepID == "" {
			return "", nil
		}
		if destination, ok := mapped[sourceStepID]; ok {
			return destination, nil
		}
		var sourceName string
		if err := tx.GetContext(ctx, &sourceName, r.db.Rebind(
			`SELECT name FROM workflow_steps WHERE id = ? AND workflow_id = ?`),
			sourceStepID, command.ExpectedSourceWorkflowID); err != nil {
			return "", taskTransferConflict(err, "pending lane unavailable")
		}
		var destinations []string
		if err := tx.SelectContext(ctx, &destinations, r.db.Rebind(
			`SELECT id FROM workflow_steps WHERE workflow_id = ? AND name = ? ORDER BY id`),
			command.DestinationWorkflowID, sourceName); err != nil {
			return "", err
		}
		if len(destinations) != 1 {
			return "", fmt.Errorf("%w: pending lane mapping is not unique", repoerrors.ErrTaskTransferConflict)
		}
		equivalent, _, err := r.transferStepsEquivalent(ctx, tx, sourceStepID, destinations[0])
		if err != nil {
			return "", err
		}
		if !equivalent {
			return "", fmt.Errorf("%w: pending lane changes task semantics", repoerrors.ErrTaskTransferConflict)
		}
		mapped[sourceStepID] = destinations[0]
		return destinations[0], nil
	}
	queuedStepID, err := mapStep(placement.QueuedStepID)
	if err != nil {
		return "", nil, err
	}
	if inventory.pendingMoves {
		var rows []struct {
			WorkflowID string `db:"workflow_id"`
			StepID     string `db:"workflow_step_id"`
		}
		if err := tx.SelectContext(ctx, &rows, r.db.Rebind(
			`SELECT workflow_id, workflow_step_id FROM pending_moves WHERE task_id = ?`), command.TaskID); err != nil {
			return "", nil, err
		}
		for _, row := range rows {
			if row.WorkflowID != command.ExpectedSourceWorkflowID {
				return "", nil, fmt.Errorf("%w: pending move source changed", repoerrors.ErrTaskTransferConflict)
			}
			if _, err := mapStep(row.StepID); err != nil {
				return "", nil, err
			}
		}
	}
	if err := r.collectTransferRelationStepMappings(ctx, tx, command, inventory, mapStep); err != nil {
		return "", nil, err
	}
	return queuedStepID, mapped, nil
}

func (r *Repository) collectTransferRelationStepMappings(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	inventory transferRelationInventory,
	mapStep func(string) (string, error),
) error {
	for _, relation := range []struct {
		name    string
		present bool
	}{{"workflow_step_participants", inventory.participants}, {"workflow_step_decisions", inventory.decisions}} {
		if !relation.present {
			continue
		}
		var stepIDs []string
		query := fmt.Sprintf(`SELECT DISTINCT step_id FROM %s WHERE task_id = ?`, relation.name)
		if err := tx.SelectContext(ctx, &stepIDs, r.db.Rebind(query), command.TaskID); err != nil {
			return err
		}
		for _, stepID := range stepIDs {
			if _, err := mapStep(stepID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Repository) updateTaskTransferRelations(
	ctx context.Context,
	tx *sqlx.Tx,
	command models.TaskTransferCommand,
	stepIDs, agentProfileMappings map[string]string,
	projections []transferWorkspaceProjection,
	inventory transferRelationInventory,
) error {
	for _, projection := range projections {
		query := fmt.Sprintf(`UPDATE %s SET workspace_id = ? WHERE %s = ?`, projection.table, projection.taskColumn)
		if _, err := tx.ExecContext(ctx, r.db.Rebind(query), command.DestinationWorkspaceID, command.TaskID); err != nil {
			return err
		}
	}
	for sourceStepID, destinationStepID := range stepIDs {
		if inventory.pendingMoves {
			if _, err := tx.ExecContext(ctx, r.db.Rebind(`
				UPDATE pending_moves SET workflow_id = ?, workflow_step_id = ?
				WHERE task_id = ? AND workflow_id = ? AND workflow_step_id = ?`),
				command.DestinationWorkflowID, destinationStepID, command.TaskID,
				command.ExpectedSourceWorkflowID, sourceStepID); err != nil {
				return err
			}
		}
		if inventory.participants {
			if _, err := tx.ExecContext(ctx, r.db.Rebind(`
				UPDATE workflow_step_participants SET step_id = ? WHERE task_id = ? AND step_id = ?`),
				destinationStepID, command.TaskID, sourceStepID); err != nil {
				return err
			}
		}
		if inventory.decisions {
			if _, err := tx.ExecContext(ctx, r.db.Rebind(`
				UPDATE workflow_step_decisions SET step_id = ? WHERE task_id = ? AND step_id = ?`),
				destinationStepID, command.TaskID, sourceStepID); err != nil {
				return err
			}
		}
	}
	for sourceProfileID, destinationProfileID := range agentProfileMappings {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE workflow_step_participants SET agent_profile_id = ?
			WHERE task_id = ? AND agent_profile_id = ?`),
			destinationProfileID, command.TaskID, sourceProfileID); err != nil {
			return err
		}
	}
	return nil
}
