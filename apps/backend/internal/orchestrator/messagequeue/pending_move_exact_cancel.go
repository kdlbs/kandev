package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type exactCancelTarget struct {
	rowID             string
	moveID            string
	sessionID         string
	taskID            string
	workflowID        string
	targetStepID      string
	currentWorkflowID string
	currentStepID     string
	workspaceID       string
	queuedAt          time.Time
}

func (r *sqliteRepository) ExactCancelPendingMove(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
	correlationID string,
) (*PendingMoveCancellationResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	defer func() { _ = tx.Rollback() }()

	authorized, err := r.exactCancelActorAuthorized(ctx, tx, actor, match)
	if err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	if !authorized {
		return nil, r.commitExactCancelMiss(ctx, tx, actor, nil, correlationID)
	}
	locked, err := r.lockExistingExactCancelSession(ctx, tx, match.SessionID)
	if err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	if !locked {
		return nil, r.commitExactCancelMiss(ctx, tx, actor, nil, correlationID)
	}

	target, err := r.readExactCancelTarget(ctx, tx, match.SessionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPendingMoveCancelFailed
	}
	if errors.Is(err, sql.ErrNoRows) || !exactCancelTargetMatches(target, actor, match) {
		return nil, r.commitExactCancelMiss(ctx, tx, actor, target, correlationID)
	}

	deleted, err := r.deleteExactCancelTarget(ctx, tx, actor, match)
	if err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	if !deleted {
		return nil, r.commitExactCancelMiss(ctx, tx, actor, target, correlationID)
	}
	if err := r.appendPendingMoveCancellationAudit(ctx, tx, actor, match, target,
		correlationID, PendingMoveCancellationOutcomeCancelled, true); err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrPendingMoveCancelFailed
	}
	return &PendingMoveCancellationResult{
		Cancelled:                  true,
		CorrelationID:              correlationID,
		ActorKind:                  actor.Kind,
		ActorID:                    actor.ID,
		PendingMoveID:              target.rowID,
		MoveID:                     target.moveID,
		TaskID:                     target.taskID,
		SessionID:                  target.sessionID,
		WorkflowID:                 target.workflowID,
		PriorCurrentWorkflowStepID: target.currentStepID,
		PriorTargetWorkflowStepID:  target.targetStepID,
		QueuedAt:                   target.queuedAt,
	}, nil
}

// lockExistingExactCancelSession takes the same durable row lock used by all
// pending-move mutations without creating an auxiliary row from an untrusted
// mismatched session ID. initSchema backfills lock rows for legacy pending
// moves, and every current Set/Take/Transfer path creates them on admission.
func (r *sqliteRepository) lockExistingExactCancelSession(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (bool, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queue_session_locks SET session_id = session_id WHERE session_id = ?
	`), sessionID)
	if err != nil {
		return false, fmt.Errorf("lock exact cancellation session: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read exact cancellation session lock count: %w", err)
	}
	return affected == 1, nil
}

func (r *sqliteRepository) commitExactCancelMiss(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	target *exactCancelTarget,
	correlationID string,
) error {
	match, target := safeExactCancelAuditTarget(actor, target)
	if err := r.appendPendingMoveCancellationAudit(ctx, tx, actor, match, target,
		correlationID, PendingMoveCancellationOutcomeNotFoundOrChanged, false); err != nil {
		return ErrPendingMoveCancelFailed
	}
	if err := tx.Commit(); err != nil {
		return ErrPendingMoveCancelFailed
	}
	return ErrPendingMoveNotFoundOrChanged
}

func safeExactCancelAuditTarget(
	actor PendingMoveCancellationActor,
	target *exactCancelTarget,
) (ExactPendingMoveMatch, *exactCancelTarget) {
	if target == nil || actor.WorkspaceID == "" || target.workspaceID != actor.WorkspaceID {
		return ExactPendingMoveMatch{}, nil
	}
	return ExactPendingMoveMatch{
		PendingMoveID:                 target.rowID,
		SessionID:                     target.sessionID,
		TaskID:                        target.taskID,
		MoveID:                        target.moveID,
		WorkflowID:                    target.workflowID,
		ExpectedCurrentWorkflowStepID: target.currentStepID,
		ExpectedTargetWorkflowStepID:  target.targetStepID,
	}, target
}

// deleteExactCancelTarget repeats the full authorization and relation tuple at
// the mutation statement. PostgreSQL READ COMMITTED transactions take a fresh
// snapshot per statement, so relying only on the earlier read could otherwise
// delete after a concurrent task move or Coordinator revocation committed.
func (r *sqliteRepository) deleteExactCancelTarget(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
) (bool, error) {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM pending_moves
		WHERE id = ? AND session_id = ? AND task_id = ? AND move_id = ?
			AND workflow_id = ? AND workflow_step_id = ?
			AND EXISTS (
				SELECT 1
				FROM tasks target
				JOIN task_sessions target_session
					ON target_session.id = pending_moves.session_id
					AND target_session.task_id = target.id
				JOIN workflows current_workflow
					ON current_workflow.id = target.workflow_id
					AND current_workflow.workspace_id = target.workspace_id
				JOIN workflow_steps current_step
					ON current_step.id = target.workflow_step_id
					AND current_step.workflow_id = current_workflow.id
				JOIN workflows target_workflow
					ON target_workflow.id = pending_moves.workflow_id
					AND target_workflow.workspace_id = target.workspace_id
				JOIN workflow_steps target_step
					ON target_step.id = pending_moves.workflow_step_id
					AND target_step.workflow_id = target_workflow.id
				WHERE target.id = pending_moves.task_id
					AND target.workspace_id = ? AND target.workflow_step_id = ?
			)
			AND EXISTS (
				SELECT 1
				FROM tasks caller
				JOIN workspaces workspace ON workspace.id = caller.workspace_id
				JOIN task_sessions caller_session
					ON caller_session.id = ? AND caller_session.task_id = caller.id
				JOIN executors_running execution
					ON execution.session_id = caller_session.id
					AND execution.task_id = caller.id
					AND execution.agent_execution_id = ?
				JOIN workspace_coordinator_grants grant
					ON grant.workspace_id = caller.workspace_id
					AND grant.coordinator_task_id = caller.id
				WHERE caller.id = ? AND caller.workspace_id = ? AND workspace.owner_id = ?
					AND caller_session.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
					AND execution.status IN ('starting', 'running', 'ready')
					AND EXISTS (
						WITH RECURSIVE target_ancestors(id, parent_id, depth) AS (
							SELECT id, parent_id, 0 FROM tasks
							WHERE id = ? AND workspace_id = caller.workspace_id
							UNION ALL
							SELECT parent.id, parent.parent_id, target_ancestors.depth + 1
							FROM tasks parent
							JOIN target_ancestors ON target_ancestors.parent_id = parent.id
							WHERE target_ancestors.depth < 64
								AND parent.workspace_id = caller.workspace_id
						)
						SELECT 1 FROM target_ancestors WHERE id = caller.id
					)
			)
	`), match.PendingMoveID, match.SessionID, match.TaskID, match.MoveID,
		match.WorkflowID, match.ExpectedTargetWorkflowStepID,
		actor.WorkspaceID, match.ExpectedCurrentWorkflowStepID,
		actor.CallerSessionID, actor.CallerExecutionID, actor.CallerTaskID, actor.WorkspaceID, actor.UserID,
		match.TaskID)
	if err != nil {
		return false, fmt.Errorf("delete exact pending move: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read exact pending move delete count: %w", err)
	}
	return affected == 1, nil
}

func (r *sqliteRepository) exactCancelActorAuthorized(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
) (bool, error) {
	if actor.Kind != pendingMoveActorKindCoordinator || actor.ID == "" || actor.UserID == "" || actor.WorkspaceID == "" ||
		actor.CallerTaskID == "" || actor.CallerSessionID == "" || actor.CallerExecutionID == "" ||
		actor.ID != actor.CallerTaskID || actor.CallerTaskID == match.TaskID || actor.CallerSessionID == match.SessionID {
		return false, nil
	}
	var one int
	err := tx.GetContext(ctx, &one, r.db.Rebind(`
		SELECT 1
		FROM tasks caller
		JOIN workspaces workspace ON workspace.id = caller.workspace_id
		JOIN task_sessions caller_session
			ON caller_session.id = ? AND caller_session.task_id = caller.id
		JOIN executors_running execution
			ON execution.session_id = caller_session.id
			AND execution.task_id = caller.id
			AND execution.agent_execution_id = ?
		JOIN workspace_coordinator_grants grant
			ON grant.workspace_id = caller.workspace_id
			AND grant.coordinator_task_id = caller.id
		WHERE caller.id = ? AND caller.workspace_id = ? AND workspace.owner_id = ?
			AND caller_session.state IN ('STARTING', 'RUNNING', 'WAITING_FOR_INPUT')
			AND execution.status IN ('starting', 'running', 'ready')
			AND EXISTS (
				WITH RECURSIVE target_ancestors(id, parent_id, depth) AS (
					SELECT id, parent_id, 0 FROM tasks
					WHERE id = ? AND workspace_id = caller.workspace_id
					UNION ALL
					SELECT parent.id, parent.parent_id, target_ancestors.depth + 1
					FROM tasks parent
					JOIN target_ancestors ON target_ancestors.parent_id = parent.id
					WHERE target_ancestors.depth < 64
						AND parent.workspace_id = caller.workspace_id
				)
				SELECT 1 FROM target_ancestors WHERE id = caller.id
			)
	`), actor.CallerSessionID, actor.CallerExecutionID, actor.CallerTaskID, actor.WorkspaceID, actor.UserID, match.TaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("authorize exact pending move cancellation: %w", err)
	}
	return true, nil
}

func (r *sqliteRepository) readExactCancelTarget(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID string,
) (*exactCancelTarget, error) {
	var target exactCancelTarget
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT pending.id, pending.move_id, pending.session_id, pending.task_id,
			pending.workflow_id, pending.workflow_step_id, task.workflow_id,
			task.workflow_step_id, task.workspace_id, pending.queued_at
		FROM pending_moves pending
		JOIN tasks task ON task.id = pending.task_id
		JOIN task_sessions target_session
			ON target_session.id = pending.session_id AND target_session.task_id = task.id
		JOIN workflows current_workflow
			ON current_workflow.id = task.workflow_id AND current_workflow.workspace_id = task.workspace_id
		JOIN workflow_steps current_step
			ON current_step.id = task.workflow_step_id AND current_step.workflow_id = current_workflow.id
		JOIN workflows target_workflow
			ON target_workflow.id = pending.workflow_id AND target_workflow.workspace_id = task.workspace_id
		JOIN workflow_steps target_step
			ON target_step.id = pending.workflow_step_id AND target_step.workflow_id = target_workflow.id
		WHERE pending.session_id = ?
	`), sessionID).Scan(
		&target.rowID, &target.moveID, &target.sessionID, &target.taskID,
		&target.workflowID, &target.targetStepID, &target.currentWorkflowID,
		&target.currentStepID, &target.workspaceID, &target.queuedAt,
	)
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func exactCancelTargetMatches(
	target *exactCancelTarget,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
) bool {
	return target != nil && target.workspaceID == actor.WorkspaceID &&
		target.rowID == match.PendingMoveID && target.sessionID == match.SessionID &&
		target.taskID == match.TaskID && target.moveID == match.MoveID &&
		target.workflowID == match.WorkflowID && target.currentStepID == match.ExpectedCurrentWorkflowStepID &&
		target.targetStepID == match.ExpectedTargetWorkflowStepID
}

func (r *sqliteRepository) appendPendingMoveCancellationAudit(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
	target *exactCancelTarget,
	correlationID, outcome string,
	changed bool,
) error {
	currentStepID, targetStepID := "", ""
	if target != nil {
		currentStepID = target.currentStepID
		targetStepID = target.targetStepID
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO pending_move_cancellation_audit (
			id, correlation_id, occurred_at, actor_kind, actor_id,
			caller_task_id, caller_session_id, caller_execution_id, workspace_id,
			pending_move_id, move_id, task_id, session_id, workflow_id,
			prior_current_workflow_step_id, prior_target_workflow_step_id, outcome, changed
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), uuid.New().String(), correlationID, time.Now().UTC(), actor.Kind, actor.ID,
		actor.CallerTaskID, actor.CallerSessionID, actor.CallerExecutionID, actor.WorkspaceID,
		match.PendingMoveID, match.MoveID, match.TaskID, match.SessionID, match.WorkflowID,
		currentStepID, targetStepID, outcome, boolToInt(changed))
	return err
}

func (r *sqliteRepository) AuditInvalidPendingMoveCancellation(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	correlationID string,
	identifiersPresent, identifiersCanonical bool,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO pending_move_cancellation_audit (
			id, correlation_id, occurred_at, actor_kind, actor_id,
			caller_task_id, caller_session_id, caller_execution_id, workspace_id,
			outcome, changed, identifiers_present, identifiers_canonical
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
	`), uuid.New().String(), correlationID, time.Now().UTC(), actor.Kind, actor.ID,
		actor.CallerTaskID, actor.CallerSessionID, actor.CallerExecutionID, actor.WorkspaceID,
		PendingMoveCancellationOutcomeInvalidArgument, boolToInt(identifiersPresent), boolToInt(identifiersCanonical))
	return err
}

func (r *memoryRepository) ExactCancelPendingMove(
	_ context.Context,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
	correlationID string,
) (*PendingMoveCancellationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.pendingMoves[match.SessionID]
	if actor.Kind != pendingMoveActorKindCoordinator || actor.CallerTaskID == match.TaskID || !ok ||
		stored.ID != match.PendingMoveID || stored.TaskID != match.TaskID || stored.MoveID != match.MoveID ||
		stored.WorkflowID != match.WorkflowID || stored.WorkflowStepID != match.ExpectedTargetWorkflowStepID {
		return nil, ErrPendingMoveNotFoundOrChanged
	}
	delete(r.pendingMoves, match.SessionID)
	return &PendingMoveCancellationResult{
		Cancelled:                  true,
		CorrelationID:              correlationID,
		ActorKind:                  actor.Kind,
		ActorID:                    actor.ID,
		PendingMoveID:              stored.ID,
		MoveID:                     stored.MoveID,
		TaskID:                     stored.TaskID,
		SessionID:                  match.SessionID,
		WorkflowID:                 stored.WorkflowID,
		PriorCurrentWorkflowStepID: match.ExpectedCurrentWorkflowStepID,
		PriorTargetWorkflowStepID:  stored.WorkflowStepID,
		QueuedAt:                   stored.QueuedAt,
	}, nil
}

func (r *memoryRepository) AuditInvalidPendingMoveCancellation(
	context.Context,
	PendingMoveCancellationActor,
	string,
	bool,
	bool,
) error {
	return nil
}

// ExactCancelPendingMove validates all public identifiers before repository
// access, delegates the atomic comparison/deletion, then emits a redacted
// structured operational mirror of the durable audit result.
func (s *Service) ExactCancelPendingMove(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
	correlationID string,
) (*PendingMoveCancellationResult, error) {
	if correlationID == "" {
		correlationID = uuid.New().String()
	}
	present, canonical := exactCancellationIdentifiersValid(actor, match)
	if !present || !canonical {
		if err := s.repo.AuditInvalidPendingMoveCancellation(ctx, actor, correlationID, present, canonical); err != nil {
			return nil, ErrPendingMoveCancelFailed
		}
		s.logger.Warn("pending move cancellation rejected",
			zap.String("correlation_id", correlationID),
			zap.String("actor_kind", actor.Kind),
			zap.String("outcome", PendingMoveCancellationOutcomeInvalidArgument))
		return nil, ErrPendingMoveInvalidArgument
	}
	result, err := s.repo.ExactCancelPendingMove(ctx, actor, match, correlationID)
	if err != nil {
		s.logger.Warn("pending move cancellation did not change state",
			zap.String("correlation_id", correlationID),
			zap.String("actor_kind", actor.Kind),
			zap.String("outcome", pendingMoveCancellationLogOutcome(err)))
		return nil, err
	}
	s.logger.Info("pending move cancelled",
		zap.String("correlation_id", correlationID),
		zap.String("actor_kind", result.ActorKind),
		zap.String("actor_id", result.ActorID),
		zap.String("pending_move_id", result.PendingMoveID),
		zap.String("move_id", result.MoveID),
		zap.String("task_id", result.TaskID),
		zap.String("session_id", result.SessionID),
		zap.String("workflow_id", result.WorkflowID),
		zap.String("prior_current_workflow_step_id", result.PriorCurrentWorkflowStepID),
		zap.String("prior_target_workflow_step_id", result.PriorTargetWorkflowStepID),
		zap.String("outcome", PendingMoveCancellationOutcomeCancelled),
		zap.Time("occurred_at", time.Now().UTC()))
	return result, nil
}

func exactCancellationIdentifiersValid(
	_ PendingMoveCancellationActor,
	match ExactPendingMoveMatch,
) (bool, bool) {
	values := []string{
		match.PendingMoveID, match.SessionID, match.TaskID, match.MoveID, match.WorkflowID,
		match.ExpectedCurrentWorkflowStepID, match.ExpectedTargetWorkflowStepID,
	}
	for _, value := range values {
		if value == "" {
			return false, false
		}
	}
	for _, value := range values {
		parsed, err := uuid.Parse(value)
		if err != nil || parsed.String() != value {
			return true, false
		}
	}
	return true, true
}

func pendingMoveCancellationLogOutcome(err error) string {
	if errors.Is(err, ErrPendingMoveNotFoundOrChanged) {
		return PendingMoveCancellationOutcomeNotFoundOrChanged
	}
	return "failed"
}
