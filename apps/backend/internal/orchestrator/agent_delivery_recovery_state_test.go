package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestAgentctlDisconnectPersistsRecoveryAndPublishesWithoutSettlingSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-recovery-view", "session-delivery-recovery-view", "step-1")
	seedExecutorRunning(t, repo, "session-delivery-recovery-view", "task-delivery-recovery-view", "exec-delivery-recovery-view")
	session, err := repo.GetTaskSession(ctx, "session-delivery-recovery-view")
	if err != nil {
		t.Fatal(err)
	}
	incarnationID := session.QueueIncarnationID
	if err := repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: incarnationID, Generation: 3,
		NativeSessionID: "native-recovery-view", CreationReason: "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError,
		models.LastAgentError{Message: "independent prior error", OccurredAt: time.Now().UTC(), Code: "OTHER"}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "submission-recovery-view", SessionID: session.ID, IncarnationID: incarnationID,
		HarnessGeneration: 3, OwnerGeneration: 1, PayloadHash: "hash-recovery-view",
		Payload: []byte("prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	}); err != nil {
		t.Fatal(err)
	}
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	stateEvents := make(chan map[string]interface{}, 2)
	if _, err := eventBus.Subscribe(events.TaskSessionStateChanged, func(_ context.Context, event *bus.Event) error {
		if data, ok := event.Data.(map[string]interface{}); ok {
			stateEvents <- data
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	service.eventBus = eventBus
	payload := lifecycle.AgentctlEventPayload{
		TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: session.AgentExecutionID,
		FailureCode: "DURABLE_DELIVERY_RECONNECTING", DeliveryRecoveryPhase: models.AgentDeliveryRecoveryReconnecting,
		DeliverySubmissionID: "submission-recovery-view", DeliveryStreamID: "stream-recovery-view",
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 3, PromptGeneration: 9,
		ErrorMessage: "reconnecting to the agent delivery stream",
	}
	service.handleAgentctlDeliveryRecovery(ctx, payload)

	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	recovery, ok := models.LoadAgentDeliveryRecovery(stored.Metadata)
	if !ok || recovery.Phase != models.AgentDeliveryRecoveryReconnecting || recovery.Revision != 1 ||
		recovery.SubmissionID != "submission-recovery-view" || recovery.PromptGeneration != 9 {
		t.Fatalf("persisted recovery = %+v, ok=%v", recovery, ok)
	}
	lastError, ok := models.LoadLastAgentError(stored.Metadata)
	if !ok || lastError.Code != "OTHER" {
		t.Fatalf("independent last error = %+v, ok=%v", lastError, ok)
	}
	if stored.State != models.TaskSessionStateRunning {
		t.Fatalf("coarse session state = %s, want RUNNING while outcome is unresolved", stored.State)
	}
	block, err := service.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil || block == nil || block.DeliverySubmissionID != "submission-recovery-view" {
		t.Fatalf("delivery admission block = %+v, %v", block, err)
	}
	select {
	case event := <-stateEvents:
		if event["old_state"] != string(models.TaskSessionStateRunning) ||
			event["new_state"] != string(models.TaskSessionStateRunning) {
			t.Fatalf("recovery notification settled the coarse state: %#v", event)
		}
		metadata, ok := event["session_metadata"].(map[string]interface{})
		if !ok || metadata[models.SessionMetaKeyAgentDeliveryRecovery] == nil {
			t.Fatalf("state event did not carry durable recovery metadata: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("persisted recovery state was not published through the session state event")
	}

	stale := lifecycle.AgentctlEventPayload{
		TaskID: session.TaskID, SessionID: session.ID, AgentExecutionID: "exec-replaced",
		DeliveryRecoveryPhase: models.AgentDeliveryRecoveryUncertain,
		DeliverySubmissionID:  "submission-old", DeliveryStreamID: "stream-old",
		DeliveryIncarnationID: incarnationID, DeliveryHarnessGeneration: 3, PromptGeneration: 10,
	}
	service.handleAgentctlDeliveryRecovery(ctx, stale)
	afterStale, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	currentRecovery, ok := models.LoadAgentDeliveryRecovery(afterStale.Metadata)
	if !ok || currentRecovery.SubmissionID != "submission-recovery-view" || currentRecovery.Revision != 1 {
		t.Fatalf("stale event changed recovery notice: %+v, ok=%v", currentRecovery, ok)
	}

	payload.DeliveryRecoveryPhase = models.AgentDeliveryRecoveryRecovered
	payload.ErrorMessage = "the original agent delivery stream is reattached"
	service.handleAgentctlDeliveryRecovery(ctx, payload)
	recoveredSession, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	recovered, ok := models.LoadAgentDeliveryRecovery(recoveredSession.Metadata)
	if !ok || recovered.Phase != models.AgentDeliveryRecoveryRecovered || recovered.Revision != 2 {
		t.Fatalf("recovered delivery view = %+v, ok=%v", recovered, ok)
	}
	openBlock, err := service.GetOpenSessionRecoveryBlock(ctx, session.ID)
	if err != nil || openBlock != nil {
		t.Fatalf("exact reattachment left delivery recovery blocked: %+v, %v", openBlock, err)
	}
}
