package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-PLATFORM-DELIVERY-RETENTION-001.1 AC-PLATFORM-DELIVERY-RETENTION-001.2
func TestDeliveryRetentionIntegrationActiveCanonicalGrace(t *testing.T) {
	for _, engine := range []string{"SQLite", "Postgres"} {
		for _, state := range []models.DeliverySubmissionState{models.DeliverySubmissionDispatching, models.DeliverySubmissionInterruptedUnknown} {
			t.Run(engine+"/"+string(state), func(t *testing.T) {
				repo := newRetentionTestRepo(t, engine)
				ctx := context.Background()
				now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
				repo.clockNow = func() time.Time { return now }
				if err := repo.CreateTask(ctx, &models.Task{ID: "active-retention-task", WorkspaceID: retentionTestWorkspaceID(t, repo), Title: "Active canonical grace"}); err != nil {
					t.Fatal(err)
				}
				if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "active-retention-session", TaskID: "active-retention-task", QueueIncarnationID: "active-retention-incarnation"}); err != nil {
					t.Fatal(err)
				}
				submission := &models.AgentDeliverySubmission{ID: "prompt:active-retention", SessionID: "active-retention-session", IncarnationID: "active-retention-incarnation", HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "active-prompt-hash", Payload: []byte("still-protected prompt"), State: state}
				if _, err := repo.PrepareAgentDeliverySubmission(ctx, submission); err != nil {
					t.Fatal(err)
				}
				payload, err := json.Marshal(streams.AgentEvent{Type: streams.EventTypeMessageChunk, Text: "canonical output already committed"})
				if err != nil {
					t.Fatal(err)
				}
				event := &models.AgentDeliveryEvent{SessionID: submission.SessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: 1, SubmissionID: submission.ID, StreamID: "active-retention-stream", Sequence: 1, EventType: streams.EventTypeMessageChunk, Payload: payload, ReceivedAt: now}
				if inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1); err != nil || !inserted {
					t.Fatalf("receive: %v %v", inserted, err)
				}
				if _, err := repo.ProjectCanonicalAgentDeliveryEvent(ctx, event, completedRetentionEventEffect(event)); err != nil {
					t.Fatal(err)
				}
				before := retentionCanonicalContent(t, repo, submission.SessionID)
				now = now.Add(2 * time.Hour)
				result, err := repo.RunAgentDeliveryRetentionBatch(ctx, models.AgentDeliveryRetentionBatchOptions{})
				if err != nil || result.EventPayloadsPruned != 1 || result.EventRowsRetired != 0 || result.PromptPayloadsPruned != 0 {
					t.Fatalf("redundant canonical output should compact while prompt remains protected: %+v %v", result, err)
				}
				protected, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
				if err != nil || protected.State != state || string(protected.Payload) != string(submission.Payload) || protected.FullySettledAt != nil {
					t.Fatalf("active/uncertain prompt evidence changed: %+v %v", protected, err)
				}
				if got := retentionCanonicalContent(t, repo, submission.SessionID); got != before {
					t.Fatal("canonical output changed")
				}
			})
		}
	}
}
