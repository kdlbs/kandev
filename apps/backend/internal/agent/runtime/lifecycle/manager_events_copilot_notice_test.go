package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCopilotInterruptionNoticeDoesNotRecordAssistantActivity(t *testing.T) {
	manager, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("execution-copilot-notice", "task-1", "session-1")
	execution.Status = v1.AgentStatusReady
	execution.promptGeneration = 4
	execution.promptTurnIDs = map[uint64]string{4: "turn-origin"}
	if err := manager.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	manager.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk, Text: "interrupted call warning", PromptGeneration: 4,
		Data: map[string]any{streams.AgentEventDataCopilotInterruptionNotice: streams.CopilotInterruptionTool},
	})

	if execution.Status != v1.AgentStatusReady {
		t.Fatalf("execution status = %q, want the notice to leave foreground activity unchanged", execution.Status)
	}
	if execution.messageBuffer.Len() != 0 || execution.assistantHistoryBuffer.Len() != 0 {
		t.Fatalf("notice entered assistant buffers: message=%q history=%q", execution.messageBuffer.String(), execution.assistantHistoryBuffer.String())
	}
	if execution.promptCompletionGeneration != 0 {
		t.Fatalf("prompt completion generation = %d, want no completion accounting", execution.promptCompletionGeneration)
	}
	events := eventBus.getStreamEvents()
	if len(events) != 1 || events[0].Data.Type != streams.EventTypeCopilotInterruptionNotice {
		t.Fatalf("published notice events = %+v", events)
	}
	if events[0].Data.TurnID != "turn-origin" {
		t.Fatalf("notice turn ID = %q, want the turn owned by prompt generation 4", events[0].Data.TurnID)
	}
}
