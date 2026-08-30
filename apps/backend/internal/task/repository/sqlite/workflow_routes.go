package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/workflow/routing"
)

// GetWorkflowRouteOperation returns the immutable stored request/outcome for
// an operation key. Producers use this before any queue, prompt, or lane
// mutation so exact retries and key collisions are settled from durable state.
func (r *Repository) GetWorkflowRouteOperation(
	ctx context.Context,
	operationID string,
) (routing.Operation, bool, error) {
	var operation routing.Operation
	var producer, outcome string
	var sessionID, turnID, actorID, supersedesID, effectID sql.NullString
	var transitionID sql.NullInt64
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, workspace_id, producer, expected_step_id,
			observed_step_id, target_step_id, session_id, turn_id,
			actor_kind, actor_id, external_cause, external_cause_id,
			outcome, supersedes_id, transition_id, effect_id
		FROM workflow_route_operations WHERE id = ?
	`), operationID).Scan(
		&operation.ID, &operation.TaskID, &operation.WorkspaceID, &producer,
		&operation.ExpectedStepID, &operation.ObservedStepID, &operation.TargetStepID,
		&sessionID, &turnID, &operation.ActorKind, &actorID,
		&operation.ExternalCause, &operation.ExternalCauseID, &outcome,
		&supersedesID, &transitionID, &effectID,
	)
	if err == sql.ErrNoRows {
		return routing.Operation{}, false, nil
	}
	if err != nil {
		return routing.Operation{}, false, fmt.Errorf("read workflow route operation: %w", err)
	}
	operation.Producer = routing.Producer(producer)
	operation.Outcome = routing.Outcome(outcome)
	operation.SessionID = sessionID.String
	operation.TurnID = turnID.String
	operation.ActorID = actorID.String
	operation.SupersedesID = supersedesID.String
	operation.TransitionID = transitionID.Int64
	operation.EffectID = effectID.String
	return operation, true, nil
}

// GetWorkflowRouteEffect returns the exact destination-entry allocation linked
// by a committed route operation.
func (r *Repository) GetWorkflowRouteEffect(
	ctx context.Context,
	effectID string,
) (routing.Effect, bool, error) {
	var effect routing.Effect
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, operation_id, task_id, transition_id, target_step_id, status, claim_token
		FROM workflow_route_effects WHERE id = ?
	`), effectID).Scan(
		&effect.ID, &effect.OperationID, &effect.TaskID, &effect.TransitionID,
		&effect.TargetStepID, &effect.Status, &effect.ClaimToken,
	)
	if err == sql.ErrNoRows {
		return routing.Effect{}, false, nil
	}
	if err != nil {
		return routing.Effect{}, false, fmt.Errorf("read workflow route effect: %w", err)
	}
	return effect, true, nil
}

// GetWorkflowRouteEffectByTransition finds the one route-owned entry effect
// for a committed task transition. Non-route transitions deliberately return
// no effect.
func (r *Repository) GetWorkflowRouteEffectByTransition(
	ctx context.Context, taskID string, transitionID int64,
) (routing.Effect, bool, error) {
	var effect routing.Effect
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, operation_id, task_id, transition_id, target_step_id, status, claim_token
		FROM workflow_route_effects WHERE task_id = ? AND transition_id = ?
	`), taskID, transitionID).Scan(
		&effect.ID, &effect.OperationID, &effect.TaskID, &effect.TransitionID,
		&effect.TargetStepID, &effect.Status, &effect.ClaimToken,
	)
	if err == sql.ErrNoRows {
		return routing.Effect{}, false, nil
	}
	if err != nil {
		return routing.Effect{}, false, fmt.Errorf("read workflow route effect by transition: %w", err)
	}
	return effect, true, nil
}

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
		claim_token TEXT NOT NULL DEFAULT '',
		claimed_at TIMESTAMP,
		completed_at TIMESTAMP,
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

// ClaimWorkflowRouteEffect atomically acquires an effect for delivery. A
// caller may reclaim a lease after its owner died before completion; completed
// effects are absorbing. The token must be supplied to Complete so a late
// worker cannot complete a lease reclaimed by crash recovery.
func (r *Repository) ClaimWorkflowRouteEffect(ctx context.Context, effectID, token string, now time.Time, lease time.Duration) (bool, error) {
	if effectID == "" || token == "" {
		return false, nil
	}
	staleBefore := now.Add(-lease)
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_route_effects
		SET status = 'claimed', claim_token = ?, claimed_at = ?, updated_at = ?
		WHERE id = ? AND (
			status = 'pending' OR (status = 'claimed' AND claimed_at <= ?)
		)
	`), token, now, now, effectID, staleBefore)
	if err != nil {
		return false, fmt.Errorf("claim workflow route effect: %w", err)
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read workflow route effect claim: %w", err)
	}
	return claimed == 1, nil
}

// CompleteWorkflowRouteEffect records successful delivery for the exact claim
// token. It is idempotent for a retried successful completion and rejects a
// stale claimant after recovery has reclaimed the effect.
func (r *Repository) CompleteWorkflowRouteEffect(ctx context.Context, effectID, token string, now time.Time) (bool, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_route_effects
		SET status = 'completed', completed_at = ?, updated_at = ?
		WHERE id = ? AND status = 'claimed' AND claim_token = ?
	`), now, now, effectID, token)
	if err != nil {
		return false, fmt.Errorf("complete workflow route effect: %w", err)
	}
	completed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read workflow route effect completion: %w", err)
	}
	return completed == 1, nil
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
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
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
		WHERE workflow_route_operations.task_id = excluded.task_id
			AND workflow_route_operations.workspace_id = excluded.workspace_id
			AND workflow_route_operations.producer = excluded.producer
			AND workflow_route_operations.expected_step_id = excluded.expected_step_id
			AND workflow_route_operations.target_step_id = excluded.target_step_id
			AND COALESCE(workflow_route_operations.session_id, '') = COALESCE(excluded.session_id, '')
			AND COALESCE(workflow_route_operations.turn_id, '') = COALESCE(excluded.turn_id, '')
			AND workflow_route_operations.actor_kind = excluded.actor_kind
			AND COALESCE(workflow_route_operations.actor_id, '') = COALESCE(excluded.actor_id, '')
			AND workflow_route_operations.external_cause = excluded.external_cause
			AND workflow_route_operations.external_cause_id = excluded.external_cause_id
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
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read workflow route operation result: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s", routing.ErrOperationIdentityConflict, operation.ID)
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
