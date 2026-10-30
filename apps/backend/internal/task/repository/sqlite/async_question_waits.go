package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) CreateAsyncQuestionWait(ctx context.Context, wait *models.AsyncQuestionWait) (*models.AsyncQuestionWait, error) {
	if wait == nil || wait.SessionID == "" || len(wait.DependencyIDs) == 0 {
		return nil, errors.New("async question wait identity is incomplete")
	}
	if wait.ID == "" {
		wait.ID = uuid.NewString()
	}
	if wait.CreatedAt.IsZero() {
		wait.CreatedAt = r.nowUTC()
	}
	wait.UpdatedAt = wait.CreatedAt
	if wait.Status == "" {
		wait.Status = models.AsyncQuestionWaitRequested
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var activeID string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id FROM task_async_question_waits
		 WHERE session_id = ? AND status IN ('requested', 'release_requested', 'parked') LIMIT 1`), wait.SessionID).Scan(&activeID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if activeID != "" {
		return nil, ErrAsyncQuestionWaitAlreadyActive
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_async_question_waits
			(id, task_id, session_id, turn_id, status, reason, released_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, ?)`),
		wait.ID, wait.TaskID, wait.SessionID, wait.TurnID, string(wait.Status), wait.Reason,
		wait.CreatedAt, wait.UpdatedAt); err != nil {
		return nil, err
	}
	for _, dependency := range wait.DependencyIDs {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_async_question_wait_dependencies (wait_id, question_id)
			VALUES (?, ?) ON CONFLICT (wait_id, question_id) DO NOTHING`), wait.ID, dependency); err != nil {
			return nil, err
		}
	}
	return wait, tx.Commit()
}

// ErrAsyncQuestionWaitAlreadyActive reports a second explicit gate for one
// session.
var ErrAsyncQuestionWaitAlreadyActive = errors.New("an explicit question wait is already active")

// GetActiveAsyncQuestionWait returns the session's one active gate.
func (r *Repository) GetActiveAsyncQuestionWait(ctx context.Context, sessionID string) (*models.AsyncQuestionWait, error) {
	row := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, workspace_id, task_id, session_id, turn_id, status, reason, released_by, created_at, updated_at
		  FROM task_async_question_waits
		 WHERE session_id = ? AND status IN ('requested', 'release_requested', 'parked')
		 ORDER BY created_at DESC LIMIT 1`), sessionID)
	wait, err := scanAsyncQuestionWait(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dependencies, err := r.asyncQuestionWaitDependencies(ctx, wait.ID)
	if err != nil {
		return nil, err
	}
	wait.DependencyIDs = dependencies
	return wait, nil
}

// GetAsyncQuestionWait reads one gate by identity.
func (r *Repository) GetAsyncQuestionWait(ctx context.Context, waitID string) (*models.AsyncQuestionWait, error) {
	row := r.db.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id, workspace_id, task_id, session_id, turn_id, status, reason, released_by, created_at, updated_at
		  FROM task_async_question_waits WHERE id = ?`), waitID)
	wait, err := scanAsyncQuestionWait(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dependencies, err := r.asyncQuestionWaitDependencies(ctx, wait.ID)
	if err != nil {
		return nil, err
	}
	wait.DependencyIDs = dependencies
	return wait, nil
}

// ListAsyncQuestionWaits returns bounded gates in the supplied statuses.
func (r *Repository) ListAsyncQuestionWaits(ctx context.Context, statuses []models.AsyncQuestionWaitStatus, limit int) ([]*models.AsyncQuestionWait, error) {
	if limit <= 0 {
		limit = 200
	}
	placeholders := make([]string, len(statuses))
	args := make([]any, 0, len(statuses)+1)
	for i, status := range statuses {
		placeholders[i] = "?"
		args = append(args, string(status))
	}
	query := `SELECT id, workspace_id, task_id, session_id, turn_id, status, reason, released_by, created_at, updated_at
		FROM task_async_question_waits`
	if len(statuses) > 0 {
		query += " WHERE status IN (" + strings.Join(placeholders, ", ") + ")"
	}
	query += " ORDER BY created_at ASC, id ASC LIMIT ?"
	args = append(args, limit)
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	waits := make([]*models.AsyncQuestionWait, 0, limit)
	for rows.Next() {
		wait, scanErr := scanAsyncQuestionWait(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		waits = append(waits, wait)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, wait := range waits {
		dependencies, depErr := r.asyncQuestionWaitDependencies(ctx, wait.ID)
		if depErr != nil {
			return nil, depErr
		}
		wait.DependencyIDs = dependencies
	}
	return waits, nil
}

// SetAsyncQuestionWaitStatus transitions one gate.
func (r *Repository) SetAsyncQuestionWaitStatus(ctx context.Context, waitID string, status models.AsyncQuestionWaitStatus, releasedBy string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_waits
		   SET status = ?, released_by = ?, updated_at = ?
		 WHERE id = ? AND status IN ('requested', 'release_requested', 'parked')`),
		string(status), releasedBy, r.nowUTC(), waitID)
	return err
}

func (r *Repository) asyncQuestionWaitDependencies(ctx context.Context, waitID string) ([]string, error) {
	var dependencies []string
	err := r.db.SelectContext(ctx, &dependencies, r.db.Rebind(`
		SELECT question_id FROM task_async_question_wait_dependencies WHERE wait_id = ? ORDER BY question_id`), waitID)
	return dependencies, err
}

func scanAsyncQuestionWait(row rowScanner) (*models.AsyncQuestionWait, error) {
	var (
		wait   models.AsyncQuestionWait
		status string
	)
	if err := row.Scan(&wait.ID, &wait.WorkspaceID, &wait.TaskID, &wait.SessionID, &wait.TurnID, &status,
		&wait.Reason, &wait.ReleasedBy, &wait.CreatedAt, &wait.UpdatedAt); err != nil {
		return nil, err
	}
	wait.Status = models.AsyncQuestionWaitStatus(status)
	return &wait, nil
}

// CancelAsyncQuestionWaitsForSession closes any active gate for a session that
// is ending or being superseded. Returns the number of gates cancelled.
func (r *Repository) CancelAsyncQuestionWaitsForSession(ctx context.Context, sessionID, reason string) (int, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_question_waits
		   SET status = 'cancelled', released_by = ?, updated_at = ?
		 WHERE session_id = ? AND status IN ('requested', 'release_requested', 'parked')`), reason, r.nowUTC(), sessionID)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

// MarkStaleAsyncQuestionsForSession closes pending requests whose author
// session can no longer receive answers.
func (r *Repository) MarkStaleAsyncQuestionsForSession(ctx context.Context, sessionID, reason string) (int, error) {
	now := r.nowUTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_async_questions
		   SET status = 'stale', withdraw_reason = ?, version = version + 1,
		       updated_at = ?, resolved_at = ?
		 WHERE session_id = ? AND status = 'pending'`), reason, now, now, sessionID)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		if _, err := r.CancelAsyncQuestionWaitsForSession(ctx, sessionID, reason); err != nil {
			return int(affected), err
		}
	}
	return int(affected), nil
}

// DeleteAsyncQuestionsForTask removes question rows during task deletion.
// Archive keeps them so History retains the record.
func (r *Repository) DeleteAsyncQuestionsForTask(ctx context.Context, taskID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_async_question_wait_dependencies
		 WHERE wait_id IN (SELECT id FROM task_async_question_waits WHERE task_id = ?)`), taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_async_question_waits WHERE task_id = ?`), taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_async_question_deliveries
		 WHERE question_id IN (SELECT id FROM task_async_questions WHERE task_id = ?)`), taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_async_question_forwards
		 WHERE question_id IN (SELECT id FROM task_async_questions WHERE task_id = ?)`), taskID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_async_questions WHERE task_id = ?`), taskID); err != nil {
		return err
	}
	return tx.Commit()
}
