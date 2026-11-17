package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	shellToolStatusComplete  = "complete"
	shellToolStatusCompleted = "completed"
	shellToolStatusSuccess   = "success"
	shellToolStatusError     = "error"
	shellToolStatusFailed    = "failed"
	shellToolStatusCancelled = "cancelled"
)

func (r *Repository) persistCanonicalShellStartTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent) (bool, error) {
	if agent.Type != streams.EventTypeToolCall || agent.Role == authorKindUser || agent.NormalizedPayload == nil || agent.NormalizedPayload.BackgroundWork() != nil {
		return false, nil
	}
	shell := agent.NormalizedPayload.ShellExec()
	if shell == nil || shell.Background || agent.ToolCallID == "" {
		return false, nil
	}
	_, _, err := loadShellOutputMessageTx(ctx, tx, event.SessionID, agent.ToolCallID)
	if err == nil {
		return true, nil
	}
	if err != sql.ErrNoRows {
		return false, err
	}
	var taskID string
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT task_id FROM task_sessions WHERE id=?`), event.SessionID).Scan(&taskID); err != nil {
		return false, err
	}
	turnID, err := r.canonicalShellStartTurnTx(ctx, tx, event, agent, taskID)
	if err != nil {
		return false, err
	}
	identity, err := json.Marshal([]any{event.SessionID, event.StreamID, agent.ToolCallID, agent.ParentToolCallID, agent.PromptGeneration})
	if err != nil {
		return false, err
	}
	digest := sha256.Sum256(identity)
	messageID := fmt.Sprintf("agent-delivery-shell-%x", digest[:16])
	metadata := map[string]any{"tool_call_id": agent.ToolCallID, "title": agent.ToolTitle, "status": agent.ToolStatus, "normalized": agent.NormalizedPayload}
	if agent.ParentToolCallID != "" {
		metadata["parent_tool_call_id"] = agent.ParentToolCallID
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return false, err
	}
	now := r.nowUTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO task_session_messages
 (id,task_session_id,task_id,turn_id,author_type,author_id,content,requests_input,type,metadata,created_at,updated_at,prompt_seq)
 VALUES(?,?,?,?,?,?,?,0,?,?,?,?,0)`), messageID, event.SessionID, taskID, turnID, string(models.MessageAuthorAgent), "", agent.ToolTitle, string(models.MessageTypeToolExecute), string(raw), now, now)
	return false, err
}

func (r *Repository) canonicalShellStartTurnTx(ctx context.Context, tx *sqlx.Tx, event *models.AgentDeliveryEvent, agent streams.AgentEvent, taskID string) (string, error) {
	var turnID string
	if agent.TurnID != "" {
		err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id FROM task_session_turns WHERE id=? AND task_session_id=?`), agent.TurnID, event.SessionID).Scan(&turnID)
		return turnID, err
	}
	switch agent.ToolStatus {
	case shellToolStatusComplete, shellToolStatusCompleted, shellToolStatusSuccess, shellToolStatusError, shellToolStatusFailed, shellToolStatusCancelled:
		err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT id FROM task_session_turns WHERE task_session_id=? AND completed_at IS NULL ORDER BY started_at DESC,id DESC LIMIT 1`), event.SessionID).Scan(&turnID)
		return turnID, err
	default:
		return r.canonicalDeliveryTurnTx(ctx, tx, event.SessionID, taskID, "", event)
	}
}
