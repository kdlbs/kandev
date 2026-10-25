package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

func TestDeliveryRetentionCompleteTerminalOutcomes(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, outcome := range []models.DeliverySubmissionState{models.DeliverySubmissionCompleted, models.DeliverySubmissionCancelled, models.DeliverySubmissionFailed} {
			t.Run(engine+"/"+string(outcome), func(t *testing.T) {
				repo, terminal, advance := seedRetentionPromptOutcome(t, engine, string(outcome), outcome)
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
				successor.Payload, _ = json.Marshal(streams.AgentEvent{Type: successor.EventType, Text: "output after settled terminal"})
				if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, &successor, 2); err != nil || !inserted {
					t.Fatalf("receive successor: %v %v", inserted, err)
				}
				if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, &successor, completedRetentionEventEffect(&successor)); err != nil {
					t.Fatal(err)
				}
				before := retentionCanonicalContent(t, repo, terminal.SessionID)
				advance(26 * time.Hour)
				for attempt := 0; attempt < 3; attempt++ {
					if _, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{}); err != nil {
						t.Fatal(err)
					}
				}
				cursor, err := repo.GetAgentDeliveryCursor(ctx, terminal.StreamID)
				if err != nil || cursor.RetiredSequence != 2 {
					t.Fatalf("settled %s frontier: %+v %v; want 2", outcome, cursor, err)
				}
				if retentionCanonicalContent(t, repo, terminal.SessionID) != before {
					t.Fatal("terminal retirement changed canonical history")
				}
				summary, err := repo.GetAgentDeliveryEffect(ctx, terminalRetentionEffectKey(terminal))
				if err != nil || summary.Outcome != string(outcome) || summary.State != models.DeliveryEffectCompleted {
					t.Fatalf("compact outcome changed: %+v %v", summary, err)
				}
			})
		}
	}
}
