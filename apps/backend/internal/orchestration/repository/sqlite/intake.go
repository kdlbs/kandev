package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// AcceptComment atomically stores intent and its outbox receipt. The first write
// serializes SQLite transactions before any read/modify/write sequence.
func (r *Repository) AcceptComment(ctx context.Context, agentID, clientID string, c *models.TaskComment) (*models.Intake, bool, error) {
	if strings.TrimSpace(c.Body) == "" || len(c.Body) > 32000 || !utf8.ValidString(c.Body) ||
		c.AuthorID == "" || len(clientID) > 200 || !utf8.ValidString(clientID) {
		return nil, false, fmt.Errorf("invalid conversation message")
	}
	if clientID == "" {
		clientID = uuid.NewString()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_conversation_intents(task_id,revision) VALUES(?,0) ON CONFLICT(task_id) DO NOTHING`), c.TaskID)
	if err != nil {
		return nil, false, err
	}
	var receipt models.Intake
	if err := authorizeIntake(ctx, tx, agentID, c); err != nil {
		return nil, false, err
	}
	err = tx.GetContext(ctx, &receipt, tx.Rebind(`SELECT * FROM orchestration_intake WHERE task_id=? AND owner_user_id=? AND client_message_id=?`), c.TaskID, c.AuthorID, clientID)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(c.Body)))
	if err == nil {
		if receipt.PayloadHash != digest {
			return nil, false, models.ErrConflict
		}
		return &receipt, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}
	receipt = models.Intake{CommentID: uuid.NewString(), TaskID: c.TaskID, AgentID: agentID,
		OwnerUserID: c.AuthorID, ClientMessageID: clientID, PayloadHash: digest, Status: "accepted"}
	if err := insertIntake(ctx, tx, &receipt, c); err != nil {
		return nil, false, err
	}
	return &receipt, true, tx.Commit()
}

func insertIntake(ctx context.Context, tx *sqlx.Tx, receipt *models.Intake, c *models.TaskComment) error {
	if err := tx.GetContext(ctx, &receipt.Sequence, tx.Rebind(`UPDATE orchestration_conversation_intents SET revision=revision+1 WHERE task_id=? RETURNING revision`), c.TaskID); err != nil {
		return err
	}
	c.ID, c.CreatedAt = receipt.CommentID, time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_comments(id,task_id,author_type,author_id,body,source,created_at) VALUES(?,?,'user',?,?,'user',?)`),
		c.ID, c.TaskID, c.AuthorID, c.Body, c.CreatedAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_intake(comment_id,task_id,agent_id,owner_user_id,client_message_id,payload_hash,sequence,status) VALUES(?,?,?,?,?,?,?,?)`),
		receipt.CommentID, receipt.TaskID, receipt.AgentID, receipt.OwnerUserID, receipt.ClientMessageID, receipt.PayloadHash, receipt.Sequence, receipt.Status)
	return err
}

func (r *Repository) PendingIntake(ctx context.Context) ([]models.Intake, error) {
	rows := []models.Intake{}
	err := r.db.SelectContext(ctx, &rows, `SELECT i.* FROM orchestration_intake i
		JOIN workspace_orchestrators o ON o.agent_id=i.agent_id
		JOIN agent_profiles a ON a.id=o.agent_id
		WHERE i.status='accepted' AND a.deleted_at IS NULL AND a.status NOT IN ('paused','stopped')
		ORDER BY i.task_id,i.sequence LIMIT 100`)
	return rows, err
}

func (r *Repository) AcknowledgeIntake(ctx context.Context, commentID, runID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_intake SET status='queued',run_id=? WHERE comment_id=? AND status='accepted'`), runID, commentID)
	return err
}

func (r *Repository) IntentRevision(ctx context.Context, taskID string) (int64, error) {
	var revision int64
	err := r.db.GetContext(ctx, &revision, r.db.Rebind(`SELECT revision FROM orchestration_conversation_intents WHERE task_id=?`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return revision, err
}
