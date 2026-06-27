package websocket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestClient_SendMessageStampsConnectionSequenceAndLog(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-1")
	c.hub = h
	registerTestClient(h, c)

	first, err := ws.NewResponse("req-1", "health.check", map[string]bool{"ok": true})
	if err != nil {
		t.Fatalf("response: %v", err)
	}
	second, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}

	if !c.sendMessage(first) {
		t.Fatal("first send failed")
	}
	if !c.sendMessage(second) {
		t.Fatal("second send failed")
	}

	gotFirst := readStampedMessage(t, c)
	gotSecond := readStampedMessage(t, c)
	if gotFirst.ConnectionID != "conn-1" || gotSecond.ConnectionID != "conn-1" {
		t.Fatalf("connection IDs = %q, %q; want conn-1", gotFirst.ConnectionID, gotSecond.ConnectionID)
	}
	if gotFirst.ConnectionSeq != 1 || gotSecond.ConnectionSeq != 2 {
		t.Fatalf("connection seqs = %d, %d; want 1, 2", gotFirst.ConnectionSeq, gotSecond.ConnectionSeq)
	}
	if gotFirst.SessionSeq != 0 || gotSecond.SessionSeq != 0 {
		t.Fatalf("connection-wide messages should not carry session_seq: %+v %+v", gotFirst, gotSecond)
	}

	events, maxSeq, ok := h.GetSentEventsFor("conn-1", 0)
	if !ok {
		t.Fatal("sent log lookup failed")
	}
	if maxSeq != 2 {
		t.Fatalf("max connection seq = %d; want 2", maxSeq)
	}
	if len(events) != 2 {
		t.Fatalf("sent log entries = %d; want 2", len(events))
	}
	if events[0].ConnectionSeq != 1 || events[1].ConnectionSeq != 2 {
		t.Fatalf("sent log connection seqs = %d, %d; want 1, 2", events[0].ConnectionSeq, events[1].ConnectionSeq)
	}
}

func TestClient_DroppedSendDoesNotRecordSentLog(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-full")
	c.hub = h
	registerTestClient(h, c)

	for range cap(c.send) {
		c.send <- []byte(`{"type":"notification","action":"preloaded"}`)
	}
	msg, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}

	if c.sendMessage(msg) {
		t.Fatal("send unexpectedly succeeded with a full client buffer")
	}

	events, maxSeq, ok := h.GetSentEventsFor("conn-full", 0)
	if !ok {
		t.Fatal("sent log lookup failed")
	}
	if len(events) != 0 || maxSeq != 0 {
		t.Fatalf("sent log = max %d events %+v; want no logged sent events", maxSeq, events)
	}
}

func TestClient_SendSessionDataStampsConnectionOnly(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-data")
	c.hub = h
	registerTestClient(h, c)

	h.SetSessionDataProvider(func(_ context.Context, sessionID string) ([]*ws.Message, error) {
		msg, err := ws.NewNotification("session.git.event", map[string]string{
			"session_id": sessionID,
			"type":       "status_update",
		})
		if err != nil {
			t.Fatalf("notification: %v", err)
		}
		return []*ws.Message{msg}, nil
	})

	c.sendSessionData("session-a")

	got := readStampedMessage(t, c)
	if got.ConnectionSeq != 1 || got.SessionSeq != 0 {
		t.Fatalf("session data seqs = connection %d session %d; want 1/0", got.ConnectionSeq, got.SessionSeq)
	}
	if got.ConnectionID != "conn-data" {
		t.Fatalf("connection id = %q; want conn-data", got.ConnectionID)
	}

	events, maxSeq, ok := h.GetSentEventsFor("conn-data", 0)
	if !ok {
		t.Fatal("connection sent log lookup failed")
	}
	if maxSeq != 1 || len(events) != 1 || events[0].SessionSeq != 0 || events[0].SessionID != "" {
		t.Fatalf("connection sent log = max %d events %+v; want one connection-only event", maxSeq, events)
	}

	sessionEvents, sessionMaxSeq, ok := h.GetSentEventsForSession("conn-data", "session-a")
	if !ok {
		t.Fatal("session sent log lookup failed")
	}
	if sessionMaxSeq != 0 || len(sessionEvents) != 0 {
		t.Fatalf("session sent log = max %d events %+v; want no replay session events", sessionMaxSeq, sessionEvents)
	}
}

func TestClient_SendMessageStampsCopyWithoutMutatingMessage(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-copy")
	c.hub = h
	registerTestClient(h, c)

	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	msg.ConnectionID = "caller-owned"
	msg.ConnectionSeq = 41
	msg.SessionSeq = 42

	if !c.sendMessageForSession("session-a", msg) {
		t.Fatal("session send failed")
	}
	gotSession := readStampedMessage(t, c)
	if gotSession.ConnectionID != "conn-copy" || gotSession.ConnectionSeq != 1 || gotSession.SessionSeq != 1 {
		t.Fatalf("session stamped message = %+v; want conn-copy seq 1 session seq 1", gotSession)
	}
	if msg.ConnectionID != "caller-owned" || msg.ConnectionSeq != 41 || msg.SessionSeq != 42 {
		t.Fatalf("source message mutated after session send: %+v", msg)
	}

	if !c.sendMessage(msg) {
		t.Fatal("connection send failed")
	}
	gotConnection := readStampedMessage(t, c)
	if gotConnection.ConnectionID != "conn-copy" || gotConnection.ConnectionSeq != 2 || gotConnection.SessionSeq != 0 {
		t.Fatalf("connection stamped message = %+v; want conn-copy seq 2 without session seq", gotConnection)
	}
	if msg.ConnectionID != "caller-owned" || msg.ConnectionSeq != 41 || msg.SessionSeq != 42 {
		t.Fatalf("source message mutated after connection send: %+v", msg)
	}
}

func TestHub_BroadcastToSessionStampsSessionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-session")
	c.hub = h
	registerTestClient(h, c)
	h.SubscribeToSession(c, "session-a")
	h.SubscribeToSession(c, "session-b")

	msgA1, _ := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	msgB1, _ := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-b"})
	msgA2, _ := ws.NewNotification("session.message.updated", map[string]string{"session_id": "session-a"})

	h.BroadcastToSession("session-a", msgA1)
	h.BroadcastToSession("session-b", msgB1)
	h.BroadcastToSession("session-a", msgA2)

	gotA1 := readStampedMessage(t, c)
	gotB1 := readStampedMessage(t, c)
	gotA2 := readStampedMessage(t, c)
	if gotA1.ConnectionSeq != 1 || gotB1.ConnectionSeq != 2 || gotA2.ConnectionSeq != 3 {
		t.Fatalf("connection seqs = %d, %d, %d; want 1, 2, 3",
			gotA1.ConnectionSeq, gotB1.ConnectionSeq, gotA2.ConnectionSeq)
	}
	if gotA1.SessionSeq != 1 || gotB1.SessionSeq != 1 || gotA2.SessionSeq != 2 {
		t.Fatalf("session seqs = %d, %d, %d; want 1, 1, 2",
			gotA1.SessionSeq, gotB1.SessionSeq, gotA2.SessionSeq)
	}

	eventsA, maxA, ok := h.GetSentEventsForSession("conn-session", "session-a")
	if !ok {
		t.Fatal("session-a sent log lookup failed")
	}
	if maxA != 2 || len(eventsA) != 2 {
		t.Fatalf("session-a sent log max/len = %d/%d; want 2/2", maxA, len(eventsA))
	}
	for i, event := range eventsA {
		want := int64(i + 1)
		if event.SessionID != "session-a" || event.SessionSeq != want {
			t.Fatalf("session-a event %d = %+v; want session_id=session-a session_seq=%d", i, event, want)
		}
	}

	eventsB, maxB, ok := h.GetSentEventsForSession("conn-session", "session-b")
	if !ok {
		t.Fatal("session-b sent log lookup failed")
	}
	if maxB != 1 || len(eventsB) != 1 || eventsB[0].SessionSeq != 1 {
		t.Fatalf("session-b sent log = max %d events %+v; want one session_seq=1", maxB, eventsB)
	}
}

func TestHub_BroadcastToSessionUsesOneSessionSequencePerLogicalEvent(t *testing.T) {
	h := newTestHub(t)
	first := newTestClient("conn-1")
	second := newTestClient("conn-2")
	first.hub = h
	second.hub = h
	registerTestClient(h, first)
	registerTestClient(h, second)
	h.SubscribeToSession(first, "session-a")
	h.SubscribeToSession(second, "session-a")

	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("session-a", msg)

	gotFirst := readStampedMessage(t, first)
	gotSecond := readStampedMessage(t, second)
	if gotFirst.SessionSeq != 1 || gotSecond.SessionSeq != 1 {
		t.Fatalf("session seqs = %d, %d; want both recipients to receive seq 1",
			gotFirst.SessionSeq, gotSecond.SessionSeq)
	}

	firstEvents, firstMax, ok := h.GetSentEventsForSession("conn-1", "session-a")
	if !ok {
		t.Fatal("first sent log lookup failed")
	}
	secondEvents, secondMax, ok := h.GetSentEventsForSession("conn-2", "session-a")
	if !ok {
		t.Fatal("second sent log lookup failed")
	}
	if firstMax != 1 || secondMax != 1 || len(firstEvents) != 1 || len(secondEvents) != 1 {
		t.Fatalf("sent logs = first max %d events %+v second max %d events %+v; want one seq=1 each",
			firstMax, firstEvents, secondMax, secondEvents)
	}
}

func TestWsSentLogEvictsOldestAndFiltersSince(t *testing.T) {
	log := newWsSentLogWithCapacity(3)
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	for seq := int64(1); seq <= 4; seq++ {
		log.Append(seq, 0, "", "notification", "task.updated", base.Add(time.Duration(seq)*time.Second))
	}

	all := log.Since(0)
	if len(all) != 3 {
		t.Fatalf("entries after eviction = %d; want 3", len(all))
	}
	if all[0].ConnectionSeq != 2 || all[2].ConnectionSeq != 4 {
		t.Fatalf("evicted entries = %+v; want seqs 2..4", all)
	}
	if got := log.MaxConnectionSeq(); got != 4 {
		t.Fatalf("max connection seq = %d; want 4", got)
	}

	filtered := log.Since(2)
	if len(filtered) != 2 || filtered[0].ConnectionSeq != 3 || filtered[1].ConnectionSeq != 4 {
		t.Fatalf("filtered entries = %+v; want seqs 3,4", filtered)
	}
}

func readStampedMessage(t *testing.T, c *Client) ws.Message {
	t.Helper()
	select {
	case raw := <-c.send:
		var msg ws.Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("decode stamped message: %v", err)
		}
		return msg
	default:
		t.Fatal("client send channel was empty")
		return ws.Message{}
	}
}
