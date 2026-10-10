package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

func TestTerminalSettlementPublishesWithoutRuntimeRecoveryRecord(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "task-terminal-view", "session-terminal-view", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-terminal-view")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1,
		NativeSessionID: "native-terminal-view", CreationReason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "terminal-view", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("prompt"), State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "terminal-view-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "terminal-view",
	}); err != nil {
		t.Fatal(err)
	}
	eventBus := &recordingEventBus{}
	service.eventBus = eventBus
	event := persistedTerminalEvent(session.ID, session.QueueIncarnationID, 1, "terminal-view", "terminal-view-stream", "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	if err := service.reconcileAgentDeliverySettlements(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	for _, published := range eventBus.events {
		if published.subject == events.TaskSessionStateChanged {
			data := published.event.Data.(map[string]interface{})
			blocks, ok := data["session_recovery_blocks"].([]dto.SessionRecoveryBlockDTO)
			if !ok || blocks == nil || len(blocks) != 0 {
				t.Fatalf("terminal projection did not explicitly clear blocks: %+v", data)
			}
			return
		}
	}
	t.Fatal("terminal recovery block resolution was not published without runtime metadata")
}
