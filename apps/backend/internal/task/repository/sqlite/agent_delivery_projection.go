package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	appendMessages, err := r.ProjectCanonicalAgentDeliveryEvents(ctx,
		[]*models.AgentDeliveryEvent{event}, []*models.AgentDeliveryEffect{effect})
	if err != nil {
		return false, err
	}
	return len(appendMessages) == 1 && appendMessages[0], nil
}

// ProjectCanonicalAgentDeliveryEvents projects one contiguous stream batch in
// one transaction. Every inbox sequence and effect remains individually
// idempotent, while compatible message chunks share the transaction and the
// cursor advances only after each committed event has been applied.
func (r *Repository) ProjectCanonicalAgentDeliveryEvents(
	ctx context.Context,
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
) ([]bool, error) {
	streamID, err := validateCanonicalDeliveryBatch(events, effects)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var projectedSequence int64
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT projected_sequence FROM agent_delivery_cursors WHERE stream_id = ?`),
		streamID).Scan(&projectedSequence); err != nil {
		return nil, fmt.Errorf("load canonical delivery cursor: %w", err)
	}
	appendMessages := make([]bool, len(events))
	if err := r.projectCanonicalDeliveryBatchTx(
		ctx, tx, events, effects, appendMessages, projectedSequence,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return appendMessages, nil
}

func (r *Repository) projectCanonicalDeliveryBatchTx(
	ctx context.Context,
	tx *sqlx.Tx,
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
	appendMessages []bool,
	projectedSequence int64,
) error {
	group := make([]canonicalDeliveryBatchEntry, 0, len(events))
	flushGroup := func() error {
		if len(group) == 0 {
			return nil
		}
		if err := r.projectCanonicalDeliveryBatchGroupTx(ctx, tx, group, appendMessages); err != nil {
			return err
		}
		group = group[:0]
		return nil
	}
	for i, event := range events {
		stored, projectedAt, loadErr := r.loadAgentDeliveryEventTx(ctx, tx, event.StreamID, event.Sequence)
		if loadErr != nil {
			return loadErr
		}
		if !sameAgentDeliveryEvent(&stored, event) {
			return repoerrors.ErrAgentDeliveryEventConflict
		}
		if projectedAt.Valid {
			if err := flushGroup(); err != nil {
				return err
			}
			if stored.Sequence > projectedSequence {
				projectedSequence = stored.Sequence
			}
			continue
		}
		if stored.Sequence != projectedSequence+1 {
			return fmt.Errorf("canonical agent delivery sequence gap: expected %d, got %d", projectedSequence+1, stored.Sequence)
		}
		var agentEvent streams.AgentEvent
		if err := json.Unmarshal(stored.Payload, &agentEvent); err != nil {
			return fmt.Errorf("decode canonical agent event: %w", err)
		}
		if len(group) > 0 && !compatibleCanonicalDeliveryEvents(group[0].agentEvent, agentEvent) {
			if err := flushGroup(); err != nil {
				return err
			}
		}
		group = append(group, canonicalDeliveryBatchEntry{
			index:      i,
			stored:     stored,
			agentEvent: agentEvent,
			effect:     effects[i],
		})
		projectedSequence = stored.Sequence
	}
	if err := flushGroup(); err != nil {
		return err
	}
	return nil
}

type canonicalDeliveryBatchEntry struct {
	index      int
	stored     models.AgentDeliveryEvent
	agentEvent streams.AgentEvent
	effect     *models.AgentDeliveryEffect
}

func compatibleCanonicalDeliveryEvents(previous, next streams.AgentEvent) bool {
	return previous.Type == next.Type && previous.CanonicalMessageID != "" &&
		previous.CanonicalMessageID == next.CanonicalMessageID
}

func (r *Repository) projectCanonicalDeliveryBatchGroupTx(
	ctx context.Context,
	tx *sqlx.Tx,
	group []canonicalDeliveryBatchEntry,
	appendMessages []bool,
) error {
	firstContent, content := canonicalDeliveryBatchContent(group)
	if firstContent != -1 {
		aggregate := group[firstContent].agentEvent
		if aggregate.Type == streams.EventTypeReasoning {
			aggregate.ReasoningText = content
		} else {
			aggregate.Text = content
		}
		appended, err := r.persistCanonicalAgentMessageTx(
			ctx, tx, &group[firstContent].stored, aggregate,
		)
		if err != nil {
			return err
		}
		setCanonicalDeliveryAppendMessages(group, appendMessages, firstContent, appended)
	}
	return r.markCanonicalDeliveryBatchProjectedTx(ctx, tx, group)
}

func canonicalDeliveryBatchContent(group []canonicalDeliveryBatchEntry) (int, string) {
	firstContent := -1
	var content strings.Builder
	for index, item := range group {
		chunk := canonicalDeliveryEventContent(item.agentEvent)
		if chunk == "" {
			continue
		}
		if firstContent == -1 {
			firstContent = index
		}
		content.WriteString(chunk)
	}
	return firstContent, content.String()
}

func setCanonicalDeliveryAppendMessages(
	group []canonicalDeliveryBatchEntry,
	appendMessages []bool,
	firstContent int,
	appended bool,
) {
	for index, item := range group {
		if canonicalDeliveryEventContent(item.agentEvent) == "" {
			continue
		}
		appendMessages[item.index] = appended || index > firstContent
	}
}

func (r *Repository) markCanonicalDeliveryBatchProjectedTx(
	ctx context.Context,
	tx *sqlx.Tx,
	group []canonicalDeliveryBatchEntry,
) error {
	now := r.nowUTC()
	for _, item := range group {
		if item.effect != nil {
			if _, err := insertDeliveryEffectTx(ctx, tx, r.db.Rebind, item.effect); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE agent_delivery_inbox SET projected_at = ?
			WHERE stream_id = ? AND sequence = ?`), now, item.stored.StreamID, item.stored.Sequence); err != nil {
			return err
		}
	}
	if err := advanceProjectedCursorTx(ctx, tx, r.db.Rebind, group[0].stored.StreamID, now); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE agent_delivery_cursors SET updated_at = ? WHERE stream_id = ?`),
		now, group[0].stored.StreamID)
	return err
}

func canonicalDeliveryEventContent(event streams.AgentEvent) string {
	if event.Role == authorKindUser {
		return ""
	}
	if event.Type == streams.EventTypeReasoning {
		return event.ReasoningText
	}
	return event.Text
}

func validateCanonicalDeliveryBatch(
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
) (string, error) {
	if len(events) == 0 {
		return "", nil
	}
	if len(effects) != len(events) {
		return "", fmt.Errorf("canonical delivery events and effects must have equal lengths")
	}
	if events[0] == nil {
		return "", fmt.Errorf("event 0 is required")
	}
	streamID := events[0].StreamID
	if streamID == "" {
		return "", fmt.Errorf("event stream is required")
	}
	for i, event := range events {
		if event == nil {
			return "", fmt.Errorf("event %d is required", i)
		}
		if event.StreamID != streamID {
			return "", fmt.Errorf("canonical delivery batch contains multiple streams")
		}
	}
	return streamID, nil
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
	var existingMetadata string
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT content, metadata FROM task_session_messages WHERE id = ?`), messageID).Scan(&existingContent, &existingMetadata)
	switch {
	case err == nil:
		return r.updateCanonicalAgentMessageTx(
			ctx, tx, messageID, messageType, existingContent, existingMetadata, content, now,
		)
	case err != sql.ErrNoRows:
		return false, fmt.Errorf("load canonical agent message: %w", err)
	}
	messageMetadata := map[string]interface{}{
		"durable_delivery":   true,
		"delivery_stream_id": event.StreamID,
		"delivery_sequence":  event.Sequence,
	}
	if messageType == string(models.MessageTypeThinking) {
		messageMetadata["thinking"] = content
		content = ""
	}
	metadata, err := json.Marshal(messageMetadata)
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

func (r *Repository) updateCanonicalAgentMessageTx(
	ctx context.Context,
	tx *sqlx.Tx,
	messageID, messageType, existingContent, existingMetadata, content string,
	now time.Time,
) (bool, error) {
	if messageType == string(models.MessageTypeThinking) {
		metadata, err := mergeThinkingMetadata(existingMetadata, content)
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_session_messages
			SET metadata = ?, updated_at = ?
			WHERE id = ?`), metadata, now, messageID); err != nil {
			return false, fmt.Errorf("append canonical agent thinking message: %w", err)
		}
		return true, nil
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_session_messages
		SET content = ?, updated_at = ?
		WHERE id = ?`), existingContent+content, now, messageID); err != nil {
		return false, fmt.Errorf("append canonical agent message: %w", err)
	}
	return true, nil
}

func mergeThinkingMetadata(raw, content string) (string, error) {
	metadata := make(map[string]interface{})
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return "", fmt.Errorf("decode canonical agent thinking metadata: %w", err)
		}
	}
	existing, _ := metadata["thinking"].(string)
	metadata["thinking"] = existing + content
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode canonical agent thinking metadata: %w", err)
	}
	return string(encoded), nil
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
