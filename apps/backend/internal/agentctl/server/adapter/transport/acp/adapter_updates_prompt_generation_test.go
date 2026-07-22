package acp

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestEnqueueACPUpdateSnapshotsPromptGenerationBeforeWorkerConversion(t *testing.T) {
	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	var notification sdk.SessionNotification
	raw := []byte(`{"sessionId":"s1","update":{"sessionUpdate":"usage_update","size":1000000,"used":23638,"_meta":{"_claude/origin":{"kind":"human"}}}}`)
	if err := json.Unmarshal(raw, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}

	// Freeze the worker at handleACPUpdate's first adapter read lock. The SDK
	// callback remains free to enqueue because prompt-generation capture uses the
	// prompt-turn lock, not the adapter state lock.
	a.mu.Lock()
	_, oldTurn := a.registerPromptTurn(context.Background(), 42)
	a.enqueueACPUpdate(notification)
	a.clearPromptTurn(oldTurn)
	_, replacementTurn := a.registerPromptTurn(context.Background(), 99)
	a.mu.Unlock()
	t.Cleanup(func() { a.clearPromptTurn(replacementTurn) })

	// The FIFO barrier proves the target notification was converted after the
	// active prompt changed; no sleep or scheduler timing is involved.
	a.syncNotifQueue()

	var idle *AgentEvent
	for _, event := range drainEvents(a) {
		if event.Type == streams.EventTypeForegroundIdle {
			eventCopy := event
			idle = &eventCopy
			break
		}
	}
	if idle == nil {
		t.Fatal("expected foreground-idle event from queued human-origin update")
	}
	if idle.PromptGeneration != 42 {
		t.Fatalf("foreground-idle generation = %d, want enqueue-time generation 42", idle.PromptGeneration)
	}
}
