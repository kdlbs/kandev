package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
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

// GetTaskComment returns a single comment by ID.
func (r *Repository) GetTaskComment(ctx context.Context, id string) (*models.TaskComment, error) {
	var comment models.TaskComment
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM task_comments WHERE id = ?`), id).StructScan(&comment)
	if err != nil {
		return nil, fmt.Errorf("get comment %s: %w", id, err)
	}
	return &comment, nil
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

// RecentComment holds the fields returned by ListRecentTaskComments.
type RecentComment struct {
	AuthorID   string `db:"author_id"`
	AuthorType string `db:"author_type"`
	Body       string `db:"body"`
	CreatedAt  string `db:"created_at"`
}

// ListRecentTaskComments returns the most recent comments for a task,
// ordered newest-first, limited to the given count.
func (r *Repository) ListRecentTaskComments(
	ctx context.Context, taskID string, limit int,
) ([]*RecentComment, error) {
	var comments []*RecentComment
	err := r.ro.SelectContext(ctx, &comments, r.ro.Rebind(`
		SELECT author_id, author_type, body, created_at
		FROM task_comments
		WHERE task_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`), taskID, limit)
	if err != nil {
		return nil, err
	}
	if comments == nil {
		comments = []*RecentComment{}
	}
	return comments, nil
}

// GetLatestCommentBody returns the most recent comment body by a specific author on a task.
func (r *Repository) GetLatestCommentBody(ctx context.Context, taskID, authorID string) (string, error) {
	var body string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT body FROM task_comments
		WHERE task_id = ? AND author_id = ?
		ORDER BY created_at DESC LIMIT 1
	`), taskID, authorID).Scan(&body)
	if err != nil {
		return "", err
	}
	return body, nil
}

// CountTaskComments returns the total number of comments on a task.
func (r *Repository) CountTaskComments(ctx context.Context, taskID string) (int, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM task_comments WHERE task_id = ?`), taskID).Scan(&count)
	return count, err
}

// HasCommentWithSource returns true when at least one comment with the given
// source value exists for the task. Used for deduplication of auto-bridged
// agent session comments.
func (r *Repository) HasCommentWithSource(ctx context.Context, taskID, source string) (bool, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM task_comments WHERE task_id = ? AND source = ?`),
		taskID, source).Scan(&count)
	return count > 0, err
}

// HasCommentWithSourceAndBody returns true when at least one comment with
// the given source AND body already exists for the task. Used to dedup
// per-turn auto-bridged session comments: each agent turn produces a
// distinct response body, so matching on body lets us bridge once per
// turn while the same session is reused across turns.
func (r *Repository) HasCommentWithSourceAndBody(
	ctx context.Context, taskID, source, body string,
) (bool, error) {
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM task_comments WHERE task_id = ? AND source = ? AND body = ?`),
		taskID, source, body).Scan(&count)
	return count > 0, err
}
