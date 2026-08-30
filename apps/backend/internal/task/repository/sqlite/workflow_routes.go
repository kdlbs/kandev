package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/workflow/routing"
)

func (r *Repository) initWorkflowRoutingSchema() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS workflow_route_operations (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		workspace_id TEXT NOT NULL DEFAULT '',
		producer TEXT NOT NULL,
		expected_step_id TEXT NOT NULL DEFAULT '',
		observed_step_id TEXT NOT NULL DEFAULT '',
		target_step_id TEXT NOT NULL DEFAULT '',
		session_id TEXT,
		turn_id TEXT,
		actor_kind TEXT NOT NULL DEFAULT '',
		actor_id TEXT,
		external_cause TEXT NOT NULL DEFAULT '',
		external_cause_id TEXT NOT NULL DEFAULT '',
		outcome TEXT NOT NULL,
		supersedes_id TEXT,
		transition_id BIGINT,
		effect_id TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_route_operations_task
		ON workflow_route_operations(task_id, created_at, id);
	CREATE INDEX IF NOT EXISTS idx_workflow_route_operations_cause
		ON workflow_route_operations(external_cause, external_cause_id);

	CREATE TABLE IF NOT EXISTS workflow_route_effects (
		id TEXT PRIMARY KEY,
		operation_id TEXT NOT NULL UNIQUE REFERENCES workflow_route_operations(id) ON DELETE CASCADE,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		transition_id BIGINT NOT NULL,
		target_step_id TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_workflow_route_effects_task
		ON workflow_route_effects(task_id, created_at, id);
	`)
	if err != nil {
		return fmt.Errorf("init workflow routing schema: %w", err)
	}
	return nil
}

// RecordWorkflowRouteOperation records a non-transitioning outcome (queued,
// stale, conflict) or readback checkpoint. Physical transitions call the same
// core from recordStepTransition inside their owning transaction.
func (r *Repository) RecordWorkflowRouteOperation(ctx context.Context, operation routing.Operation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.recordWorkflowRouteOperationTx(ctx, tx, operation); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) recordWorkflowRouteOperationTx(
	ctx context.Context,
	tx stepTransitionTx,
	operation routing.Operation,
) error {
	if operation.ID == "" || operation.TaskID == "" {
		return nil
	}
	if operation.Outcome == "" {
		operation.Outcome = routing.OutcomePending
	}
	now := time.Now().UTC()
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_route_operations (
			id, task_id, workspace_id, producer, expected_step_id, observed_step_id,
			target_step_id, session_id, turn_id, actor_kind, actor_id,
			external_cause, external_cause_id, outcome, supersedes_id,
			transition_id, effect_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			observed_step_id = CASE WHEN workflow_route_operations.outcome = 'pending' THEN excluded.observed_step_id ELSE workflow_route_operations.observed_step_id END,
			target_step_id = CASE WHEN workflow_route_operations.outcome = 'pending' THEN excluded.target_step_id ELSE workflow_route_operations.target_step_id END,
			outcome = CASE WHEN workflow_route_operations.outcome = 'pending' THEN excluded.outcome ELSE workflow_route_operations.outcome END,
			transition_id = COALESCE(workflow_route_operations.transition_id, excluded.transition_id),
			effect_id = COALESCE(workflow_route_operations.effect_id, excluded.effect_id),
			updated_at = excluded.updated_at
	`),
		operation.ID, operation.TaskID, operation.WorkspaceID, string(operation.Producer),
		operation.ExpectedStepID, operation.ObservedStepID, operation.TargetStepID,
		nullableString(operation.SessionID), nullableString(operation.TurnID), operation.ActorKind,
		nullableString(operation.ActorID), operation.ExternalCause, operation.ExternalCauseID,
		string(operation.Outcome), nullableString(operation.SupersedesID), nullableInt64(operation.TransitionID),
		nullableString(operation.EffectID), now, now,
	)
	if err != nil {
		return fmt.Errorf("record workflow route operation: %w", err)
	}
	if operation.TransitionID == 0 || operation.EffectID == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_route_effects (
			id, operation_id, task_id, transition_id, target_step_id, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, 'pending', ?, ?)
		ON CONFLICT(operation_id) DO NOTHING
	`), operation.EffectID, operation.ID, operation.TaskID, operation.TransitionID, operation.TargetStepID, now, now); err != nil {
		return fmt.Errorf("record workflow route effect: %w", err)
	}
	return nil
}

func nullableInt64(value int64) interface{} {
	if value == 0 {
		return nil
	}
	return value
}
