package adapter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/opencode"
)

func newTestOpenCodeAdapter() *OpenCodeAdapter {
	log, _ := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	cfg := &Config{
		WorkDir:     "/tmp/test",
		AutoApprove: true,
	}
	return NewOpenCodeAdapter(cfg, log)
}

// drainEvents reads events from the updates channel with a timeout
func drainEvents(adapter *OpenCodeAdapter, timeout time.Duration) []AgentEvent {
	var events []AgentEvent
	deadline := time.After(timeout)
	for {
		select {
		case evt := <-adapter.updatesCh:
			events = append(events, evt)
		case <-deadline:
			return events
		default:
			// Small sleep to avoid busy loop
			time.Sleep(10 * time.Millisecond)
			select {
			case evt := <-adapter.updatesCh:
				events = append(events, evt)
			case <-deadline:
				return events
			default:
				return events
			}
		}
	}
}

func TestOpenCodeAdapter_TextPartDeduplication(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	sessionID := "test-session"
	operationID := "test-op"

	// Track message role first (assistant message)
	adapter.mu.Lock()
	adapter.messageRoles = make(map[string]string)
	adapter.messageRoles["msg-1"] = "assistant"
	adapter.mu.Unlock()

	// Simulate streaming events with cumulative text
	// The key insight: part.Text is CUMULATIVE, not incremental
	events := []struct {
		partID string
		text   string // Cumulative text so far
		delta  string
	}{
		{partID: "part-1", text: "Hello", delta: "Hello"},
		{partID: "part-1", text: "Hello world", delta: " world"},
		{partID: "part-1", text: "Hello world!", delta: "!"},
	}

	for _, evt := range events {
		props := opencode.MessagePartUpdatedProperties{
			Part: opencode.Part{
				ID:        evt.partID,
				Type:      opencode.PartTypeText,
				MessageID: "msg-1",
				SessionID: sessionID,
				Text:      evt.text,
			},
			Delta: evt.delta,
		}
		propsJSON, _ := json.Marshal(props)
		adapter.handleMessagePartUpdated(propsJSON, sessionID, operationID)
	}

	// Collect sent events
	sentEvents := drainEvents(adapter, 100*time.Millisecond)

	// Should have received 3 events with incremental text
	if len(sentEvents) != 3 {
		t.Fatalf("expected 3 events, got %d", len(sentEvents))
	}

	// Verify text is incremental (computed from cumulative), not duplicated
	expectedTexts := []string{"Hello", " world", "!"}
	for i, evt := range sentEvents {
		if evt.Text != expectedTexts[i] {
			t.Errorf("event %d: expected text %q, got %q", i, expectedTexts[i], evt.Text)
		}
	}
}

func TestOpenCodeAdapter_TextDeduplicationWithRepeatedDeltas(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	sessionID := "test-session"
	operationID := "test-op"

	// Track message role
	adapter.mu.Lock()
	adapter.messageRoles = make(map[string]string)
	adapter.messageRoles["msg-1"] = "assistant"
	adapter.mu.Unlock()

	// Simulate the problematic case: same delta sent multiple times
	// This can happen when OpenCode sends multiple events with the same delta
	// Our fix: we prefer cumulative text over delta
	events := []struct {
		text  string
		delta string
	}{
		{text: "Done", delta: "Done"},
		{text: "Done", delta: "Done"}, // Duplicate event with same delta
		{text: "Done.", delta: "."},
	}

	for _, evt := range events {
		props := opencode.MessagePartUpdatedProperties{
			Part: opencode.Part{
				ID:        "part-1",
				Type:      opencode.PartTypeText,
				MessageID: "msg-1",
				SessionID: sessionID,
				Text:      evt.text,
			},
			Delta: evt.delta,
		}
		propsJSON, _ := json.Marshal(props)
		adapter.handleMessagePartUpdated(propsJSON, sessionID, operationID)
	}

	sentEvents := drainEvents(adapter, 100*time.Millisecond)

	// Should only have 2 events (the duplicate should be filtered)
	if len(sentEvents) != 2 {
		t.Fatalf("expected 2 events (duplicate filtered), got %d", len(sentEvents))
	}

	// Verify final combined text
	combinedText := ""
	for _, evt := range sentEvents {
		combinedText += evt.Text
	}
	if combinedText != "Done." {
		t.Errorf("expected combined text 'Done.', got %q", combinedText)
	}
}

func TestOpenCodeAdapter_FilterUserMessages(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	sessionID := "test-session"
	operationID := "test-op"

	// Track message roles
	adapter.mu.Lock()
	adapter.messageRoles = make(map[string]string)
	adapter.messageRoles["user-msg"] = "user"
	adapter.messageRoles["assistant-msg"] = "assistant"
	adapter.mu.Unlock()

	// Send user message part - should be filtered
	userProps := opencode.MessagePartUpdatedProperties{
		Part: opencode.Part{
			ID:        "user-part",
			Type:      opencode.PartTypeText,
			MessageID: "user-msg",
			SessionID: sessionID,
			Text:      "User input that should not appear",
		},
	}
	userJSON, _ := json.Marshal(userProps)
	adapter.handleMessagePartUpdated(userJSON, sessionID, operationID)

	// Send assistant message part - should be processed
	assistantProps := opencode.MessagePartUpdatedProperties{
		Part: opencode.Part{
			ID:        "assistant-part",
			Type:      opencode.PartTypeText,
			MessageID: "assistant-msg",
			SessionID: sessionID,
			Text:      "Assistant response",
		},
	}
	assistantJSON, _ := json.Marshal(assistantProps)
	adapter.handleMessagePartUpdated(assistantJSON, sessionID, operationID)

	sentEvents := drainEvents(adapter, 100*time.Millisecond)

	// Should only have 1 event (assistant message)
	if len(sentEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sentEvents))
	}

	if sentEvents[0].Text != "Assistant response" {
		t.Errorf("expected assistant response, got %q", sentEvents[0].Text)
	}
}

func TestOpenCodeAdapter_MessageRoleTracking(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	sessionID := "test-session"

	// Simulate message.updated event
	props := opencode.MessageUpdatedProperties{
		Info: opencode.MessageInfo{
			ID:        "msg-123",
			SessionID: sessionID,
			Role:      "assistant",
		},
	}
	propsJSON, _ := json.Marshal(props)

	adapter.handleMessageUpdated(propsJSON, sessionID)

	// Check role was tracked
	adapter.mu.RLock()
	role := adapter.messageRoles["msg-123"]
	adapter.mu.RUnlock()

	if role != "assistant" {
		t.Errorf("expected role 'assistant', got %q", role)
	}
}

func TestOpenCodeAdapter_ClearSessionState(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	// Add some state
	adapter.mu.Lock()
	adapter.textParts = map[string]*textPartState{
		"old-part": {lastTextLen: 100},
	}
	adapter.messageRoles = map[string]string{
		"old-msg": "assistant",
	}
	adapter.seenToolCalls = map[string]bool{
		"old-call": true,
	}
	adapter.mu.Unlock()

	// Clear state
	adapter.clearSessionState()

	// Verify state is cleared
	adapter.mu.RLock()
	defer adapter.mu.RUnlock()

	if len(adapter.textParts) != 0 {
		t.Errorf("expected textParts to be empty, got %d entries", len(adapter.textParts))
	}
	if len(adapter.messageRoles) != 0 {
		t.Errorf("expected messageRoles to be empty, got %d entries", len(adapter.messageRoles))
	}
	if len(adapter.seenToolCalls) != 0 {
		t.Errorf("expected seenToolCalls to be empty, got %d entries", len(adapter.seenToolCalls))
	}
}

func TestOpenCodeAdapter_ToolCallDeduplication(t *testing.T) {
	adapter := newTestOpenCodeAdapter()

	sessionID := "test-session"
	operationID := "test-op"

	// Simulate multiple tool call events for the same tool call ID
	for i := range 3 {
		var status string
		switch i {
		case 1:
			status = "running"
		case 2:
			status = "completed"
		default:
			status = "pending"
		}

		props := opencode.MessagePartUpdatedProperties{
			Part: opencode.Part{
				ID:        "tool-part-1",
				Type:      opencode.PartTypeTool,
				MessageID: "msg-1",
				SessionID: sessionID,
				CallID:    "call-123",
				Tool:      "bash",
				State: &opencode.ToolStateUpdate{
					Status: status,
					Title:  "Running command",
				},
			},
		}
		propsJSON, _ := json.Marshal(props)
		adapter.handleMessagePartUpdated(propsJSON, sessionID, operationID)
	}

	sentEvents := drainEvents(adapter, 100*time.Millisecond)

	// Should have 1 tool_call event and 2 tool_update events
	toolCallCount := 0
	toolUpdateCount := 0
	for _, evt := range sentEvents {
		switch evt.Type {
		case EventTypeToolCall:
			toolCallCount++
		case EventTypeToolUpdate:
			toolUpdateCount++
		}
	}

	if toolCallCount != 1 {
		t.Errorf("expected 1 tool_call event, got %d", toolCallCount)
	}
	if toolUpdateCount != 2 {
		t.Errorf("expected 2 tool_update events, got %d", toolUpdateCount)
	}
}
