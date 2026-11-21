package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

func TestConfigurationCommandCompletionRequiresUserMessage(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turn := &models.Turn{
		ID: "turn-system-created", TaskSessionID: sessionID, TaskID: "task-123", StartedAt: time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	eventBus.ClearEvents()
	result := &streams.ConfigurationCommandResult{
		Status: "applied", ConfigID: "model", Requested: "model-requested", Effective: "model-effective", Confirmed: true,
	}
	if err := svc.CompleteTurnWithConfigurationResult(ctx, turn.ID, result); err != nil {
		t.Fatalf("CompleteTurnWithConfigurationResult: %v", err)
	}
	completed := lastTurnCompletedData(t, eventBus)
	if _, exists := completed["configuration_result"]; exists {
		t.Fatalf("system-created turn carries configuration_result: %#v", completed["configuration_result"])
	}
	receipt, ok := completed["conversation_receipt"].(*models.ConversationMutationReceipt)
	if !ok || receipt == nil {
		t.Fatalf("conversation_receipt = %#v, want mutation receipt", completed["conversation_receipt"])
	}
	for _, operation := range receipt.Operations {
		if operation.Entity == models.ConversationEntityTurn && operation.ID == turn.ID && operation.ConfigurationCommandResult != nil {
			t.Fatalf("system-created turn receipt carries configuration result: %+v", operation.ConfigurationCommandResult)
		}
	}
}

func TestConfigurationCommandCompletionRequiresInitiatingSlashCommand(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turn := &models.Turn{
		ID: "turn-user-message", TaskSessionID: sessionID, TaskID: "task-123", StartedAt: time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	userMessage := &models.Message{
		ID: "message-normal-prompt", TaskSessionID: sessionID, TaskID: turn.TaskID,
		TurnID: turn.ID, AuthorType: models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: "Please change the model",
	}
	if err := repo.CreateMessage(ctx, userMessage); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}
	eventBus.ClearEvents()
	result := &streams.ConfigurationCommandResult{
		Status: "applied", ConfigID: "model", Requested: "Model B", Effective: "Model B", Confirmed: true,
	}
	if err := svc.CompleteTurnWithConfigurationResult(ctx, turn.ID, result); err != nil {
		t.Fatalf("CompleteTurnWithConfigurationResult: %v", err)
	}
	completed := lastTurnCompletedData(t, eventBus)
	if _, exists := completed["configuration_result"]; exists {
		t.Fatalf("non-command user turn carries configuration_result: %#v", completed["configuration_result"])
	}
}

func TestConfigurationCommandAcceptsProviderModelConfigID(t *testing.T) {
	message := &models.Message{
		AuthorType: models.MessageAuthorUser,
		Content:    "/model model-b",
	}
	result := &streams.ConfigurationCommandResult{
		Status: "applied", ConfigID: "primary_model", Requested: "Model B", Effective: "Model B", Confirmed: true,
	}
	if !isConfigurationCommandMessage(message, result) {
		t.Fatal("provider model config option ID was rejected for /model")
	}

	result.ConfigID = ""
	if isConfigurationCommandMessage(message, result) {
		t.Fatal("empty config option ID was accepted for /model")
	}
}

func TestRecoveredTurnCompletionOmitsConfigurationCommandReceipt(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)
	turn, err := svc.ReserveTurn(ctx, sessionID, &models.PromptDispatchRecovery{})
	if err != nil {
		t.Fatalf("ReserveTurn: %v", err)
	}
	if err := svc.MarkReservedTurnDispatchAttempted(ctx, turn); err != nil {
		t.Fatalf("MarkReservedTurnDispatchAttempted: %v", err)
	}
	if err := repo.CompleteTurn(ctx, turn.ID); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}
	eventBus.ClearEvents()
	if _, err := svc.ReconcileUnpublishedPromptTurns(ctx); err != nil {
		t.Fatalf("ReconcileUnpublishedPromptTurns: %v", err)
	}
	completed := lastTurnCompletedData(t, eventBus)
	if _, exists := completed["configuration_result"]; exists {
		t.Fatalf("recovered turn carries configuration_result: %#v", completed["configuration_result"])
	}
	receipt, ok := completed["conversation_receipt"].(*models.ConversationMutationReceipt)
	if ok && receipt != nil {
		for _, operation := range receipt.Operations {
			if operation.Entity == models.ConversationEntityTurn && operation.ConfigurationCommandResult != nil {
				t.Fatalf("recovered turn receipt carries configuration result: %+v", operation.ConfigurationCommandResult)
			}
		}
	}
}

func lastTurnCompletedData(t *testing.T, eventBus *MockEventBus) map[string]interface{} {
	t.Helper()
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.TurnCompleted {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("turn.completed data has type %T", event.Data)
		}
		return data
	}
	t.Fatal("turn.completed event was not published")
	return nil
}
