package sqlite

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateTaskComment creates a new task comment.
func (r *Repository) CreateTaskComment(ctx context.Context, comment *models.TaskComment) error {
	if comment.ID == "" {
		comment.ID = uuid.New().String()
	}
	comment.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_comments (
			id, task_id, author_type, author_id, body,
			source, reply_channel_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), comment.ID, comment.TaskID, comment.AuthorType, comment.AuthorID,
		comment.Body, comment.Source, comment.ReplyChannelID, comment.CreatedAt)
	return err
}

// ListTaskComments returns all comments for a task, ordered by creation time.
func (r *Repository) ListTaskComments(ctx context.Context, taskID string) ([]*models.TaskComment, error) {
	var comments []*models.TaskComment
	err := r.ro.SelectContext(ctx, &comments, r.ro.Rebind(
		`SELECT * FROM task_comments WHERE task_id = ? ORDER BY created_at`), taskID)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []*models.TaskComment{}
	}
	return comments, nil
}

// DeleteTaskComment deletes a task comment by ID.
func (r *Repository) DeleteTaskComment(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_comments WHERE id = ?`), id)
	return err
}
