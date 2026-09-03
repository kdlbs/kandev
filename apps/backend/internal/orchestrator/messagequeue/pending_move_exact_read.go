package messagequeue

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

// ReadPendingMoveCensus is the read-only companion to ExactCancelPendingMove.
// A Coordinator rarely already holds the seven exact predicates a
// cancellation requires — this is how it discovers them safely, inside the
// same transactional authorization boundary, without resuming or messaging
// the target session and without mutating any row.
func (r *sqliteRepository) ReadPendingMoveCensus(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	taskID string,
	correlationID string,
) (*PendingMoveCensusResult, error) {
	tx, err := r.db.BeginTxx(ctx, pendingMoveCensusTxOptions())
	if err != nil {
		return nil, ErrPendingMoveReadFailed
	}
	defer func() { _ = tx.Rollback() }()

	match := ExactPendingMoveMatch{TaskID: taskID}
	authorized, err := r.exactCancelActorAuthorized(ctx, tx, actor, match)
	if err != nil {
		return nil, ErrPendingMoveReadFailed
	}
	if !authorized {
		return nil, r.commitPendingMoveCensusDenied(ctx, tx, actor, correlationID)
	}

	target, rowExists, err := r.readPendingMoveCensusTarget(ctx, tx, taskID)
	if err != nil {
		return nil, ErrPendingMoveReadFailed
	}
	if target == nil {
		if rowExists {
			return nil, r.commitPendingMoveCensusDenied(ctx, tx, actor, correlationID)
		}
		if err := r.appendPendingMoveCensusAudit(ctx, tx, actor, taskID, nil, correlationID,
			PendingMoveCensusOutcomeZeroRow); err != nil {
			return nil, ErrPendingMoveReadFailed
		}
		if err := tx.Commit(); err != nil {
			return nil, ErrPendingMoveReadFailed
		}
		return &PendingMoveCensusResult{
			Found:         false,
			CorrelationID: correlationID,
			ActorKind:     actor.Kind,
			ActorID:       actor.ID,
			TaskID:        taskID,
		}, nil
	}
	if err := r.appendPendingMoveCensusAudit(ctx, tx, actor, taskID, target, correlationID,
		PendingMoveCensusOutcomeFound); err != nil {
		return nil, ErrPendingMoveReadFailed
	}
	if err := tx.Commit(); err != nil {
		return nil, ErrPendingMoveReadFailed
	}
	return &PendingMoveCensusResult{
		Found:                 true,
		CorrelationID:         correlationID,
		ActorKind:             actor.Kind,
		ActorID:               actor.ID,
		PendingMoveID:         target.rowID,
		MoveID:                target.moveID,
		TaskID:                target.taskID,
		SessionID:             target.sessionID,
		WorkflowID:            target.workflowID,
		CurrentWorkflowStepID: target.currentStepID,
		TargetWorkflowStepID:  target.targetStepID,
		QueuedAt:              target.queuedAt,
	}, nil
}

func pendingMoveCensusTxOptions() *sql.TxOptions {
	// SQLite already snapshots reads for a transaction and ignores this option.
	// PostgreSQL otherwise defaults to READ COMMITTED and would take a new
	// snapshot for each authorization and target-relation statement.
	return &sql.TxOptions{Isolation: sql.LevelRepeatableRead}
}

func (r *sqliteRepository) readPendingMoveCensusTarget(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*exactCancelTarget, bool, error) {
	pendingMoveID, err := r.readLatestPendingMoveIDByTask(ctx, tx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	target, err := r.readExactCancelTargetByTask(ctx, tx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, true, nil
	}
	if err != nil {
		return nil, true, err
	}
	if target.rowID != pendingMoveID {
		return nil, true, nil
	}
	return target, true, nil
}

func (r *sqliteRepository) commitPendingMoveCensusDenied(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	correlationID string,
) error {
	if err := r.appendPendingMoveCensusAudit(ctx, tx, actor, "", nil, correlationID,
		PendingMoveCancellationOutcomeNotFoundOrChanged); err != nil {
		return ErrPendingMoveReadFailed
	}
	if err := tx.Commit(); err != nil {
		return ErrPendingMoveReadFailed
	}
	return ErrPendingMoveNotFoundOrChanged
}

func (r *sqliteRepository) readLatestPendingMoveIDByTask(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (string, error) {
	var pendingMoveID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id
		FROM pending_moves
		WHERE task_id = ?
		ORDER BY queued_at DESC
		LIMIT 1
	`), taskID).Scan(&pendingMoveID)
	if err != nil {
		return "", err
	}
	return pendingMoveID, nil
}

// readExactCancelTargetByTask is readExactCancelTarget's task-keyed sibling.
// A census caller supplies only the target task ID — the exact tuple is what
// it is trying to discover — so this joins the same relation chain keyed on
// pending.task_id instead of pending.session_id. Ordering by queued_at DESC
// makes the result deterministic in the operationally-unexpected case of more
// than one live session on the task each carrying an armed move.
func (r *sqliteRepository) readExactCancelTargetByTask(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
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
		WHERE pending.task_id = ?
		ORDER BY pending.queued_at DESC
		LIMIT 1
	`), taskID).Scan(
		&target.rowID, &target.moveID, &target.sessionID, &target.taskID,
		&target.workflowID, &target.targetStepID, &target.currentWorkflowID,
		&target.currentStepID, &target.workspaceID, &target.queuedAt,
	)
	if err != nil {
		return nil, err
	}
	return &target, nil
}

func (r *sqliteRepository) appendPendingMoveCensusAudit(
	ctx context.Context,
	tx *sqlx.Tx,
	actor PendingMoveCancellationActor,
	taskID string,
	target *exactCancelTarget,
	correlationID, outcome string,
) error {
	pendingMoveID, moveID, sessionID, workflowID, currentStepID, targetStepID := "", "", "", "", "", ""
	if target != nil {
		pendingMoveID, moveID, sessionID = target.rowID, target.moveID, target.sessionID
		workflowID, currentStepID, targetStepID = target.workflowID, target.currentStepID, target.targetStepID
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO pending_move_cancellation_audit (
			id, correlation_id, occurred_at, actor_kind, actor_id,
			caller_task_id, caller_session_id, caller_execution_id, workspace_id,
			pending_move_id, move_id, task_id, session_id, workflow_id,
			prior_current_workflow_step_id, prior_target_workflow_step_id, outcome, changed, action
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
	`), uuid.New().String(), correlationID, time.Now().UTC(), actor.Kind, actor.ID,
		actor.CallerTaskID, actor.CallerSessionID, actor.CallerExecutionID, actor.WorkspaceID,
		pendingMoveID, moveID, taskID, sessionID, workflowID,
		currentStepID, targetStepID, outcome, pendingMoveAuditActionRead)
	return err
}

func (r *sqliteRepository) AuditInvalidPendingMoveCensus(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	correlationID string,
	identifiersPresent, identifiersCanonical bool,
) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO pending_move_cancellation_audit (
			id, correlation_id, occurred_at, actor_kind, actor_id,
			caller_task_id, caller_session_id, caller_execution_id, workspace_id,
			outcome, changed, identifiers_present, identifiers_canonical, action
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
	`), uuid.New().String(), correlationID, time.Now().UTC(), actor.Kind, actor.ID,
		actor.CallerTaskID, actor.CallerSessionID, actor.CallerExecutionID, actor.WorkspaceID,
		PendingMoveCancellationOutcomeInvalidArgument, boolToInt(identifiersPresent), boolToInt(identifiersCanonical),
		pendingMoveAuditActionRead)
	return err
}

// ReadPendingMoveCensus fails closed for the relation-free in-memory repository.
func (r *memoryRepository) ReadPendingMoveCensus(
	_ context.Context,
	_ PendingMoveCancellationActor,
	_ string,
	_ string,
) (*PendingMoveCensusResult, error) {
	// The census shares cancellation's authorization boundary, which the
	// relation-free in-memory queue cannot establish.
	return nil, ErrPendingMoveNotFoundOrChanged
}

func (r *memoryRepository) AuditInvalidPendingMoveCensus(
	context.Context,
	PendingMoveCancellationActor,
	string,
	bool,
	bool,
) error {
	return nil
}

// ReadPendingMove validates public identifiers before repository access,
// delegates the authorized read to the repository, then emits a redacted
// structured operational mirror of the durable audit result. It never
// mutates pending-move state.
func (s *Service) ReadPendingMove(
	ctx context.Context,
	actor PendingMoveCancellationActor,
	taskID string,
	correlationID string,
) (*PendingMoveCensusResult, error) {
	if correlationID == "" {
		correlationID = uuid.New().String()
	}
	present, canonical := pendingMoveCensusIdentifierValid(taskID)
	if !present || !canonical {
		if err := s.repo.AuditInvalidPendingMoveCensus(ctx, actor, correlationID, present, canonical); err != nil {
			return nil, ErrPendingMoveReadFailed
		}
		s.logger.Warn("pending move census rejected",
			zap.String("correlation_id", correlationID),
			zap.String("actor_kind", actor.Kind),
			zap.String("outcome", PendingMoveCancellationOutcomeInvalidArgument))
		return nil, ErrPendingMoveInvalidArgument
	}
	result, err := s.repo.ReadPendingMoveCensus(ctx, actor, taskID, correlationID)
	if err != nil {
		s.logger.Warn("pending move census denied",
			zap.String("correlation_id", correlationID),
			zap.String("actor_kind", actor.Kind),
			zap.String("outcome", pendingMoveCancellationLogOutcome(err)))
		return nil, err
	}
	outcome := PendingMoveCensusOutcomeZeroRow
	if result.Found {
		outcome = PendingMoveCensusOutcomeFound
	}
	s.logger.Info("pending move census read",
		zap.String("correlation_id", correlationID),
		zap.String("actor_kind", result.ActorKind),
		zap.String("actor_id", result.ActorID),
		zap.String("task_id", result.TaskID),
		zap.Bool("found", result.Found),
		zap.String("pending_move_id", result.PendingMoveID),
		zap.String("session_id", result.SessionID),
		zap.String("workflow_id", result.WorkflowID),
		zap.String("current_workflow_step_id", result.CurrentWorkflowStepID),
		zap.String("target_workflow_step_id", result.TargetWorkflowStepID),
		zap.String("outcome", outcome),
		zap.Time("occurred_at", time.Now().UTC()))
	return result, nil
}

func pendingMoveCensusIdentifierValid(taskID string) (bool, bool) {
	if taskID == "" {
		return false, false
	}
	parsed, err := uuid.Parse(taskID)
	if err != nil || parsed.String() != taskID {
		return true, false
	}
	return true, true
}
