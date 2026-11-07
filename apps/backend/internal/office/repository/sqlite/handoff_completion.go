package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/office/models"
)

// HandoffCompletionOutbox is the durable work left after a handoff completion
// is authorized. The comment event and source-task wake have independent
// delivery state because either side effect can fail on its own.
type HandoffCompletionOutbox struct {
	CompletionKey                string    `db:"completion_key"`
	CommentID                    string    `db:"comment_id"`
	SourceTaskID                 string    `db:"source_task_id"`
	SourceWorkspaceID            string    `db:"source_workspace_id"`
	SourceSessionID              string    `db:"source_session_id"`
	SourceAgentProfileID         string    `db:"source_agent_profile_id"`
	SourceAssigneeAgentProfileID string    `db:"source_assignee_agent_profile_id"`
	DeliveryTaskID               string    `db:"delivery_task_id"`
	DeliveryWorkspaceID          string    `db:"delivery_workspace_id"`
	DeliveryTitle                string    `db:"delivery_title"`
	DeliveryState                string    `db:"delivery_state"`
	CommentEventPending          bool      `db:"comment_event_pending"`
	WakePending                  bool      `db:"wake_pending"`
	CreatedAt                    time.Time `db:"created_at"`
	LastAttemptAt                time.Time `db:"last_attempt_at"`
}

func sameHandoffCompletionIdentity(existing, incoming *HandoffCompletionOutbox) bool {
	return existing.SourceTaskID == incoming.SourceTaskID &&
		existing.SourceWorkspaceID == incoming.SourceWorkspaceID &&
		existing.SourceSessionID == incoming.SourceSessionID &&
		existing.SourceAgentProfileID == incoming.SourceAgentProfileID &&
		existing.DeliveryTaskID == incoming.DeliveryTaskID &&
		existing.DeliveryWorkspaceID == incoming.DeliveryWorkspaceID &&
		existing.DeliveryState == incoming.DeliveryState
}

func (r *Repository) createHandoffCompletionOutboxTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_handoff_completion_outbox (
		completion_key TEXT PRIMARY KEY,
		comment_id TEXT NOT NULL DEFAULT '',
		source_task_id TEXT NOT NULL,
		source_workspace_id TEXT NOT NULL,
		source_session_id TEXT NOT NULL,
		source_agent_profile_id TEXT NOT NULL,
		source_assignee_agent_profile_id TEXT NOT NULL DEFAULT '',
		delivery_task_id TEXT NOT NULL,
		delivery_workspace_id TEXT NOT NULL,
		delivery_title TEXT NOT NULL,
		delivery_state TEXT NOT NULL,
		comment_event_pending BOOLEAN NOT NULL DEFAULT FALSE,
		wake_pending BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL,
		last_attempt_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_handoff_completion_outbox_pending
		ON office_handoff_completion_outbox(comment_event_pending, wake_pending, last_attempt_at, created_at);
	`)
	return err
}

// EnsureHandoffCompletionOutbox atomically stores the optional report comment
// and its durable follow-up work. The completion key is a write-once receipt,
// so a replay cannot recreate an already delivered wake after run retention.
func (r *Repository) EnsureHandoffCompletionOutbox(
	ctx context.Context, item *HandoffCompletionOutbox, comment *models.TaskComment,
) error {
	if err := validateHandoffCompletionOutbox(item, comment); err != nil {
		return err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin handoff completion transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, err := getHandoffCompletionOutboxTx(ctx, tx, item.CompletionKey)
	if err != nil {
		return err
	}
	if existing != nil {
		if !sameHandoffCompletionIdentity(existing, item) {
			return fmt.Errorf("handoff completion key belongs to another delivery")
		}
		return nil
	}

	if comment != nil {
		if err := reuseHandoffCompletionAnchorTx(ctx, tx, item.CompletionKey, comment); err != nil {
			return err
		}
		item.CommentID = comment.ID
		if err := ensureHandoffCompletionCommentTx(ctx, tx, comment); err != nil {
			return err
		}
	}

	item.CreatedAt = time.Now().UTC()
	item.LastAttemptAt = item.CreatedAt
	if err := insertHandoffCompletionOutboxTx(ctx, tx, item); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit handoff completion transaction: %w", err)
	}
	return nil
}

func validateHandoffCompletionOutbox(item *HandoffCompletionOutbox, comment *models.TaskComment) error {
	if item == nil || item.CompletionKey == "" || item.SourceTaskID == "" || item.DeliveryTaskID == "" {
		return fmt.Errorf("handoff completion outbox identity is required")
	}
	if item.CommentEventPending != (comment != nil) ||
		(item.WakePending && item.SourceAssigneeAgentProfileID == "") {
		return fmt.Errorf("handoff completion outbox actions do not match their payload")
	}
	if comment != nil && (comment.ID == "" || comment.ID != item.CommentID || comment.TaskID != item.SourceTaskID) {
		return fmt.Errorf("handoff completion comment identity does not match its outbox")
	}
	return nil
}

func getHandoffCompletionOutboxTx(
	ctx context.Context, tx *sqlx.Tx, completionKey string,
) (*HandoffCompletionOutbox, error) {
	var item HandoffCompletionOutbox
	err := tx.QueryRowxContext(ctx, tx.Rebind(`
		SELECT * FROM office_handoff_completion_outbox WHERE completion_key = ?
	`), completionKey).StructScan(&item)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read handoff completion outbox receipt: %w", err)
	}
	return &item, nil
}

// reuseHandoffCompletionAnchorTx retains historical UUID report identities.
// Resolve the anchor in the same transaction that stores the durable receipt;
// the existing body and nonempty author remain owned by the stored comment.
func reuseHandoffCompletionAnchorTx(
	ctx context.Context, tx *sqlx.Tx, completionKey string, comment *models.TaskComment,
) error {
	if comment.Source != "handoff" {
		return nil
	}
	var comments []models.TaskComment
	if err := tx.SelectContext(ctx, &comments, tx.Rebind(`
  SELECT * FROM task_comments WHERE task_id = ? AND source = ? ORDER BY created_at, id
 `), comment.TaskID, comment.Source); err != nil {
		return fmt.Errorf("find historical handoff completion comment: %w", err)
	}
	anchor := "<!-- handoff-completion:" + completionKey + " -->"
	for _, existing := range comments {
		if strings.Contains(existing.Body, anchor) {
			comment.ID = existing.ID
			return nil
		}
	}
	return nil
}

func ensureHandoffCompletionCommentTx(ctx context.Context, tx *sqlx.Tx, comment *models.TaskComment) error {
	comment.CreatedAt = time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO task_comments (id, task_id, author_type, author_id, body, source, reply_channel_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING
	`), comment.ID, comment.TaskID, comment.AuthorType, comment.AuthorID, comment.Body, comment.Source,
		comment.ReplyChannelID, comment.CreatedAt)
	if err != nil {
		return fmt.Errorf("create handoff completion comment: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read handoff completion comment result: %w", err)
	}
	if inserted == 1 {
		return nil
	}

	var existing models.TaskComment
	if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT * FROM task_comments WHERE id = ?`), comment.ID).StructScan(&existing); err != nil {
		return fmt.Errorf("read existing handoff completion comment: %w", err)
	}
	if existing.TaskID != comment.TaskID || existing.Source != comment.Source {
		return fmt.Errorf("handoff completion comment belongs to another report")
	}
	if existing.AuthorID == "" && comment.AuthorID != "" && existing.AuthorType == comment.AuthorType {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE task_comments SET author_id = ? WHERE id = ? AND author_id = ''
		`), comment.AuthorID, comment.ID); err != nil {
			return fmt.Errorf("repair handoff completion comment author: %w", err)
		}
	}
	return nil
}

func insertHandoffCompletionOutboxTx(ctx context.Context, tx *sqlx.Tx, item *HandoffCompletionOutbox) error {
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO office_handoff_completion_outbox (
			completion_key, comment_id, source_task_id, source_workspace_id,
			source_session_id, source_agent_profile_id, source_assignee_agent_profile_id,
			delivery_task_id, delivery_workspace_id, delivery_title, delivery_state,
			comment_event_pending, wake_pending, created_at, last_attempt_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (completion_key) DO NOTHING
	`), item.CompletionKey, item.CommentID, item.SourceTaskID, item.SourceWorkspaceID,
		item.SourceSessionID, item.SourceAgentProfileID, item.SourceAssigneeAgentProfileID,
		item.DeliveryTaskID, item.DeliveryWorkspaceID, item.DeliveryTitle, item.DeliveryState,
		item.CommentEventPending, item.WakePending, item.CreatedAt, item.LastAttemptAt)
	if err != nil {
		return fmt.Errorf("create handoff completion outbox: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read handoff completion outbox result: %w", err)
	}
	if inserted == 1 {
		return nil
	}
	existing, err := getHandoffCompletionOutboxTx(ctx, tx, item.CompletionKey)
	if err != nil {
		return fmt.Errorf("read concurrent handoff completion receipt: %w", err)
	}
	if existing == nil || !sameHandoffCompletionIdentity(existing, item) {
		return fmt.Errorf("handoff completion key belongs to another delivery")
	}
	return nil
}

func (r *Repository) GetHandoffCompletionOutbox(ctx context.Context, completionKey string) (*HandoffCompletionOutbox, error) {
	var item HandoffCompletionOutbox
	err := r.ro.GetContext(ctx, &item, r.ro.Rebind(`
		SELECT * FROM office_handoff_completion_outbox WHERE completion_key = ?
	`), completionKey)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read handoff completion outbox: %w", err)
	}
	return &item, nil
}

func (r *Repository) ListPendingHandoffCompletionOutbox(ctx context.Context, limit int) ([]HandoffCompletionOutbox, error) {
	var items []HandoffCompletionOutbox
	err := r.ro.SelectContext(ctx, &items, r.ro.Rebind(`
		SELECT * FROM office_handoff_completion_outbox
		WHERE comment_event_pending = ? OR wake_pending = ?
		ORDER BY last_attempt_at ASC, created_at ASC, completion_key ASC
		LIMIT ?
	`), true, true, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending handoff completion outbox: %w", err)
	}
	return items, nil
}

func (r *Repository) TouchHandoffCompletionOutboxAttempt(ctx context.Context, completionKey string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_handoff_completion_outbox SET last_attempt_at = ?
		WHERE completion_key = ?
	`), time.Now().UTC(), completionKey)
	if err != nil {
		return fmt.Errorf("record handoff completion outbox attempt: %w", err)
	}
	return nil
}

func (r *Repository) MarkHandoffCompletionCommentEventPublished(ctx context.Context, completionKey string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_handoff_completion_outbox SET comment_event_pending = ?
		WHERE completion_key = ?
	`), false, completionKey)
	if err != nil {
		return fmt.Errorf("mark handoff completion comment event published: %w", err)
	}
	return nil
}

func (r *Repository) MarkHandoffCompletionWakeResolved(ctx context.Context, completionKey string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_handoff_completion_outbox SET wake_pending = ?
		WHERE completion_key = ?
	`), false, completionKey)
	if err != nil {
		return fmt.Errorf("resolve handoff completion wake: %w", err)
	}
	return nil
}
