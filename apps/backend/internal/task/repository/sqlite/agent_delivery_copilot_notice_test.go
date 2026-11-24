package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	copilotNoticeStatusMessageID = "copilot-notice-status"
	copilotNoticeReplyMessageID  = "copilot-notice-reply"
	copilotNoticeStreamID        = "copilot-notice-stream"
	copilotNoticeWarning         = "Warning: 1 tool call(s) had no recorded result at resume."
)

func TestCopilotInterruptedToolStatusProjection(t *testing.T) {
	repositories := map[string]func(*testing.T) *Repository{
		"sqlite":   newRepoForSessionTests,
		"postgres": openPostgresRepo,
	}
	for name, open := range repositories {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := open(t)
			seedCombinedDeliveryBatchSession(t, repo)
			events := []*models.AgentDeliveryEvent{
				copilotNoticeDeliveryEvent(t, 1, copilotNoticeStatusMessageID, copilotNoticeWarning,
					streams.CopilotInterruptionClarification, "turn-combined-batch"),
				copilotNoticeDeliveryEvent(t, 2, copilotNoticeReplyMessageID, "Here is the next step.", "",
					"turn-combined-batch"),
			}
			effects := combinedDeliveryBatchEffects(events)

			repo.agentDeliveryCombinedProjectionFailpoint = func(string) error {
				return errors.New("injected projection failure")
			}
			if _, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0); err == nil {
				t.Fatal("projection succeeded despite the injected failure")
			}
			repo.agentDeliveryCombinedProjectionFailpoint = nil
			for _, messageID := range []string{copilotNoticeStatusMessageID, copilotNoticeReplyMessageID} {
				if got := countCombinedDeliveryRows(t, repo,
					`SELECT COUNT(*) FROM task_session_messages WHERE id = ?`, messageID); got != 0 {
					t.Fatalf("message %q survived rollback: %d rows", messageID, got)
				}
			}
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ?`, copilotNoticeStreamID); got != 0 {
				t.Fatalf("inbox rows after rollback = %d, want 0", got)
			}
			if got := countCombinedDeliveryRows(t, repo,
				`SELECT COUNT(*) FROM agent_delivery_cursors WHERE stream_id = ?`, copilotNoticeStreamID); got != 0 {
				t.Fatalf("cursor rows after rollback = %d, want 0", got)
			}

			results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0)
			if err != nil {
				t.Fatalf("retry projection: %v", err)
			}
			if len(results) != 2 || results[0].CanonicalMessageID != copilotNoticeStatusMessageID ||
				results[1].CanonicalMessageID != copilotNoticeReplyMessageID {
				t.Fatalf("projection results = %+v, want separate status and reply rows", results)
			}
			status, err := repo.GetMessage(ctx, copilotNoticeStatusMessageID)
			if err != nil {
				t.Fatalf("read interruption status: %v", err)
			}
			if status.Type != models.MessageTypeStatus || status.Content != copilotNoticeWarning ||
				status.Metadata[streams.AgentEventDataCopilotInterruptionNotice] != streams.CopilotInterruptionClarification ||
				status.Metadata["variant"] != streams.EventTypeCopilotInterruptionNotice ||
				status.Metadata["delivery_sequence"] != float64(1) {
				t.Fatalf("interruption status = %+v, want original text and retained clarification metadata", status)
			}
			reply, err := repo.GetMessage(ctx, copilotNoticeReplyMessageID)
			if err != nil {
				t.Fatalf("read assistant reply: %v", err)
			}
			if reply.Type != models.MessageTypeMessage || reply.Content != "Here is the next step." ||
				reply.Metadata[streams.AgentEventDataCopilotInterruptionNotice] != nil ||
				reply.Metadata["delivery_sequence"] != float64(2) {
				t.Fatalf("assistant reply = %+v, want a separate regular message after the status", reply)
			}

			duplicates, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, events, effects, 0)
			if err != nil {
				t.Fatalf("replay projected events: %v", err)
			}
			for index, result := range duplicates {
				if !result.AlreadyProjected {
					t.Fatalf("replayed event %d was not idempotent: %+v", index, result)
				}
			}
			cursor, err := repo.GetAgentDeliveryCursor(ctx, copilotNoticeStreamID)
			if err != nil {
				t.Fatalf("read delivery cursor: %v", err)
			}
			if cursor.ReceivedSequence != 2 || cursor.ProjectedSequence != 2 {
				t.Fatalf("cursor = %+v, want both sequences committed", cursor)
			}

			if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(`
			UPDATE agent_delivery_inbox SET payload = '{}', payload_pruned_at = ?
			WHERE stream_id = ? AND sequence = 1`), repo.nowUTC(), copilotNoticeStreamID); err != nil {
				t.Fatalf("simulate retained-event payload pruning: %v", err)
			}
			retained, err := repo.GetMessage(ctx, copilotNoticeStatusMessageID)
			if err != nil || retained.Content != copilotNoticeWarning ||
				retained.Metadata[streams.AgentEventDataCopilotInterruptionNotice] != streams.CopilotInterruptionClarification {
				t.Fatalf("status after event payload pruning = %+v, err %v", retained, err)
			}
		})
	}
}

func TestCopilotInterruptionWithoutOwnedTurnProjectsAsOrdinaryText(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedCombinedDeliveryBatchSession(t, repo)
	event := copilotNoticeDeliveryEvent(t, 1, "copilot-unowned-notice", copilotNoticeWarning,
		streams.CopilotInterruptionClarification, "")
	effects := combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event})

	if _, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, effects, 0); err != nil {
		t.Fatalf("project notice without owned turn: %v", err)
	}
	message, err := repo.GetMessage(ctx, "copilot-unowned-notice")
	if err != nil {
		t.Fatalf("read projected text fallback: %v", err)
	}
	if message.Type != models.MessageTypeMessage || message.Content != copilotNoticeWarning ||
		message.Metadata[streams.AgentEventDataCopilotInterruptionNotice] != nil {
		t.Fatalf("unowned notice = %+v, want original warning as an ordinary message", message)
	}
}

func copilotNoticeDeliveryEvent(
	t *testing.T,
	sequence int64,
	messageID, content, classification, turnID string,
) *models.AgentDeliveryEvent {
	t.Helper()
	data := map[string]any(nil)
	if classification != "" {
		data = map[string]any{streams.AgentEventDataCopilotInterruptionNotice: classification}
	}
	payload, err := json.Marshal(streams.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: content,
		TurnID:                 turnID,
		CanonicalMessageBaseID: messageID, CanonicalMessageID: messageID,
		PromptGeneration: 17, Data: data,
	})
	if err != nil {
		t.Fatalf("encode Copilot delivery event: %v", err)
	}
	event := combinedDeliveryBatchEvent(t, sequence, content)
	event.StreamID = copilotNoticeStreamID
	event.Payload = payload
	event.ReceivedAt = event.ReceivedAt.UTC()
	return event
}
