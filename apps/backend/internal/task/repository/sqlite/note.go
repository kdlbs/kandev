package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/task/models"
)

const taskNoteSelectCols = `id, task_id, user_id, content, updated_by, created_at, updated_at`

// GetTaskNote retrieves a task note for a (task, user) pair. userID is the
// note owner; "" is the unscoped/auth-disabled owner.
func (r *Repository) GetTaskNote(ctx context.Context, taskID, userID string) (*models.TaskNote, error) {
	note, err := scanTaskNoteRow(r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+taskNoteSelectCols+`
		FROM task_notes WHERE task_id = ? AND user_id = ?
	`), taskID, userID))
	if err != nil {
		return nil, fmt.Errorf("failed to get task note: %w", err)
	}
	return note, nil
}

// UpsertTaskNote creates or replaces a task note by task ID.
func (r *Repository) UpsertTaskNote(ctx context.Context, note *models.TaskNote) error {
	now := time.Now().UTC()
	if note.ID == "" {
		note.ID = uuid.New().String()
	}
	if note.UpdatedBy == "" {
		note.UpdatedBy = authorKindUser
	}
	if note.CreatedAt.IsZero() {
		note.CreatedAt = now
	}
	note.UpdatedAt = now

	if _, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_notes (id, task_id, user_id, content, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, user_id) DO UPDATE SET
			content = excluded.content,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at
	`), note.ID, note.TaskID, note.UserID, note.Content, note.UpdatedBy, note.CreatedAt, note.UpdatedAt); err != nil {
		return fmt.Errorf("failed to upsert task note: %w", err)
	}
	return nil
}

// DeleteTaskNote deletes a (task, user) note.
func (r *Repository) DeleteTaskNote(ctx context.Context, taskID, userID string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_notes WHERE task_id = ? AND user_id = ?`), taskID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete task note: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrTaskNoteNotFound, taskID)
	}
	return nil
}

func scanTaskNoteRow(row *sql.Row) (*models.TaskNote, error) {
	note := &models.TaskNote{}
	if err := row.Scan(
		&note.ID,
		&note.TaskID,
		&note.UserID,
		&note.Content,
		&note.UpdatedBy,
		&note.CreatedAt,
		&note.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return note, nil
}
