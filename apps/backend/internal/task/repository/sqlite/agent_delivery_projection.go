package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// ProjectCanonicalAgentDeliveryEvent persists output and advances the inbox
// cursor in one transaction. The returned bool is true when the stable
// canonical message already existed and this event appended to it.
func (r *Repository) ProjectCanonicalAgentDeliveryEvent(
	ctx context.Context,
	event *models.AgentDeliveryEvent,
	effect *models.AgentDeliveryEffect,
) (bool, error) {
	if event == nil {
		return false, fmt.Errorf("event is required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	stored, projectedAt, err := r.loadAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
	if err != nil {
		return false, err
	}
	if !sameAgentDeliveryEvent(&stored, event) {
		return false, repoerrors.ErrAgentDeliveryEventConflict
	}
	if projectedAt.Valid {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	var projectedSequence int64
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT projected_sequence FROM agent_delivery_cursors WHERE stream_id = ?`),
		stored.StreamID).Scan(&projectedSequence); err != nil {
		return false, fmt.Errorf("load canonical delivery cursor: %w", err)
	}
	if stored.Sequence != projectedSequence+1 {
		return false, fmt.Errorf("canonical agent delivery sequence gap: expected %d, got %d", projectedSequence+1, stored.Sequence)
	}

	var agentEvent streams.AgentEvent
	if err := json.Unmarshal(stored.Payload, &agentEvent); err != nil {
		return false, fmt.Errorf("decode canonical agent event: %w", err)
	}
	appendMessage, err := r.persistCanonicalAgentMessageTx(ctx, tx, &stored, agentEvent)
	if err != nil {
		return false, err
	}
	if effect != nil {
		if _, err := insertDeliveryEffectTx(ctx, tx, r.db.Rebind, effect); err != nil {
			return false, err
		}
	}
	if err := r.markAgentDeliveryProjectedTx(ctx, tx, stored.StreamID, stored.Sequence); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return appendMessage, nil
}

func (r *Repository) persistCanonicalAgentMessageTx(
	ctx context.Context,
	tx *sqlx.Tx,
	event *models.AgentDeliveryEvent,
	agentEvent streams.AgentEvent,
) (bool, error) {
	if agentEvent.Role == authorKindUser {
		return false, nil
	}
	content := agentEvent.Text
	messageType := "message"
	if agentEvent.Type == streams.EventTypeReasoning {
		content = agentEvent.ReasoningText
		messageType = string(models.MessageTypeThinking)
	}
	if content == "" {
		return false, nil
	}

	var taskID string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id FROM task_sessions WHERE id = ?`), event.SessionID).Scan(&taskID); err != nil {
		return false, fmt.Errorf("load canonical message task: %w", err)
	}
	turnID, err := r.canonicalDeliveryTurnTx(ctx, tx, event.SessionID, taskID, agentEvent.TurnID, event)
	if err != nil {
		return false, err
	}
	messageID := agentEvent.CanonicalMessageID
	if messageID == "" {
		messageID = fmt.Sprintf("agent-delivery-%s-%d", event.StreamID, event.Sequence)
	}
	now := r.nowUTC()
	var existingContent string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT content FROM task_session_messages WHERE id = ?`), messageID).Scan(&existingContent)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_session_messages
			SET content = ?, updated_at = ?
			WHERE id = ?`), existingContent+content, now, messageID); err != nil {
			return false, fmt.Errorf("append canonical agent message: %w", err)
		}
		return true, nil
	case err != sql.ErrNoRows:
		return false, fmt.Errorf("load canonical agent message: %w", err)
	}
	metadata, err := json.Marshal(map[string]interface{}{
		"durable_delivery":   true,
		"delivery_stream_id": event.StreamID,
		"delivery_sequence":  event.Sequence,
	})
	if err != nil {
		return false, fmt.Errorf("encode canonical agent message metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_messages
		(id, task_session_id, task_id, turn_id, author_type, author_id, content,
		 requests_input, type, metadata, created_at, updated_at, prompt_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 0)`),
		messageID, event.SessionID, taskID, turnID, string(models.MessageAuthorAgent),
		event.SubmissionID, content, messageType, string(metadata), now, now); err != nil {
		return false, fmt.Errorf("insert canonical agent message: %w", err)
	}
	return false, nil
}

func (r *Repository) canonicalDeliveryTurnTx(
	ctx context.Context,
	tx *sqlx.Tx,
	sessionID, taskID, requestedTurnID string,
	event *models.AgentDeliveryEvent,
) (string, error) {
	if requestedTurnID != "" {
		return requestedTurnID, nil
	}
	var turnID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT id FROM task_session_turns
		WHERE task_session_id = ? AND completed_at IS NULL
		ORDER BY started_at DESC, id DESC LIMIT 1`), sessionID).Scan(&turnID)
	if err == nil {
		return turnID, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("load canonical delivery turn: %w", err)
	}
	turnID = fmt.Sprintf("agent-delivery-turn-%s-%d", event.StreamID, event.Sequence)
	now := r.nowUTC()
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_session_turns
		(id, task_session_id, task_id, started_at, completed_at, execution_profile_id,
		route_generation, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, '', 0, '{}', ?, ?)
		ON CONFLICT (id) DO NOTHING`), turnID, sessionID, taskID, now, now, now)
	if err != nil {
		return "", fmt.Errorf("create canonical delivery turn: %w", err)
	}
	return turnID, nil
}

func (r *Repository) markAgentDeliveryProjectedTx(
	ctx context.Context,
	tx *sqlx.Tx,
	streamID string,
	sequence int64,
) error {
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_inbox SET projected_at = ?
		WHERE stream_id = ? AND sequence = ?`), now, streamID, sequence); err != nil {
		return err
	}
	if err := advanceProjectedCursorTx(ctx, tx, r.db.Rebind, streamID, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cursors SET updated_at = ? WHERE stream_id = ?`), now, streamID)
	return err
}
