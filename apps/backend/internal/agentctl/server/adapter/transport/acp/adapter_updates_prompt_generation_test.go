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
	a.agentID = claudeAgentID
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

// TestHandleACPUpdate_HumanOriginDeliversLeadingContextWindowThenForegroundIdle
// pins tryConvertUntypedUpdate's split return: a human-origin usage_update
// carries both a context-window reading and a derived foreground-idle
// transition from the same provider frame. Both must reach handleACPUpdate's
// single log/trace/send path — in provider order, context window first — so
// the leading event is no longer sent out of band, bypassing
// shared.LogNormalizedEvent.
func TestHandleACPUpdate_HumanOriginDeliversLeadingContextWindowThenForegroundIdle(t *testing.T) {
	a := newTestAdapter()
	a.agentID = claudeAgentID
	t.Cleanup(func() { _ = a.Close() })

	var notification sdk.SessionNotification
	raw := []byte(`{"sessionId":"s1","update":{"sessionUpdate":"usage_update","size":1000000,"used":23638,"_meta":{"_claude/origin":{"kind":"human"}}}}`)
	if err := json.Unmarshal(raw, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}

	a.handleACPUpdate(notification, 42)

	events := drainEvents(a)
	if len(events) != 2 {
		t.Fatalf("expected 2 delivered events (leading context window + foreground idle), got %d: %+v", len(events), events)
	}
	if events[0].Type != streams.EventTypeContextWindow {
		t.Fatalf("first delivered event type = %q, want %q (context window must precede the derived lifecycle event)",
			events[0].Type, streams.EventTypeContextWindow)
	}
	if events[1].Type != streams.EventTypeForegroundIdle {
		t.Fatalf("second delivered event type = %q, want %q", events[1].Type, streams.EventTypeForegroundIdle)
	}
}

func TestHandleACPUpdate_HumanOriginOnlySuppressesUnsupportedHumanTurnHandoff(t *testing.T) {
	var notification sdk.SessionNotification
	raw := []byte(`{"sessionId":"s1","update":{"sessionUpdate":"usage_update","size":1000000,"used":23638,"_meta":{"_claude/origin":{"kind":"human"}}}}`)
	if err := json.Unmarshal(raw, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}

	for _, test := range []struct {
		name             string
		agentID          string
		promptGeneration uint64
		wantIdle         bool
	}{
		{name: "unsupported adapter", agentID: "other-acp", promptGeneration: 42},
		{name: "synthetic generation zero", agentID: claudeAgentID, promptGeneration: 0, wantIdle: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapter()
			a.agentID = test.agentID
			t.Cleanup(func() { _ = a.Close() })

			a.handleACPUpdate(notification, test.promptGeneration)

			events := drainEvents(a)
			if !test.wantIdle && (len(events) != 1 || events[0].Type != streams.EventTypeContextWindow) {
				t.Fatalf("events = %+v, want context-window only", events)
			}
			if test.wantIdle {
				if len(events) != 2 || events[1].Type != streams.EventTypeForegroundIdle {
					t.Fatalf("events = %+v, want context-window then foreground-idle", events)
				}
				if events[1].PromptGeneration != 0 {
					t.Fatalf("synthetic idle generation = %d, want 0", events[1].PromptGeneration)
				}
				if handoff, _ := events[1].Data[streams.AgentEventDataPromptHandoff].(bool); handoff {
					t.Fatal("generation-zero idle incorrectly attested human prompt handoff")
				}
			}
		})
	}
}
