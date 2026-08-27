package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSessionStreamBroadcaster_FileChangeBatching(t *testing.T) {
	// Test that multiple file changes are batched correctly
	b := &SessionStreamBroadcaster{
		fileChangeBatch:  make(map[string][]any),
		fileChangeTimers: make(map[string]*time.Timer),
	}

	sessionID := "test-session-123"

	// Add multiple file changes
	for i := range 5 {
		b.fileChangeMu.Lock()
		b.fileChangeBatch[sessionID] = append(b.fileChangeBatch[sessionID], map[string]any{
			"path":       "/test/file" + string(rune('0'+i)) + ".txt",
			"operation":  "write",
			"session_id": sessionID,
		})
		b.fileChangeMu.Unlock()
	}

	// Check batch size
	b.fileChangeMu.Lock()
	batchLen := len(b.fileChangeBatch[sessionID])
	b.fileChangeMu.Unlock()

	if batchLen != 5 {
		t.Errorf("expected batch size 5, got %d", batchLen)
	}

	// Verify batch contents
	b.fileChangeMu.Lock()
	batch := b.fileChangeBatch[sessionID]
	for i, item := range batch {
		m, ok := item.(map[string]any)
		if !ok {
			t.Errorf("batch item %d is not a map", i)
			continue
		}
		if m["session_id"] != sessionID {
			t.Errorf("batch item %d has wrong session_id", i)
		}
	}
	b.fileChangeMu.Unlock()
}

func TestSessionStreamBroadcaster_MaxBatchSize(t *testing.T) {
	// Test that batch reaches max size correctly
	b := &SessionStreamBroadcaster{
		fileChangeBatch:  make(map[string][]any),
		fileChangeTimers: make(map[string]*time.Timer),
	}

	sessionID := "test-session-456"

	// Add maxFileChangeBatchSize items
	b.fileChangeMu.Lock()
	for range maxFileChangeBatchSize {
		b.fileChangeBatch[sessionID] = append(b.fileChangeBatch[sessionID], map[string]any{
			"path": "/test/file.txt",
		})
	}
	batchLen := len(b.fileChangeBatch[sessionID])
	b.fileChangeMu.Unlock()

	if batchLen != maxFileChangeBatchSize {
		t.Errorf("expected batch size %d, got %d", maxFileChangeBatchSize, batchLen)
	}

	// Verify the constant is set correctly
	if maxFileChangeBatchSize != 50 {
		t.Errorf("expected maxFileChangeBatchSize to be 50, got %d", maxFileChangeBatchSize)
	}
}

func TestSessionStreamBroadcaster_DebounceWindowConstant(t *testing.T) {
	// Verify debounce window is set to expected value
	expectedWindow := 100 * time.Millisecond
	if fileChangeDebounceWindow != expectedWindow {
		t.Errorf("expected fileChangeDebounceWindow to be %v, got %v", expectedWindow, fileChangeDebounceWindow)
	}
}

func TestSessionStreamBroadcaster_Close(t *testing.T) {
	b := &SessionStreamBroadcaster{
		fileChangeBatch:  make(map[string][]any),
		fileChangeTimers: make(map[string]*time.Timer),
	}

	// Add a timer
	sessionID := "test-session-789"
	b.fileChangeTimers[sessionID] = time.AfterFunc(time.Hour, func() {})
	b.fileChangeBatch[sessionID] = []any{map[string]any{"path": "/test.txt"}}

	// Close should clean up
	b.Close()

	if b.fileChangeBatch != nil {
		t.Error("expected fileChangeBatch to be nil after Close")
	}
	if b.fileChangeTimers != nil {
		t.Error("expected fileChangeTimers to be nil after Close")
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		expected string
	}{
		{
			name:     "nil data",
			data:     nil,
			expected: "",
		},
		{
			name: "map with session_id",
			data: map[string]any{
				"session_id": "session-123",
				"path":       "/test.txt",
			},
			expected: "session-123",
		},
		{
			name: "map without session_id",
			data: map[string]any{
				"path": "/test.txt",
			},
			expected: "",
		},
		{
			name:     "non-map type",
			data:     "string value",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSessionID(tt.data)
			if result != tt.expected {
				t.Errorf("extractSessionID(%v) = %q, want %q", tt.data, result, tt.expected)
			}
		})
	}
}

// TestSessionStreamBroadcaster_SubscribesFallbackSubject pins the subject
// mapping: RegisterSessionStreamNotifications must subscribe the
// session_model_fallback wildcard so orchestrator publishes reach the hub.
func TestSessionStreamBroadcaster_SubscribesFallbackSubject(t *testing.T) {
	log := testLogger()
	eventBus := &subscriptionRecordingEventBus{MemoryEventBus: bus.NewMemoryEventBus(log)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := NewHub(nil, log)
	b := RegisterSessionStreamNotifications(ctx, eventBus, hub, log)
	_ = b

	for _, subject := range eventBus.subjects {
		if subject == events.BuildSessionModelFallbackWildcardSubject() {
			return
		}
	}
	t.Fatalf("session stream broadcaster must subscribe the fallback wildcard subject %q",
		events.BuildSessionModelFallbackWildcardSubject())
}

// TestSessionStreamBroadcaster_BroadcastsFallbackNotification verifies a
// published session_model_fallback event is routed to the session's
// subscribers as the session.model_fallback action — the payload's
// GetSessionID must drive extractSessionID (a broken routing key drops the
// notification silently).
func TestSessionStreamBroadcaster_BroadcastsFallbackNotification(t *testing.T) {
	hub := newAccessTestHub(t)
	eventBus := bus.NewMemoryEventBus(testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = RegisterSessionStreamNotifications(ctx, eventBus, hub, testLogger())

	client := registerAccessClient(t, hub, "client-fallback", authn.Identity{UserID: "user-1", Role: authn.RoleMember})
	raw, err := json.Marshal(map[string]interface{}{"session_id": "session-fallback-1"})
	if err != nil {
		t.Fatalf("marshal subscribe payload: %v", err)
	}
	client.handleSessionSubscribe(&ws.Message{
		ID: "1", Type: ws.MessageTypeRequest, Action: ws.ActionSessionSubscribe, Payload: raw,
	})

	sessionID := "session-fallback-1"
	_ = eventBus.Publish(ctx, events.BuildSessionModelFallbackSubject(sessionID), bus.NewEvent(
		events.SessionModelFallbackUpdated,
		"test",
		lifecycle.SessionModelFallbackEventPayload{
			TaskID:        "task-1",
			SessionID:     sessionID,
			AgentID:       "a1",
			FallbackModel: "gpt-5",
			Timestamp:     time.Now().UTC().Format(time.RFC3339),
		},
	))

	// Wait deterministically on the client's send channels (the hub writes
	// the broadcast there synchronously) instead of polling on a clock.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case raw := <-client.send:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil && msg.Action == ws.ActionSessionModelFallback {
				return
			}
		case raw := <-client.controlSend:
			var msg ws.Message
			if err := json.Unmarshal(raw, &msg); err == nil && msg.Action == ws.ActionSessionModelFallback {
				return
			}
		case <-timeout:
			t.Fatalf("client did not receive %s", ws.ActionSessionModelFallback)
		}
	}
}
