package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/office/models"
)

// HasHandoffSourceRun checks actual historical participation, including a
// finished run. Handoff metadata has no run ID, so this does not identify an
// individual handoff turn or infer the most recent run for the session.
func (r *Repository) HasHandoffSourceRun(ctx context.Context, taskID, sessionID, agentProfileID string) (bool, error) {
	var exists bool
	taskIDExpr := dialect.JSONExtract(r.ro.DriverName(), "payload", "task_id")
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM runs WHERE session_id = ? AND agent_profile_id = ?
			  AND claimed_at IS NOT NULL AND %s = ?
		)
	`, taskIDExpr)), sessionID, agentProfileID, taskID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("read handoff source run relationship: %w", err)
	}
	return exists, nil
}

// CreateHandoffCompletionComment reuses the existing comment primary key as
// the completion key. Concurrent deliveries never create a second report.
func (r *Repository) CreateHandoffCompletionComment(ctx context.Context, comment *models.TaskComment) (*models.TaskComment, bool, error) {
	if comment == nil || comment.ID == "" || comment.TaskID == "" {
		return nil, false, fmt.Errorf("handoff completion comment identity is required")
	}
	comment.CreatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, reply_channel_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), comment.ID, comment.TaskID, comment.AuthorType, comment.AuthorID, comment.Body, comment.Source, comment.ReplyChannelID, comment.CreatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("create handoff completion comment: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("read handoff completion insert result: %w", err)
	}
	if inserted == 1 {
		return comment, true, nil
	}
	var existing models.TaskComment
	if err := r.db.QueryRowxContext(ctx, r.db.Rebind(`SELECT * FROM task_comments WHERE id = ?`), comment.ID).StructScan(&existing); err != nil {
		return nil, false, fmt.Errorf("read existing handoff completion comment: %w", err)
	}
	if existing.TaskID != comment.TaskID || existing.Source != comment.Source {
		return nil, false, fmt.Errorf("handoff completion comment belongs to another report")
	}
	return &existing, false, nil
}
