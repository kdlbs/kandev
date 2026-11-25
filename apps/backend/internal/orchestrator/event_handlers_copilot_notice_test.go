package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCopilotInterruptedToolStatusNotification(t *testing.T) {
	creator := &mockMessageCreator{}
	service := &Service{messageCreator: creator, logger: testLogger()}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:      "task-1",
		SessionID:   "session-1",
		ExecutionID: "execution-1",
		Data: &lifecycle.AgentStreamEventData{
			Type:             streams.EventTypeCopilotInterruptionNotice,
			Text:             "original warning text",
			PromptGeneration: 17,
			TurnID:           "turn-1",
			Data: map[string]any{
				streams.AgentEventDataCopilotInterruptionNotice: streams.CopilotInterruptionTool,
			},
		},
	}

	service.handleAgentStreamEvent(context.Background(), payload)
	service.handleAgentStreamEvent(context.Background(), payload)

	creator.mu.Lock()
	defer creator.mu.Unlock()
	if creator.sessionMessageAttempts != 2 || len(creator.sessionMessages) != 1 {
		t.Fatalf("attempts=%d persisted=%d, want two idempotent attempts and one message", creator.sessionMessageAttempts, len(creator.sessionMessages))
	}
	message := creator.sessionMessages[0]
	if message.id == "" || message.taskID != "task-1" || message.sessionID != "session-1" ||
		message.turnID != "turn-1" || message.content != "original warning text" ||
		message.messageType != string(v1.MessageTypeStatus) || message.requestsInput {
		t.Fatalf("persisted message = %+v, want an informational status in the current turn", message)
	}
	if message.metadata[streams.AgentEventDataCopilotInterruptionNotice] != streams.CopilotInterruptionTool {
		t.Fatalf("notice metadata = %#v, want generic tool classification", message.metadata)
	}
}

func TestCopilotInterruptionNoticeFromCompletedExecutionIsDropped(t *testing.T) {
	creator := &mockMessageCreator{}
	service := &Service{messageCreator: creator, logger: testLogger()}
	service.activeTurns.Store("session-1", "turn-successor")
	service.completedExecutions.Store(terminalExecutionKey("session-1", "execution-completed"), terminalExecutionMarker{
		expiresAt: time.Now().Add(time.Minute),
	})

	service.handleAgentStreamEvent(context.Background(), copilotNoticePayload(
		"execution-completed", "", "late warning",
	))
	service.handleAgentStreamEvent(context.Background(), copilotNoticePayload(
		"execution-successor", "turn-successor", "current warning",
	))

	creator.mu.Lock()
	defer creator.mu.Unlock()
	if creator.sessionMessageAttempts != 1 || len(creator.sessionMessages) != 1 {
		t.Fatalf("attempts=%d persisted=%+v, want only the current execution's notice", creator.sessionMessageAttempts, creator.sessionMessages)
	}
	message := creator.sessionMessages[0]
	if message.content != "current warning" || message.turnID != "turn-successor" {
		t.Fatalf("persisted message = %+v, want only the current warning on the successor turn", message)
	}
}

func TestCopilotInterruptionNoticeWithoutTurnOwnershipUsesTextFallback(t *testing.T) {
	creator := &mockMessageCreator{}
	service := &Service{messageCreator: creator, logger: testLogger()}
	service.activeTurns.Store("session-1", "turn-successor")
	payload := copilotNoticePayload("execution-1", "", "original warning text")
	payload.Data.MessageID = "ordinary-warning-message"

	service.handleAgentStreamEvent(context.Background(), payload)

	if creator.agentStreamWrites != 1 || len(creator.agentStreamTexts) != 1 ||
		creator.agentStreamTexts[0] != "original warning text" || creator.sessionMessageAttempts != 0 {
		t.Fatalf("stream writes=%d texts=%v status writes=%d, want the original warning through ordinary text delivery",
			creator.agentStreamWrites, creator.agentStreamTexts, creator.sessionMessageAttempts)
	}
}

func copilotNoticePayload(executionID, turnID, content string) *lifecycle.AgentStreamEventPayload {
	return &lifecycle.AgentStreamEventPayload{
		TaskID: "task-1", SessionID: "session-1", ExecutionID: executionID,
		Data: &lifecycle.AgentStreamEventData{
			Type: streams.EventTypeCopilotInterruptionNotice, Text: content,
			MessageID: "copilot-notice-message", PromptGeneration: 17, TurnID: turnID,
			Data: map[string]any{
				streams.AgentEventDataCopilotInterruptionNotice: streams.CopilotInterruptionTool,
			},
		},
	}
}
