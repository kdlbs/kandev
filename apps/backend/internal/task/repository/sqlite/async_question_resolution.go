package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) ResolveAsyncQuestion(
	ctx context.Context,
	id string,
	outcome models.AsyncQuestionOutcome,
	deliveries []*models.AsyncQuestionDelivery,
) (*models.AsyncQuestion, bool, error) {
	return r.ResolveAsyncQuestionWithWake(ctx, id, outcome, deliveries, nil)
}

func (r *Repository) ResolveAsyncQuestionWithWake(
	ctx context.Context,
	id string,
	outcome models.AsyncQuestionOutcome,
	deliveries []*models.AsyncQuestionDelivery,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, error) {
	return r.closeAsyncQuestion(ctx, id, "", "", outcome, deliveries, false, buildWake)
}

// WithdrawAsyncQuestion closes a pending request as withdrawn without
// recording an answer. A repeated withdrawal returns the canonical state.
func (r *Repository) WithdrawAsyncQuestion(
	ctx context.Context,
	id, authorSessionID, reason string,
	deliveries []*models.AsyncQuestionDelivery,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, []*models.AsyncQuestionWait, error) {
	question, won, released, err := r.closeAsyncQuestionWithWaits(ctx, id, authorSessionID, reason, models.AsyncQuestionOutcome{}, deliveries, true, buildWake)
	return question, won, released, err
}

func (r *Repository) closeAsyncQuestion(
	ctx context.Context,
	id, authorSessionID, withdrawReason string,
	outcome models.AsyncQuestionOutcome,
	deliveries []*models.AsyncQuestionDelivery,
	withdraw bool,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, error) {
	question, won, _, err := r.closeAsyncQuestionWithWaits(ctx, id, authorSessionID, withdrawReason, outcome, deliveries, withdraw, buildWake)
	return question, won, err
}

func (r *Repository) closeAsyncQuestionWithWaits(
	ctx context.Context,
	id, authorSessionID, withdrawReason string,
	outcome models.AsyncQuestionOutcome,
	deliveries []*models.AsyncQuestionDelivery,
	withdraw bool,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, bool, []*models.AsyncQuestionWait, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.acquireAsyncQuestionWriteLockTx(ctx, tx, id); err != nil {
		return nil, false, nil, err
	}

	current, err := r.lockAsyncQuestionTx(ctx, tx, id)
	if err != nil {
		return nil, false, nil, err
	}
	if current == nil {
		return nil, false, nil, ErrAsyncQuestionNotFound
	}
	if withdraw && current.SessionID != authorSessionID {
		return nil, false, nil, ErrAsyncQuestionNotAuthor
	}
	if current.Status.Terminal() {
		return current, false, nil, tx.Commit()
	}
	if err := r.authorizeAsyncQuestionCloseTx(ctx, tx, current, authorSessionID, withdraw, outcome, buildWake); err != nil {
		return nil, false, nil, err
	}

	now := r.nowUTC()
	_, _, won, released, err := r.finalizeAsyncQuestionTx(
		ctx, tx, current, outcome, withdraw, withdrawReason, now, deliveries, buildWake)
	if err != nil {
		return nil, false, nil, err
	}
	if !won {
		winner, readErr := r.getAsyncQuestionTx(ctx, tx, id)
		if readErr != nil {
			return nil, false, nil, readErr
		}
		return winner, false, nil, tx.Commit()
	}
	updated, err := r.getAsyncQuestionTx(ctx, tx, id)
	if err != nil {
		return nil, false, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, nil, err
	}
	return updated, true, released, nil
}

func (r *Repository) authorizeAsyncQuestionCloseTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current *models.AsyncQuestion,
	authorSessionID string,
	withdraw bool,
	outcome models.AsyncQuestionOutcome,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	action := asyncQuestionAuthorityResolveOperator
	if withdraw {
		action = asyncQuestionAuthorityWithdraw
	} else if outcome.ResponderKind == models.AsyncQuestionResponderParent {
		action = asyncQuestionAuthorityResolveParent
	}
	err := r.validateAsyncQuestionAuthorityTx(ctx, tx, current, action, outcome.ResponderID, buildWake)
	if errors.Is(err, models.ErrAsyncQuestionStale) {
		if commitErr := tx.Commit(); commitErr != nil {
			return commitErr
		}
	}
	return err
}

func (r *Repository) applyAsyncQuestionTerminalTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current *models.AsyncQuestion,
	outcome models.AsyncQuestionOutcome,
	withdraw bool,
	withdrawReason string,
	now time.Time,
) (models.AsyncQuestionStatus, int64, bool, error) {
	status := models.AsyncQuestionStatusAnswered
	if withdraw {
		status = models.AsyncQuestionStatusWithdrawn
	} else if outcome.Rejected {
		status = models.AsyncQuestionStatusRejected
	}
	nextVersion := current.Version + 1
	outcomeJSON := ""
	if !withdraw {
		outcome.Version = nextVersion
		encoded, err := json.Marshal(outcome)
		if err != nil {
			return status, nextVersion, false, err
		}
		outcomeJSON = string(encoded)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_questions
		   SET status = ?, outcome_json = ?, withdraw_reason = ?, version = ?,
		       updated_at = ?, resolved_at = ?
		 WHERE id = ? AND status = 'pending' AND version = ?`),
		string(status), outcomeJSON, withdrawReason, nextVersion, now, now, current.ID, current.Version)
	if err != nil {
		return status, nextVersion, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return status, nextVersion, false, err
	}
	return status, nextVersion, rows > 0, nil
}

type asyncQuestionAuthority int

const (
	asyncQuestionAuthorityResolveOperator asyncQuestionAuthority = iota
	asyncQuestionAuthorityResolveParent
	asyncQuestionAuthorityForward
	asyncQuestionAuthorityWithdraw
)

// acquireAsyncQuestionWriteLockTx takes SQLite's write lock before the first
// read-modify-write so a concurrent reparent/transfer cannot interleave
// between the authority read and the terminal transition.
func (r *Repository) acquireAsyncQuestionWriteLockTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(
		`UPDATE task_async_questions SET updated_at = updated_at WHERE id = ?`), id)
	return err
}

func (r *Repository) validateAsyncQuestionAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	action asyncQuestionAuthority,
	responderParentID string,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	if question == nil {
		return ErrAsyncQuestionNotFound
	}
	parentID, err := r.validateTaskAuthorityTx(ctx, tx, question, buildWake)
	if err != nil {
		return err
	}
	if err := r.validateSessionAuthorityTx(ctx, tx, question, action, buildWake); err != nil {
		return err
	}
	if question.Recipient != models.AsyncQuestionRecipientParent {
		return nil
	}
	return r.validateParentAuthorityTx(ctx, tx, question, action, responderParentID, parentID, buildWake)
}

func (r *Repository) validateTaskAuthorityTx(ctx context.Context, tx *sqlx.Tx, question *models.AsyncQuestion, buildWake models.AsyncQuestionWakeBuilder) (string, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		var locked string
		err := tx.QueryRowxContext(ctx, r.db.Rebind("SELECT id FROM tasks WHERE id = ?"+forUpdateClause), question.TaskID).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return "", r.markAsyncQuestionStaleTx(ctx, tx, question, "task_missing", buildWake)
		}
		if err != nil {
			return "", err
		}
	}
	var workspaceID, parentID string
	var archivedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind(
		`SELECT workspace_id, parent_id, archived_at FROM tasks WHERE id = ?`), question.TaskID,
	).Scan(&workspaceID, &parentID, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", r.markAsyncQuestionStaleTx(ctx, tx, question, "task_missing", buildWake)
	}
	if err != nil {
		return "", err
	}
	if archivedAt.Valid || workspaceID != question.WorkspaceID {
		return "", r.markAsyncQuestionStaleTx(ctx, tx, question, "task_ownership_changed", buildWake)
	}
	return parentID, nil
}

func (r *Repository) validateSessionAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	action asyncQuestionAuthority,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	var sessionTaskID, sessionState, sessionIncarnation string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(
		`SELECT task_id, state, queue_incarnation_id FROM task_sessions WHERE id = ?`), question.SessionID,
	).Scan(&sessionTaskID, &sessionState, &sessionIncarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "session_missing", buildWake)
	}
	if err != nil {
		return err
	}
	if sessionTaskID != question.TaskID {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "session_reassigned", buildWake)
	}
	if question.SessionIncarnationID != "" && sessionIncarnation != question.SessionIncarnationID {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "session_incarnation_changed", buildWake)
	}
	if action != asyncQuestionAuthorityWithdraw && terminalAsyncQuestionSessionState(sessionState) {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "session_terminal", buildWake)
	}
	return nil
}

func (r *Repository) validateParentAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	action asyncQuestionAuthority,
	responderParentID, parentID string,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	if parentID == "" || parentID != question.ParentTaskID {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "parent_changed", buildWake)
	}
	if err := r.validateParentTaskAuthorityTx(ctx, tx, question, parentID, buildWake); err != nil {
		return err
	}
	switch action {
	case asyncQuestionAuthorityResolveParent:
		if responderParentID != "" && responderParentID != parentID {
			return ErrAsyncQuestionForwardUnauthorized
		}
	case asyncQuestionAuthorityForward:
		return nil
	case asyncQuestionAuthorityResolveOperator:
		var forwarded string
		err := tx.QueryRowxContext(ctx, r.db.Rebind(
			`SELECT parent_task_id FROM task_async_question_forwards WHERE question_id = ?`), question.ID,
		).Scan(&forwarded)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAsyncQuestionForwardUnauthorized
		}
		if err != nil {
			return err
		}
		if forwarded != parentID {
			return r.markAsyncQuestionStaleTx(ctx, tx, question, "forward_parent_changed", buildWake)
		}
	}
	return nil
}

func (r *Repository) validateParentTaskAuthorityTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	parentID string,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		var locked string
		err := tx.QueryRowxContext(ctx, r.db.Rebind("SELECT id FROM tasks WHERE id = ?"+forUpdateClause), parentID).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return r.markAsyncQuestionStaleTx(ctx, tx, question, "parent_missing", buildWake)
		}
		if err != nil {
			return err
		}
	}
	var parentArchived sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind("SELECT archived_at FROM tasks WHERE id = ?"), parentID).Scan(&parentArchived)
	if errors.Is(err, sql.ErrNoRows) {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "parent_missing", buildWake)
	}
	if err != nil {
		return err
	}
	if parentArchived.Valid {
		return r.markAsyncQuestionStaleTx(ctx, tx, question, "parent_archived", buildWake)
	}
	return nil
}

func terminalAsyncQuestionSessionState(state string) bool {
	switch state {
	case sessionStateCompleted, sessionStateFailed, sessionStateCancelled:
		return true
	default:
		return false
	}
}

func (r *Repository) markAsyncQuestionStaleTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	reason string,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	if err := r.transitionAsyncQuestionStaleTx(ctx, tx, question, reason, buildWake); err != nil {
		return err
	}
	return models.ErrAsyncQuestionStale
}

func (r *Repository) transitionAsyncQuestionStaleTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	reason string,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	now := r.nowUTC()
	version := question.Version + 1
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_questions
		   SET status = 'stale', withdraw_reason = ?, version = ?, updated_at = ?, resolved_at = ?
		 WHERE id = ? AND status = 'pending' AND version = ?`),
		reason, version, now, now, question.ID, question.Version)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return nil
	}
	released, err := r.releaseAsyncQuestionWaitsTx(ctx, tx, question.ID, now, "stale:"+reason)
	if err != nil {
		return err
	}
	if question.Recipient == models.AsyncQuestionRecipientParent {
		if err := r.abandonParentQuestionIntentsTx(ctx, tx, question.ID, reason); err != nil {
			return err
		}
	}
	if buildWake == nil || len(released) == 0 {
		return nil
	}
	live, err := r.asyncQuestionAuthorLiveTx(ctx, tx, question)
	if err != nil || !live {
		return err
	}
	wakes := make([]*models.AsyncQuestionDelivery, 0, len(released))
	for _, wait := range released {
		if wait.TaskID != question.TaskID || wait.SessionID != question.SessionID {
			continue
		}
		if delivery := buildWake(question, wait, version, reason); delivery != nil {
			wakes = append(wakes, delivery)
		}
	}
	return r.insertAsyncQuestionDeliveriesTx(ctx, tx, question.ID, version, now, wakes)
}

func (r *Repository) asyncQuestionAuthorLiveTx(ctx context.Context, tx *sqlx.Tx, question *models.AsyncQuestion) (bool, error) {
	var taskID, state, incarnation string
	var taskFound sql.NullString
	var archivedAt sql.NullTime
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT session.task_id, session.state, session.queue_incarnation_id, task.id, task.archived_at
		  FROM task_sessions AS session
		  LEFT JOIN tasks AS task ON task.id = session.task_id
		 WHERE session.id = ?`), question.SessionID,
	).Scan(&taskID, &state, &incarnation, &taskFound, &archivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if taskID != question.TaskID || !taskFound.Valid || archivedAt.Valid || terminalAsyncQuestionSessionState(state) {
		return false, nil
	}
	if question.SessionIncarnationID != "" && incarnation != question.SessionIncarnationID {
		return false, nil
	}
	return true, nil
}

func (r *Repository) abandonParentQuestionIntentsTx(ctx context.Context, tx *sqlx.Tx, questionID, reason string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_deliveries
		   SET state = 'abandoned', last_error = ?
		 WHERE question_id = ? AND kind = 'parent_question' AND state = 'pending'`), reason, questionID)
	return err
}

func (r *Repository) finalizeAsyncQuestionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	current *models.AsyncQuestion,
	outcome models.AsyncQuestionOutcome,
	withdraw bool,
	withdrawReason string,
	now time.Time,
	deliveries []*models.AsyncQuestionDelivery,
	buildWake models.AsyncQuestionWakeBuilder,
) (models.AsyncQuestionStatus, int64, bool, []*models.AsyncQuestionWait, error) {
	status, nextVersion, won, err := r.applyAsyncQuestionTerminalTx(ctx, tx, current, outcome, withdraw, withdrawReason, now)
	if err != nil || !won {
		return status, nextVersion, won, nil, err
	}
	if err := r.insertAsyncQuestionDeliveriesTx(ctx, tx, current.ID, nextVersion, now, deliveries); err != nil {
		return status, nextVersion, false, nil, err
	}
	if withdraw {
		if err := r.abandonParentQuestionIntentsTx(ctx, tx, current.ID, "withdrawn"); err != nil {
			return status, nextVersion, false, nil, err
		}
	}
	released, err := r.releaseAsyncQuestionWaitsTx(ctx, tx, current.ID, now, string(status))
	if err != nil {
		return status, nextVersion, false, nil, err
	}
	if withdraw && buildWake != nil {
		if err := r.insertWithdrawWakesTx(ctx, tx, current, nextVersion, now, released, buildWake); err != nil {
			return status, nextVersion, false, nil, err
		}
	}
	return status, nextVersion, true, released, nil
}

func (r *Repository) insertWithdrawWakesTx(
	ctx context.Context,
	tx *sqlx.Tx,
	question *models.AsyncQuestion,
	version int64,
	now time.Time,
	released []*models.AsyncQuestionWait,
	buildWake models.AsyncQuestionWakeBuilder,
) error {
	wakeDeliveries := make([]*models.AsyncQuestionDelivery, 0, len(released))
	for _, wait := range released {
		if wait.Status == models.AsyncQuestionWaitReleaseRequested {
			continue
		}
		if delivery := buildWake(question, wait, version, "withdrawn"); delivery != nil {
			wakeDeliveries = append(wakeDeliveries, delivery)
		}
	}
	return r.insertAsyncQuestionDeliveriesTx(ctx, tx, question.ID, version, now, wakeDeliveries)
}

// ErrAsyncQuestionNotAuthor reports a withdrawal attempted by another session.
var ErrAsyncQuestionNotAuthor = errors.New("only the author session can withdraw an async question")

func (r *Repository) releaseAsyncQuestionWaitsTx(
	ctx context.Context,
	tx *sqlx.Tx,
	questionID string,
	now time.Time,
	releasedBy string,
) ([]*models.AsyncQuestionWait, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT id, workspace_id, task_id, session_id, turn_id, status, reason, released_by, created_at, updated_at
		  FROM task_async_question_waits
		 WHERE status IN ('requested', 'release_requested', 'parked')
		   AND id IN (SELECT wait_id FROM task_async_question_wait_dependencies WHERE question_id = ?)`),
		questionID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	waits := make([]*models.AsyncQuestionWait, 0, 1)
	for rows.Next() {
		var wait models.AsyncQuestionWait
		var status string
		if err := rows.Scan(&wait.ID, &wait.WorkspaceID, &wait.TaskID, &wait.SessionID, &wait.TurnID,
			&status, &wait.Reason, &wait.ReleasedBy, &wait.CreatedAt, &wait.UpdatedAt); err != nil {
			return nil, err
		}
		wait.Status = models.AsyncQuestionWaitStatus(status)
		waits = append(waits, &wait)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, wait := range waits {
		next := models.AsyncQuestionWaitReleased
		if wait.Status == models.AsyncQuestionWaitRequested {
			// The pause has not acknowledged yet. Fence the release until the
			// handler observes the settled pause, so an answer cannot start a
			// turn that the old pause would then cancel.
			next = models.AsyncQuestionWaitReleaseRequested
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_async_question_waits
			   SET status = ?, released_by = ?, updated_at = ?
			 WHERE id = ? AND status = ?`),
			string(next), releasedBy, now, wait.ID, string(wait.Status)); err != nil {
			return nil, err
		}
	}
	return waits, nil
}

func (r *Repository) lockAsyncQuestionTx(ctx context.Context, tx *sqlx.Tx, id string) (*models.AsyncQuestion, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		var lockedID string
		err := tx.QueryRowxContext(ctx, r.db.Rebind(
			"SELECT id FROM task_async_questions WHERE id = ?"+forUpdateClause), id).Scan(&lockedID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}
	row := tx.QueryRowxContext(ctx, r.db.Rebind(asyncQuestionSelectSQL+" WHERE task_async_questions.id = ?"), id)
	question, err := scanAsyncQuestion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return question, err
}

// ForwardAsyncQuestion links a pending child request to a direct parent. The
// first forward wins; a repeated forward by the same parent is idempotent and
// a different parent is rejected without changing ownership.
func (r *Repository) ForwardAsyncQuestion(ctx context.Context, forward *models.AsyncQuestionForward) (*models.AsyncQuestion, error) {
	return r.ForwardAsyncQuestionWithWake(ctx, forward, nil)
}

func (r *Repository) ForwardAsyncQuestionWithWake(
	ctx context.Context,
	forward *models.AsyncQuestionForward,
	buildWake models.AsyncQuestionWakeBuilder,
) (*models.AsyncQuestion, error) {
	if err := validateAsyncQuestionForward(forward); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.acquireAsyncQuestionWriteLockTx(ctx, tx, forward.QuestionID); err != nil {
		return nil, err
	}
	question, err := r.lockAsyncQuestionTx(ctx, tx, forward.QuestionID)
	if err != nil {
		return nil, err
	}
	if question == nil {
		return nil, ErrAsyncQuestionNotFound
	}
	if question.Status.Terminal() {
		return question, tx.Commit()
	}
	if err := authorizeAsyncQuestionForward(question, forward); err != nil {
		return nil, err
	}
	if err := r.validateAsyncQuestionAuthorityTx(ctx, tx, question, asyncQuestionAuthorityForward, "", buildWake); err != nil {
		if errors.Is(err, models.ErrAsyncQuestionStale) {
			if commitErr := tx.Commit(); commitErr != nil {
				return nil, commitErr
			}
		}
		return nil, err
	}
	if err := r.upsertAsyncQuestionForwardTx(ctx, tx, forward); err != nil {
		return nil, err
	}
	updated, err := r.getAsyncQuestionTx(ctx, tx, forward.QuestionID)
	if err != nil {
		return nil, err
	}
	if updated.Forwarded && updated.ParentTaskID != forward.ParentTaskID {
		return nil, ErrAsyncQuestionForwardUnauthorized
	}
	return updated, tx.Commit()
}

func validateAsyncQuestionForward(forward *models.AsyncQuestionForward) error {
	if forward == nil || forward.QuestionID == "" || forward.ParentTaskID == "" || forward.ParentSessionID == "" {
		return errors.New("async question forward identity is incomplete")
	}
	return nil
}

func authorizeAsyncQuestionForward(question *models.AsyncQuestion, forward *models.AsyncQuestionForward) error {
	if question.Recipient != models.AsyncQuestionRecipientParent || question.ParentTaskID != forward.ParentTaskID {
		return ErrAsyncQuestionForwardUnauthorized
	}
	return nil
}

func (r *Repository) upsertAsyncQuestionForwardTx(ctx context.Context, tx *sqlx.Tx, forward *models.AsyncQuestionForward) error {
	if forward.CreatedAt.IsZero() {
		forward.CreatedAt = r.nowUTC()
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_async_question_forwards (question_id, parent_task_id, parent_session_id, reason, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (question_id) DO UPDATE SET
			reason = task_async_question_forwards.reason
		WHERE task_async_question_forwards.parent_task_id = excluded.parent_task_id`),
		forward.QuestionID, forward.ParentTaskID, forward.ParentSessionID, forward.Reason, forward.CreatedAt,
	)
	return err
}

// ErrAsyncQuestionForwardUnauthorized reports an unauthorized forward target.
var ErrAsyncQuestionForwardUnauthorized = errors.New("async question forward is not authorized")

// ListAsyncQuestionDeliveries returns due durable outbox intents.
