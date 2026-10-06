package lifecycle

import (
	"encoding/json"
	"reflect"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.27
func TestHandleResponseAttemptResetRetractsCurrentAttemptRecords(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt: %v", err)
	}
	mgr.executionStore.MarkPromptDispatched(execution.ID, generation)
	t.Cleanup(func() { mgr.closeStreamCoalescer(execution) })

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              streams.EventTypeMessageChunk,
		Text:              "abandoned answer",
		ProtocolMessageID: "assistant-attempt-1",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              streams.EventTypeReasoning,
		ReasoningText:     "abandoned reasoning",
		ProtocolMessageID: "thinking-attempt-1",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             "response_attempt_reset",
		PromptGeneration: generation,
	})

	streamed := eventBus.getStreamEvents()
	wantTypes := []string{"message_chunk", "message_streaming", "reasoning", thinkingStreamingEventType, "response_attempt_reset"}
	if len(streamed) != len(wantTypes) {
		t.Fatalf("streamed events = %+v, want original assistant/reasoning evidence, their projections, then reset", streamed)
	}
	for i, want := range wantTypes {
		if streamed[i].Data.Type != want {
			t.Fatalf("stream event types = %+v, want %v", streamed, wantTypes)
		}
	}
	if streamed[0].Data.Text != "abandoned answer" || streamed[2].Data.Text != "abandoned reasoning" {
		t.Fatalf("original evidence = (%+v, %+v), want exact assistant and reasoning text", streamed[0].Data, streamed[2].Data)
	}
	wantIDs := []string{streamed[1].Data.MessageID, streamed[3].Data.MessageID}
	resets := streamEventsOfType(eventBus, "response_attempt_reset")
	if len(resets) != 1 {
		t.Fatalf("reset events = %+v, want one", resets)
	}
	encoded, err := json.Marshal(resets[0].Data)
	if err != nil {
		t.Fatalf("marshal reset data: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("decode reset data: %v", err)
	}
	rawIDs, _ := wire["retracted_message_ids"].([]any)
	gotIDs := make([]string, 0, len(rawIDs))
	for _, rawID := range rawIDs {
		if id, ok := rawID.(string); ok {
			gotIDs = append(gotIDs, id)
		}
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("retracted IDs = %v, want allocation order %v", gotIDs, wantIDs)
	}

	execution.messageMu.Lock()
	messageMap := execution.protocolMessageIDs
	thinkingMap := execution.protocolThinkingIDs
	history := execution.assistantHistoryBuffer.String()
	execution.messageMu.Unlock()
	if messageMap != nil || thinkingMap != nil || history != "" {
		t.Fatalf("abandoned stream state remains: messages=%v thinking=%v history=%q", messageMap, thinkingMap, history)
	}
	if evidence := execution.promptAttemptEvidenceSnapshot(); !evidence.OutputObserved || !evidence.EffectObserved {
		t.Fatalf("reset relaxed prompt replay evidence: %+v", evidence)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             "response_attempt_reset",
		PromptGeneration: generation,
	})
	resets = streamEventsOfType(eventBus, "response_attempt_reset")
	if len(resets) != 2 || len(resets[1].Data.RetractedMessageIDs) != 0 {
		t.Fatalf("repeated reset = %+v, want one empty no-op boundary", resets)
	}
}

// @covers AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.28
func TestResponseAttemptResetRequiresDispatchedPrompt(t *testing.T) {
	for _, test := range []struct {
		name      string
		completed bool
	}{
		{name: "admitted but not dispatched"},
		{name: "already completed", completed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			mgr, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("exec-1", "task-1", "session-1")
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}
			generation, err := mgr.executionStore.BeginPrompt(execution.ID)
			if err != nil {
				t.Fatalf("begin prompt: %v", err)
			}
			if test.completed {
				mgr.executionStore.MarkPromptDispatched(execution.ID, generation)
				if err := mgr.executionStore.WithLock(execution.ID, func(current *AgentExecution) {
					current.promptCompletionGeneration = generation
				}); err != nil {
					t.Fatalf("complete prompt: %v", err)
				}
			}

			mgr.handleAgentEvent(execution, agentctl.AgentEvent{
				Type:             "response_attempt_reset",
				PromptGeneration: generation,
			})

			if resets := streamEventsOfType(eventBus, "response_attempt_reset"); len(resets) != 0 {
				t.Fatalf("inactive prompt published reset: %+v", resets)
			}
		})
	}
}

func TestResponseAttemptRecordsIncludeLegacyStreams(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt: %v", err)
	}
	mgr.executionStore.MarkPromptDispatched(execution.ID, generation)
	t.Cleanup(func() { mgr.closeStreamCoalescer(execution) })

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk,
		Text: "legacy answer\n",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:          streams.EventTypeReasoning,
		ReasoningText: "legacy reasoning\n",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             streams.EventTypeResponseAttemptReset,
		PromptGeneration: generation,
	})

	events := eventBus.getStreamEvents()
	wantTypes := []string{"message_chunk", "message_streaming", "reasoning", thinkingStreamingEventType, "response_attempt_reset"}
	if len(events) != len(wantTypes) {
		t.Fatalf("events = %+v, want original legacy evidence, transcript projections, then reset", events)
	}
	for i, want := range wantTypes {
		if events[i].Data.Type != want {
			t.Fatalf("stream event types = %+v, want %v", events, wantTypes)
		}
	}
	if events[0].Data.Text != "legacy answer\n" || events[2].Data.Text != "legacy reasoning\n" {
		t.Fatalf("original evidence = (%+v, %+v), want exact legacy assistant and reasoning text", events[0].Data, events[2].Data)
	}
	wantIDs := []string{events[1].Data.MessageID, events[3].Data.MessageID}
	if got := events[4].Data.RetractedMessageIDs; !reflect.DeepEqual(got, wantIDs) {
		t.Fatalf("legacy retracted IDs = %v, want %v", got, wantIDs)
	}
}

func TestResponseAttemptResetPreservesCommittedProtocolRecord(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt: %v", err)
	}
	mgr.executionStore.MarkPromptDispatched(execution.ID, generation)
	t.Cleanup(func() { mgr.closeStreamCoalescer(execution) })

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              streams.EventTypeMessageChunk,
		Text:              "committed answer",
		ProtocolMessageID: "shared-protocol-id",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:       streams.EventTypeToolCall,
		ToolCallID: "tool-1",
		ToolName:   "read_file",
	})
	committedEvents := streamEventsOfType(eventBus, "message_streaming")
	if len(committedEvents) != 1 {
		t.Fatalf("committed message events = %+v, want one", committedEvents)
	}
	committedID := committedEvents[0].Data.MessageID

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              streams.EventTypeMessageChunk,
		Text:              " abandoned append",
		ProtocolMessageID: "shared-protocol-id",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:              streams.EventTypeMessageChunk,
		Text:              "abandoned new record",
		ProtocolMessageID: "new-protocol-id",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             streams.EventTypeResponseAttemptReset,
		PromptGeneration: generation,
	})

	resets := streamEventsOfType(eventBus, streams.EventTypeResponseAttemptReset)
	if len(resets) != 1 || len(resets[0].Data.RetractedMessageIDs) != 1 {
		t.Fatalf("reset = %+v, want only the newly allocated record", resets)
	}
	if resets[0].Data.RetractedMessageIDs[0] == committedID {
		t.Fatal("protocol ID reuse made a committed record deletion-eligible")
	}
	execution.messageMu.Lock()
	retainedID := execution.protocolMessageIDs["shared-protocol-id"]
	_, newRetained := execution.protocolMessageIDs["new-protocol-id"]
	execution.messageMu.Unlock()
	if retainedID != committedID || newRetained {
		t.Fatalf(
			"protocol correlations after reset: shared=%q new_present=%t, want shared=%q new_present=false",
			retainedID,
			newRetained,
			committedID,
		)
	}
}

func TestResponseAttemptResetRejectsStaleGenerationAndExecution(t *testing.T) {
	t.Run("stale generation", func(t *testing.T) {
		mgr, eventBus := createTestManagerWithTracking()
		execution := createTestExecution("exec-1", "task-1", "session-1")
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatalf("add execution: %v", err)
		}
		first, err := mgr.executionStore.BeginPrompt(execution.ID)
		if err != nil {
			t.Fatalf("begin first prompt: %v", err)
		}
		second, err := mgr.executionStore.BeginPrompt(execution.ID)
		if err != nil {
			t.Fatalf("begin second prompt: %v", err)
		}
		mgr.executionStore.MarkPromptDispatched(execution.ID, second)
		t.Cleanup(func() { mgr.closeStreamCoalescer(execution) })
		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type:              streams.EventTypeMessageChunk,
			Text:              "replacement",
			ProtocolMessageID: "replacement-id",
		})

		mgr.handleAgentEvent(execution, agentctl.AgentEvent{
			Type:             streams.EventTypeResponseAttemptReset,
			PromptGeneration: first,
		})

		if resets := streamEventsOfType(eventBus, streams.EventTypeResponseAttemptReset); len(resets) != 0 {
			t.Fatalf("stale generation published reset: %+v", resets)
		}
		execution.messageMu.Lock()
		retained := execution.protocolMessageIDs["replacement-id"]
		execution.messageMu.Unlock()
		if retained == "" {
			t.Fatal("stale generation detached replacement record")
		}
	})

	t.Run("stale execution", func(t *testing.T) {
		mgr, eventBus := createTestManagerWithTracking()
		stale := createTestExecution("exec-old", "task-1", "session-1")
		if err := mgr.executionStore.Add(stale); err != nil {
			t.Fatalf("add stale execution: %v", err)
		}
		generation, err := mgr.executionStore.BeginPrompt(stale.ID)
		if err != nil {
			t.Fatalf("begin prompt: %v", err)
		}
		mgr.executionStore.MarkPromptDispatched(stale.ID, generation)
		t.Cleanup(func() { mgr.closeStreamCoalescer(stale) })
		mgr.handleAgentEvent(stale, agentctl.AgentEvent{
			Type:              streams.EventTypeMessageChunk,
			Text:              "old execution output",
			ProtocolMessageID: "old-id",
		})

		mgr.executionStore.Remove(stale.ID)
		replacement := createTestExecution("exec-new", "task-1", "session-1")
		if err := mgr.executionStore.Add(replacement); err != nil {
			t.Fatalf("add replacement execution: %v", err)
		}
		mgr.handleAgentEvent(stale, agentctl.AgentEvent{
			Type:             streams.EventTypeResponseAttemptReset,
			PromptGeneration: generation,
		})

		if resets := streamEventsOfType(eventBus, streams.EventTypeResponseAttemptReset); len(resets) != 0 {
			t.Fatalf("stale execution published reset: %+v", resets)
		}
		stale.messageMu.Lock()
		retained := stale.protocolMessageIDs["old-id"]
		stale.messageMu.Unlock()
		if retained == "" {
			t.Fatal("stale execution detached its stream state")
		}
	})
}
