package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-001.1 AC-PLATFORM-DELIVERY-RETENTION-005.2
func TestDeliveryRetentionIntegration(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, terminal, advance := seedRetentionPromptSubmission(t, engine, "integrated")
			ctx := context.Background()
			if _, err := repo.PutAgentDeliveryEffect(ctx, completedRetentionEventEffect(&terminal)); err != nil {
				t.Fatal(err)
			}
			putRetentionWorkflowOutcome(t, repo, terminal, "not_required", retentionPromptTurnID(t, terminal))
			if complete, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(terminal), time.Time{}); err != nil || !complete {
				t.Fatalf("settlement: %v %v", complete, err)
			}
			successor := terminal
			successor.Sequence, successor.Terminal, successor.SubmissionID = 2, false, ""
			successor.EventType = streams.EventTypeMessageChunk
			successor.Payload, _ = json.Marshal(streams.AgentEvent{Type: successor.EventType, Text: "saved output after a settled turn"})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &successor, 2); err != nil || !inserted {
				t.Fatalf("receive successor: %v %v", inserted, err)
			}
			if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, &successor, completedRetentionEventEffect(&successor)); err != nil {
				t.Fatal(err)
			}
			tool := successor
			tool.Sequence, tool.EventType = 3, streams.EventTypeToolUpdate
			tool.Payload, _ = json.Marshal(streams.AgentEvent{Type: tool.EventType, Text: "saved tool result"})
			if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &tool, 3); err != nil || !inserted {
				t.Fatalf("receive tool update: %v %v", inserted, err)
			}
			if _, err := repo.ProjectAgentDeliveryEvent(ctx, &tool, completedRetentionEventEffect(&tool)); err != nil {
				t.Fatal(err)
			}
			before := retentionCanonicalContent(t, repo, terminal.SessionID)
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_inbox SET projected_at = ? WHERE stream_id = ?`), repo.nowUTC(), terminal.StreamID); err != nil {
				t.Fatal(err)
			}
			advance(26 * time.Hour)
			for attempt := 0; attempt < 3; attempt++ {
				if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			cursor, err := repo.GetAgentDeliveryCursor(ctx, terminal.StreamID)
			if err != nil || cursor.RetiredSequence != 3 {
				t.Fatalf("settled multi-turn frontier: %+v %v; want 3", cursor, err)
			}
			assertRetentionRowCount(t, repo, "agent_delivery_inbox", "stream_id = ?", terminal.StreamID, 0)
			if after := retentionCanonicalContent(t, repo, terminal.SessionID); after != before {
				t.Fatalf("canonical output changed: %q to %q", before, after)
			}
			for _, event := range []*models.AgentDeliveryEvent{&terminal, &successor, &tool} {
				if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 3); inserted || !errors.Is(err, repoerrors.ErrAgentDeliveryReplayExpired) {
					t.Fatalf("expired replay %d: %v %v", event.Sequence, inserted, err)
				}
			}
			summary, err := repo.GetAgentDeliveryEffect(ctx, terminalRetentionEffectKey(terminal))
			if err != nil || summary.State != models.DeliveryEffectCompleted {
				t.Fatalf("retained terminal summary: %+v %v", summary, err)
			}
			submission, err := repo.GetAgentDeliverySubmissionMetadata(ctx, terminal.SubmissionID)
			if err != nil || submission.PayloadPrunedAt == nil {
				t.Fatalf("prompt summary after event retirement: %+v %v", submission, err)
			}
		})
	}
}

func retentionCanonicalContent(t *testing.T, repo *Repository, sessionID string) string {
	t.Helper()
	rows, err := repo.db.Queryx(repo.db.Rebind(`SELECT content FROM task_session_messages WHERE task_session_id = ? ORDER BY id`), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var content string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		content += value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if content == "" {
		t.Fatal(fmt.Errorf("canonical positive control has no output"))
	}
	return content
}

func TestDeliveryRetentionIntegrationMismatchedTerminalSummary(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		t.Run(engine, func(t *testing.T) {
			repo, event, advance := seedRetentionPromptSubmission(t, engine, "mismatch")
			ctx := context.Background()
			if _, err := repo.PutAgentDeliveryEffect(ctx, completedRetentionEventEffect(&event)); err != nil {
				t.Fatal(err)
			}
			putRetentionWorkflowOutcome(t, repo, event, "not_required", retentionPromptTurnID(t, event))
			if complete, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx, terminalRetentionEffectKey(event), time.Time{}); err != nil || !complete {
				t.Fatalf("settle: %v %v", complete, err)
			}
			if _, err := repo.db.Exec(repo.db.Rebind(`UPDATE agent_delivery_submissions SET terminal_turn_id = ? WHERE id = ?`), "unproved-successor", event.SubmissionID); err != nil {
				t.Fatal(err)
			}
			advance(26 * time.Hour)
			result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if result.EventPayloadsPruned != 0 || result.EventRowsRetired != 0 || result.PromptPayloadsPruned != 0 {
				t.Fatalf("mismatched summary authorized pruning: %+v", result)
			}
		})
	}
}
