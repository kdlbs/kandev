package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-001.4 AC-PLATFORM-DELIVERY-RETENTION-002.3
func TestDeliveryRetentionIntegrationHistoricalCanonicalPayloads(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, missing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/missing=%t", engine, missing), func(t *testing.T) {
				repo, terminal, advance := seedRetentionPromptSubmission(t, engine, "historical")
				events := seedHistoricalCanonicalOutputs(t, repo, terminal)
				if missing {
					if _, err := repo.db.Exec(repo.db.Rebind(`DELETE FROM agent_delivery_submissions WHERE id = ?`), terminal.SubmissionID); err != nil {
						t.Fatal(err)
					}
				}
				before := retentionCanonicalContent(t, repo, terminal.SessionID)
				advance(26 * time.Hour)
				preview, err := repo.AnalyzeAgentDeliveryRetention(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				if err != nil || preview.EventPayloadCandidates != 2 {
					t.Fatalf("preview must find independent output behind protected terminal: %+v %v", preview, err)
				}
				result, err := repo.RunAgentDeliveryRetentionBatch(t.Context(), models.AgentDeliveryRetentionBatchOptions{})
				if err != nil || result.EventPayloadsPruned != 2 || result.EventRowsRetired != 0 || result.PromptPayloadsPruned != 0 {
					t.Fatalf("independent bodies must compact without retiring protected evidence: %+v %v", result, err)
				}
				assertHistoricalRetentionEvidence(t, repo, terminal, events, missing)
				if got := retentionCanonicalContent(t, repo, terminal.SessionID); got != before {
					t.Fatal("canonical conversation changed")
				}
			})
		}
	}
}

func seedHistoricalCanonicalOutputs(t *testing.T, repo *Repository, terminal models.AgentDeliveryEvent) []models.AgentDeliveryEvent {
	t.Helper()
	var events []models.AgentDeliveryEvent
	for sequence := int64(2); sequence <= 5; sequence++ {
		event := terminal
		event.Sequence, event.Terminal, event.EventType = sequence, false, streams.EventTypeMessageChunk
		var err error
		event.Payload, err = json.Marshal(streams.AgentEvent{Type: event.EventType, Text: fmt.Sprintf("canonical historical output %d", sequence)})
		if err != nil {
			t.Fatal(err)
		}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(t.Context(), &event, sequence); err != nil || !inserted {
			t.Fatalf("receive historical output: %v %v", inserted, err)
		}
		effect := completedRetentionEventEffect(&event)
		if sequence == 5 {
			effect.EffectKey = "unrelated-completed-effect"
		}
		if _, err := repo.ProjectCanonicalAgentDeliveryEvent(t.Context(), &event, effect); err != nil {
			t.Fatal(err)
		}
		if sequence == 4 {
			pending := &models.AgentDeliveryEffect{EffectKey: "pending-historical-effect", StreamID: event.StreamID, Sequence: sequence,
				EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectPending, CreatedAt: event.ReceivedAt}
			if _, err := repo.PutAgentDeliveryEffect(t.Context(), pending); err != nil {
				t.Fatal(err)
			}
		}
		events = append(events, event)
	}
	return events
}

func assertHistoricalRetentionEvidence(t *testing.T, repo *Repository, terminal models.AgentDeliveryEvent, events []models.AgentDeliveryEvent, missing bool) {
	t.Helper()
	ctx := context.Background()
	var terminalBytes int
	if err := repo.db.QueryRowx(repo.db.Rebind(`SELECT LENGTH(payload) FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = 1`), terminal.StreamID).Scan(&terminalBytes); err != nil || terminalBytes != len(terminal.Payload) {
		t.Fatalf("protected terminal changed: bytes=%d err=%v", terminalBytes, err)
	}
	for _, event := range events {
		var bytes int
		if err := repo.db.QueryRowx(repo.db.Rebind(`SELECT LENGTH(payload) FROM agent_delivery_inbox WHERE stream_id = ? AND sequence = ?`), event.StreamID, event.Sequence).Scan(&bytes); err != nil {
			t.Fatal(err)
		}
		if event.Sequence >= 4 {
			if bytes != len(event.Payload) {
				t.Fatalf("unproved event %d lost its body", event.Sequence)
			}
			continue
		}
		if bytes != 0 {
			t.Fatalf("redundant body %d retained %d bytes", event.Sequence, bytes)
		}
		if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 5); err != nil || inserted {
			t.Fatalf("identical replay must remain duplicate: %v %v", inserted, err)
		}
		event.Payload = []byte(`{"type":"message_chunk","text":"changed replay"}`)
		if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &event, 5); err == nil || inserted {
			t.Fatalf("conflicting replay accepted: %v %v", inserted, err)
		}
	}
	cursor, err := repo.GetAgentDeliveryCursor(ctx, terminal.StreamID)
	if err != nil || cursor.ProjectedSequence != 5 || cursor.RetiredSequence != 0 {
		t.Fatalf("protected cursor changed: %+v %v", cursor, err)
	}
	if !missing {
		prompt, err := repo.GetAgentDeliverySubmission(ctx, terminal.SubmissionID)
		if err != nil || prompt.FullySettledAt != nil || len(prompt.Payload) == 0 {
			t.Fatalf("unsettled prompt changed: %+v %v", prompt, err)
		}
	}
}
