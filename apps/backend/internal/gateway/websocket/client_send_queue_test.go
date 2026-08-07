package websocket

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClient_ControlResponseIsNotBlockedByStreamBacklog(t *testing.T) {
	c := newTestClient("control-priority")
	c.controlSend = make(chan []byte, controlSendBufferSize)

	for index := 0; index < cap(c.send); index++ {
		if !c.sendBytes([]byte("stream")) {
			t.Fatalf("stream frame %d was not queued", index)
		}
	}

	response, err := ws.NewResponse("request-1", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build response: %v", err)
	}
	c.sendMessage(response)

	select {
	case data := <-c.controlSend:
		var got ws.Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode control frame: %v", err)
		}
		if got.ID != "request-1" || got.Type != ws.MessageTypeResponse {
			t.Fatalf("unexpected control frame: %+v", got)
		}
	default:
		t.Fatal("control response was not queued while stream queue was full")
	}
}

func TestClient_ControlOverflowClosesConnection(t *testing.T) {
	c := newTestClient("control-overflow")
	c.controlSend = make(chan []byte, 1)

	first, err := ws.NewResponse("request-1", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build first response: %v", err)
	}
	second, err := ws.NewResponse("request-2", "message.add", map[string]any{"accepted": true})
	if err != nil {
		t.Fatalf("build second response: %v", err)
	}
	c.sendMessage(first)
	c.sendMessage(second)

	if !c.closed {
		t.Fatal("control overflow should close the client")
	}
	if _, ok := <-c.send; ok {
		t.Fatal("stream queue should be closed after control overflow")
	}
	if _, ok := <-c.controlSend; ok {
		// The first response is still drainable, but the queue must close after
		// its buffered frame is consumed.
		if _, stillOpen := <-c.controlSend; stillOpen {
			t.Fatal("control queue remained open after overflow")
		}
	}
}

func TestClient_ReplaceableSessionMessageKeepsLatestFrame(t *testing.T) {
	c := newTestClient("replaceable-session-message")

	first := []byte(`{"type":"notification","action":"session.message.updated","payload":{"session_id":"session-1","message_id":"message-1","content":"old"}}`)
	latest := []byte(`{"type":"notification","action":"session.message.updated","payload":{"session_id":"session-1","message_id":"message-1","content":"latest"}}`)

	if !c.sendBytes(first) || !c.sendBytes(latest) {
		t.Fatal("replaceable frames should be accepted")
	}
	if got := len(c.send); got != 0 {
		t.Fatalf("replaceable frame leaked into semantic queue, got %d", got)
	}

	frame, ok := c.popNextReplaceable()
	if !ok {
		t.Fatal("expected one replaceable frame")
	}
	var message ws.Message
	if err := json.Unmarshal(frame.data, &message); err != nil {
		t.Fatalf("decode queued frame: %v", err)
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Content != "latest" {
		t.Fatalf("expected latest content, got %q", payload.Content)
	}
	if stats := c.notificationQueueStats(); stats.Depth != 0 || stats.Replacements != 1 {
		t.Fatalf("unexpected replaceable stats: %+v", stats)
	}
}

func TestClient_ReplaceableQueuePreservesPositionAndSemanticBarrier(t *testing.T) {
	c := newTestClient("replaceable-order")

	first, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
		"session_id": "session-1", "message_id": "message-1", "content": "first",
	})
	if err != nil {
		t.Fatalf("build first update: %v", err)
	}
	latest, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
		"session_id": "session-1", "message_id": "message-1", "content": "latest",
	})
	if err != nil {
		t.Fatalf("build latest update: %v", err)
	}
	deleted, err := ws.NewNotification(ws.ActionSessionMessageDeleted, map[string]any{
		"session_id": "session-1", "message_id": "message-1",
	})
	if err != nil {
		t.Fatalf("build delete: %v", err)
	}

	firstBytes, _ := json.Marshal(first)
	latestBytes, _ := json.Marshal(latest)
	deletedBytes, _ := json.Marshal(deleted)
	if !c.sendNotification(firstBytes, first.Action) || !c.sendNotification(latestBytes, latest.Action) {
		t.Fatal("message updates should be accepted")
	}
	if !c.sendNotification(deletedBytes, deleted.Action) {
		t.Fatal("semantic barrier should be accepted")
	}
	if got := len(c.send); got != 0 {
		t.Fatalf("semantic barrier overtook replacement queue, got %d frames", got)
	}

	frame, ok := c.popNextReplaceable()
	if !ok {
		t.Fatal("expected replacement frame")
	}
	var got ws.Message
	if err := json.Unmarshal(frame.data, &got); err != nil {
		t.Fatalf("decode replacement: %v", err)
	}
	if got.Action != ws.ActionSessionMessageUpdated {
		t.Fatalf("expected message update first, got %q", got.Action)
	}
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("decode replacement payload: %v", err)
	}
	if payload.Content != "latest" {
		t.Fatalf("expected latest payload in original position, got %q", payload.Content)
	}

	barrierFrame, ok := c.popNextReplaceable()
	if !ok {
		t.Fatal("expected semantic barrier after replacement")
	}
	var barrierMessage ws.Message
	if err := json.Unmarshal(barrierFrame.data, &barrierMessage); err != nil {
		t.Fatalf("decode semantic barrier: %v", err)
	}
	if barrierMessage.Action != ws.ActionSessionMessageDeleted {
		t.Fatalf("expected delete barrier after replacement, got %q", barrierMessage.Action)
	}
}

func TestClient_ReplaceableQueueRoundRobinsSessions(t *testing.T) {
	c := newTestClient("replaceable-round-robin")
	for _, item := range []struct {
		sessionID string
		messageID string
		content   string
	}{
		{sessionID: "session-a", messageID: "message-a1", content: "a1"},
		{sessionID: "session-a", messageID: "message-a2", content: "a2"},
		{sessionID: "session-b", messageID: "message-b1", content: "b1"},
		{sessionID: "session-b", messageID: "message-b2", content: "b2"},
	} {
		message, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
			"session_id": item.sessionID, "message_id": item.messageID, "content": item.content,
		})
		if err != nil {
			t.Fatalf("build update: %v", err)
		}
		data, _ := json.Marshal(message)
		if !c.sendNotification(data, message.Action) {
			t.Fatalf("queue update %s/%s", item.sessionID, item.messageID)
		}
	}

	var got []string
	for i := 0; i < 4; i++ {
		frame, ok := c.popNextReplaceable()
		if !ok {
			t.Fatalf("missing frame %d", i)
		}
		var message ws.Message
		if err := json.Unmarshal(frame.data, &message); err != nil {
			t.Fatalf("decode frame %d: %v", i, err)
		}
		var payload struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatalf("decode frame payload %d: %v", i, err)
		}
		got = append(got, payload.MessageID)
	}
	want := []string{"message-a1", "message-b1", "message-a2", "message-b2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round-robin order = %v, want %v", got, want)
	}
}

func TestClient_ReplaceableQueueBoundsNoisySessionOnly(t *testing.T) {
	c := newTestClient("replaceable-bounds")
	c.replaceablePerSessionLimit = 2
	c.replaceableGlobalLimit = 4

	queueUpdate := func(sessionID, messageID string) {
		t.Helper()
		message, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
			"session_id": sessionID, "message_id": messageID,
		})
		if err != nil {
			t.Fatalf("build update: %v", err)
		}
		data, _ := json.Marshal(message)
		if !c.sendNotification(data, message.Action) {
			t.Fatalf("queue update %s/%s", sessionID, messageID)
		}
	}

	queueUpdate("noisy", "n1")
	queueUpdate("noisy", "n2")
	queueUpdate("noisy", "n3") // evicts noisy/n1
	queueUpdate("quiet", "q1")
	queueUpdate("quiet", "q2")
	queueUpdate("quiet", "q3") // evicts quiet/q1, not noisy
	if accepted := c.sendNotification([]byte(`{"type":"notification","action":"session.message.updated","payload":{"session_id":"other","message_id":"o1"}}`), ws.ActionSessionMessageUpdated); accepted {
		t.Fatal("global overflow should reject a new session rather than evict another session")
	}

	stats := c.notificationQueueStats()
	if stats.Depth != 4 || stats.Evictions != 2 || stats.Rejected != 1 {
		t.Fatalf("unexpected bounded queue stats: %+v", stats)
	}
	var got []string
	for {
		frame, ok := c.popNextReplaceable()
		if !ok {
			break
		}
		var message ws.Message
		if err := json.Unmarshal(frame.data, &message); err != nil {
			t.Fatalf("decode bounded frame: %v", err)
		}
		var payload struct {
			MessageID string `json:"message_id"`
		}
		if err := json.Unmarshal(message.Payload, &payload); err != nil {
			t.Fatalf("decode bounded payload: %v", err)
		}
		got = append(got, payload.MessageID)
	}
	for _, messageID := range []string{"n2", "n3", "q2", "q3"} {
		if !slices.Contains(got, messageID) {
			t.Fatalf("bounded queue lost %s: %v", messageID, got)
		}
	}
	for _, messageID := range []string{"n1", "q1", "o1"} {
		if slices.Contains(got, messageID) {
			t.Fatalf("bounded queue retained evicted/rejected %s: %v", messageID, got)
		}
	}
}

func TestClient_ReplaceableEvictionKeepsSessionOrderUnique(t *testing.T) {
	c := newTestClient("replaceable-eviction-order")
	c.replaceablePerSessionLimit = 2
	c.replaceableGlobalLimit = 2

	queueUpdate := func(sessionID, messageID string) {
		t.Helper()
		message, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
			"session_id": sessionID, "message_id": messageID,
		})
		if err != nil {
			t.Fatalf("build update: %v", err)
		}
		data, _ := json.Marshal(message)
		if !c.sendNotification(data, message.Action) {
			t.Fatalf("queue update %s/%s", sessionID, messageID)
		}
	}

	queueUpdate("session-a", "message-a1")
	queueUpdate("session-b", "message-b1")
	queueUpdate("session-a", "message-a2") // evicts a1 while a remains registered

	c.mu.RLock()
	order := append([]string(nil), c.replaceableSessionOrder...)
	c.mu.RUnlock()
	if !reflect.DeepEqual(order, []string{"session-a", "session-b"}) {
		t.Fatalf("replaceable session order = %v, want one entry per session", order)
	}
}

func TestClient_SemanticBarrierPartitionsReplacementUpdates(t *testing.T) {
	c := newTestClient("semantic-barrier-partition")

	queueUpdate := func(content string) {
		t.Helper()
		message, err := ws.NewNotification(ws.ActionSessionMessageUpdated, map[string]any{
			"session_id": "session-1", "message_id": "message-1", "content": content,
		})
		if err != nil {
			t.Fatalf("build update: %v", err)
		}
		data, _ := json.Marshal(message)
		if !c.sendNotification(data, message.Action) {
			t.Fatalf("queue update %q", content)
		}
	}
	queueUpdate("before")

	deleted, err := ws.NewNotification(ws.ActionSessionMessageDeleted, map[string]any{
		"session_id": "session-1", "message_id": "message-1",
	})
	if err != nil {
		t.Fatalf("build delete: %v", err)
	}
	deletedData, _ := json.Marshal(deleted)
	if !c.sendNotification(deletedData, deleted.Action) {
		t.Fatal("semantic barrier was not accepted")
	}
	queueUpdate("after")

	readAction := func() (string, string) {
		t.Helper()
		frame, ok := c.popNextReplaceable()
		if !ok {
			t.Fatal("expected scheduled frame")
		}
		var message ws.Message
		if err := json.Unmarshal(frame.data, &message); err != nil {
			t.Fatalf("decode scheduled frame: %v", err)
		}
		var payload struct {
			Content string `json:"content"`
		}
		if len(message.Payload) > 0 {
			if err := json.Unmarshal(message.Payload, &payload); err != nil {
				t.Fatalf("decode scheduled payload: %v", err)
			}
		}
		return message.Action, payload.Content
	}

	if action, content := readAction(); action != ws.ActionSessionMessageUpdated || content != "before" {
		t.Fatalf("first scheduled frame = (%q, %q), want before update", action, content)
	}
	if action, _ := readAction(); action != ws.ActionSessionMessageDeleted {
		t.Fatalf("second scheduled frame = %q, want delete barrier", action)
	}
	if action, content := readAction(); action != ws.ActionSessionMessageUpdated || content != "after" {
		t.Fatalf("third scheduled frame = (%q, %q), want after update", action, content)
	}
}

func TestClient_SemanticDropsAreCounted(t *testing.T) {
	c := newTestClient("semantic-drop-count")
	c.send = make(chan []byte, 1)
	c.send <- []byte("occupied")

	if c.sendNotification([]byte(`{"type":"notification","action":"session.state_updated"}`), "session.state_updated") {
		t.Fatal("semantic frame should be rejected when the queue is full")
	}
	if got := c.notificationQueueStats().DroppedSemantic; got != 1 {
		t.Fatalf("dropped semantic count = %d, want 1", got)
	}
}

func TestClient_ReplaceableQueueCloseAndZeroValueAreSafe(t *testing.T) {
	var zero Client
	frame := []byte(`{"type":"notification","action":"session.message.updated","payload":{"session_id":"session-1","message_id":"message-1"}}`)
	if !zero.sendBytes(frame) {
		t.Fatal("zero-value client should accept replaceable traffic")
	}
	if _, ok := zero.popNextReplaceable(); !ok {
		t.Fatal("zero-value replaceable traffic was not drainable")
	}
	if zero.sendBytes([]byte("semantic")) {
		t.Fatal("zero-value client should reject semantic traffic without a channel")
	}

	c := newTestClient("replaceable-close")
	if !c.sendBytes(frame) {
		t.Fatal("replaceable frame was not accepted before close")
	}
	c.closeSend()
	if c.sendBytes(frame) {
		t.Fatal("closed client accepted replaceable traffic")
	}
	if _, ok := c.popNextReplaceable(); !ok {
		t.Fatal("queued replaceable frame was lost during close drain")
	}
}
