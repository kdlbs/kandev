package orchestrator

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

type responseAttemptMessageServiceStub struct {
	deleted []string
	fail    map[string]error
}

func (s *responseAttemptMessageServiceStub) DeleteMessage(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return s.fail[id]
}

func responseAttemptResetPayload(generation uint64, messageIDs ...string) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID:      "task-1",
		SessionID:   "session-1",
		ExecutionID: "exec-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:                streams.EventTypeResponseAttemptReset,
			PromptGeneration:    generation,
			RetractedMessageIDs: messageIDs,
		},
	}
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27
func TestHandleResponseAttemptResetDeletesEveryRecord(t *testing.T) {
	messageService := &responseAttemptMessageServiceStub{}
	agentManager := &mockAgentManager{currentPromptExecutionID: "exec-1"}
	agentManager.currentPromptGeneration.Store(7)
	svc := &Service{logger: testLogger(), agentManager: agentManager}
	svc.SetStreamingMessageRetractionService(messageService)

	svc.handleAgentStreamEvent(context.Background(), responseAttemptResetPayload(
		7, "assistant-1", "thinking-1",
	))

	if want := []string{"assistant-1", "thinking-1"}; !reflect.DeepEqual(messageService.deleted, want) {
		t.Fatalf("deleted IDs = %v, want %v", messageService.deleted, want)
	}
}

func TestHandleResponseAttemptResetContinuesAfterDeleteFailure(t *testing.T) {
	deleteErr := errors.New("delete failed")
	messageService := &responseAttemptMessageServiceStub{
		fail: map[string]error{"assistant-1": deleteErr},
	}
	agentManager := &mockAgentManager{currentPromptExecutionID: "exec-1"}
	agentManager.currentPromptGeneration.Store(7)
	log, observed := observingTestLogger(t)
	svc := &Service{logger: log, agentManager: agentManager}
	svc.SetStreamingMessageRetractionService(messageService)

	svc.handleAgentStreamEvent(context.Background(), responseAttemptResetPayload(
		7, "assistant-1", "thinking-1",
	))

	if want := []string{"assistant-1", "thinking-1"}; !reflect.DeepEqual(messageService.deleted, want) {
		t.Fatalf("delete attempts = %v, want %v", messageService.deleted, want)
	}
	if got := observed.FilterMessage("failed to retract abandoned response message").Len(); got != 1 {
		t.Fatalf("delete failure warnings = %d, want 1", got)
	}
}

func TestHandleResponseAttemptResetRejectsInactiveOwnership(t *testing.T) {
	for _, test := range []struct {
		name       string
		generation uint64
		current    uint64
		completed  bool
	}{
		{name: "zero generation", generation: 0, current: 7},
		{name: "stale generation", generation: 6, current: 7},
		{name: "completed execution", generation: 7, current: 7, completed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			messageService := &responseAttemptMessageServiceStub{}
			agentManager := &mockAgentManager{currentPromptExecutionID: "exec-1"}
			agentManager.currentPromptGeneration.Store(test.current)
			svc := &Service{logger: testLogger(), agentManager: agentManager}
			svc.SetStreamingMessageRetractionService(messageService)
			if test.completed {
				svc.markExecutionCompleted("session-1", "exec-1")
			}

			svc.handleAgentStreamEvent(
				context.Background(),
				responseAttemptResetPayload(test.generation, "assistant-1"),
			)

			if len(messageService.deleted) != 0 {
				t.Fatalf("inactive reset deleted messages: %v", messageService.deleted)
			}
		})
	}

	svc := &Service{logger: testLogger()}
	messageService := &responseAttemptMessageServiceStub{}
	svc.SetStreamingMessageRetractionService(messageService)
	svc.handleAgentStreamEvent(context.Background(), responseAttemptResetPayload(7, "assistant-1"))
	if len(messageService.deleted) != 0 {
		t.Fatalf("reset without a generation owner deleted messages: %v", messageService.deleted)
	}
}

func TestHandleResponseAttemptResetDeletesThroughTaskService(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "session-1", "step-1")
	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-1", TaskSessionID: "session-1", TaskID: "task-1",
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	for _, message := range []*models.Message{
		{
			ID:            "assistant-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			TurnID:        "turn-1",
			AuthorType:    models.MessageAuthorAgent,
			Content:       "abandoned answer",
			Type:          models.MessageTypeMessage,
		},
		{
			ID:            "thinking-1",
			TaskSessionID: "session-1",
			TaskID:        "task-1",
			TurnID:        "turn-1",
			AuthorType:    models.MessageAuthorAgent,
			Content:       "abandoned reasoning",
			Type:          models.MessageTypeThinking,
		},
	} {
		if err := repo.CreateMessage(ctx, message); err != nil {
			t.Fatalf("create message %s: %v", message.ID, err)
		}
	}

	eventBus := &recordingEventBus{}
	taskServices := newTaskServiceStateRepository(repo, eventBus)
	agentManager := &mockAgentManager{currentPromptExecutionID: "exec-1"}
	agentManager.currentPromptGeneration.Store(7)
	svc := &Service{logger: testLogger(), agentManager: agentManager}
	svc.SetStreamingMessageRetractionService(taskServices.service)

	svc.handleAgentStreamEvent(ctx, responseAttemptResetPayload(
		7, "assistant-1", "thinking-1",
	))

	messages, err := repo.ListMessages(ctx, "session-1")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("persisted messages after reset = %+v, want none", messages)
	}
	deletedEvents := 0
	for _, published := range eventBus.events {
		if published.subject == events.MessageDeleted {
			deletedEvents++
		}
	}
	if deletedEvents != 2 {
		t.Fatalf("message deletion events = %d, want 2", deletedEvents)
	}
}
