package websocket

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
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

func TestNewClient_DisablesSentAccountingByDefault(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "")
	h := newTestHub(t)
	c := NewClient("conn-prod", nil, h, testLogger())
	registerTestClient(h, c)

	if c.sentLog != nil {
		t.Fatal("sent log should be nil when E2E harness is disabled")
	}
	msg, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if !c.sendMessage(msg) {
		t.Fatal("send failed")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionID != "" || got.ConnectionSeq != 0 || got.SessionID != "" || got.SessionSeq != 0 {
		t.Fatalf("production envelope carried accounting fields: %+v", got)
	}
	if events, maxSeq, ok := h.GetSentEventsFor("conn-prod", 0); ok || len(events) != 0 || maxSeq != 0 {
		t.Fatalf("sent events = ok %v max %d events %+v; want disabled", ok, maxSeq, events)
	}
}

func TestNewClient_EnablesSentAccountingInHarness(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	h := newTestHub(t)
	c := NewClient("conn-harness", nil, h, testLogger())
	registerTestClient(h, c)

	if c.sentLog == nil {
		t.Fatal("sent log should be allocated when E2E harness is enabled")
	}
	msg, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if !c.sendMessage(msg) {
		t.Fatal("send failed")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionID != "conn-harness" || got.ConnectionSeq != 1 {
		t.Fatalf("accounting fields = connection_id %q seq %d; want conn-harness/1",
			got.ConnectionID, got.ConnectionSeq)
	}
	events, maxSeq, ok := h.GetSentEventsFor("conn-harness", 0)
	if !ok || maxSeq != 1 || len(events) != 1 || events[0].ConnectionSeq != 1 {
		t.Fatalf("sent events = ok %v max %d events %+v; want one event at seq 1", ok, maxSeq, events)
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
	if got := c.connectionSeq.Load(); got != 0 {
		t.Fatalf("connection seq after dropped send = %d; want 0", got)
	}

	for range cap(c.send) {
		<-c.send
	}
	next, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-2"})
	if err != nil {
		t.Fatalf("next notification: %v", err)
	}
	if !c.sendMessage(next) {
		t.Fatal("next send failed after draining buffer")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionSeq != 1 {
		t.Fatalf("connection seq after dropped send = %d; want 1", got.ConnectionSeq)
	}
}

func TestClient_DroppedSessionSendDoesNotAdvanceSessionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-full-session-send")
	c.hub = h
	registerTestClient(h, c)

	for range cap(c.send) {
		c.send <- []byte(`{"type":"notification","action":"preloaded"}`)
	}
	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}

	if c.sendMessageForSession("session-a", msg) {
		t.Fatal("session send unexpectedly succeeded with a full client buffer")
	}
	if got := currentSessionSeq(h, "session-a"); got != 0 {
		t.Fatalf("session seq after dropped send = %d; want 0", got)
	}

	for range cap(c.send) {
		<-c.send
	}
	if !c.sendMessageForSession("session-a", msg) {
		t.Fatal("session send failed after draining buffer")
	}
	got := readStampedMessage(t, c)
	if got.SessionSeq != 1 {
		t.Fatalf("session seq after dropped send = %d; want 1", got.SessionSeq)
	}
	if got := currentSessionSeq(h, "session-a"); got != 1 {
		t.Fatalf("hub session seq after successful send = %d; want 1", got)
	}
}

func TestClient_MarshalFailureDoesNotAdvanceConnectionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-marshal")
	c.hub = h
	registerTestClient(h, c)

	bad := &ws.Message{
		Type:      ws.MessageTypeNotification,
		Action:    "task.updated",
		Payload:   json.RawMessage(`{`),
		Timestamp: time.Now(),
	}
	if c.sendMessage(bad) {
		t.Fatal("send unexpectedly succeeded with invalid JSON payload")
	}
	if got := c.connectionSeq.Load(); got != 0 {
		t.Fatalf("connection seq after marshal failure = %d; want 0", got)
	}

	good, err := ws.NewNotification("task.updated", map[string]string{"task_id": "task-1"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if !c.sendMessage(good) {
		t.Fatal("valid send failed after marshal failure")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionSeq != 1 {
		t.Fatalf("connection seq after marshal failure = %d; want 1", got.ConnectionSeq)
	}
}

func TestClient_SessionMarshalFailureDoesNotAdvanceSessionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-session-marshal")
	c.hub = h
	registerTestClient(h, c)

	bad := &ws.Message{
		Type:      ws.MessageTypeNotification,
		Action:    "session.message.added",
		Payload:   json.RawMessage(`{`),
		Timestamp: time.Now(),
	}
	if c.sendMessageForSession("session-a", bad) {
		t.Fatal("session send unexpectedly succeeded with invalid JSON payload")
	}
	if got := c.connectionSeq.Load(); got != 0 {
		t.Fatalf("connection seq after marshal failure = %d; want 0", got)
	}
	if got := currentSessionSeq(h, "session-a"); got != 0 {
		t.Fatalf("session seq after marshal failure = %d; want 0", got)
	}

	good, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if !c.sendMessageForSession("session-a", good) {
		t.Fatal("valid session send failed after marshal failure")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionSeq != 1 || got.SessionSeq != 1 {
		t.Fatalf("seqs after marshal failure = connection %d session %d; want 1/1",
			got.ConnectionSeq, got.SessionSeq)
	}
}

func TestClient_SessionJSONMarshalFailureDoesNotAdvanceSessionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-session-json-marshal")
	c.hub = h
	registerTestClient(h, c)

	bad := &ws.Message{
		Type:      ws.MessageTypeNotification,
		Action:    "session.message.added",
		Payload:   json.RawMessage(`{"session_id":"session-a"}`),
		Timestamp: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if c.sendMessageForSession("session-a", bad) {
		t.Fatal("session send unexpectedly succeeded with invalid timestamp")
	}
	if got := c.connectionSeq.Load(); got != 0 {
		t.Fatalf("connection seq after marshal failure = %d; want 0", got)
	}
	if got := currentSessionSeq(h, "session-a"); got != 0 {
		t.Fatalf("session seq after marshal failure = %d; want 0", got)
	}

	good, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if !c.sendMessageForSession("session-a", good) {
		t.Fatal("valid session send failed after marshal failure")
	}
	got := readStampedMessage(t, c)
	if got.ConnectionSeq != 1 || got.SessionSeq != 1 {
		t.Fatalf("seqs after marshal failure = connection %d session %d; want 1/1",
			got.ConnectionSeq, got.SessionSeq)
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
	if gotSession.ConnectionID != "conn-copy" || gotSession.ConnectionSeq != 1 || gotSession.SessionSeq != 1 || gotSession.SessionID != "session-a" {
		t.Fatalf("session stamped message = %+v; want conn-copy seq 1 session seq 1", gotSession)
	}
	if msg.ConnectionID != "caller-owned" || msg.ConnectionSeq != 41 || msg.SessionSeq != 42 {
		t.Fatalf("source message mutated after session send: %+v", msg)
	}

	if !c.sendMessage(msg) {
		t.Fatal("connection send failed")
	}
	gotConnection := readStampedMessage(t, c)
	if gotConnection.ConnectionID != "conn-copy" || gotConnection.ConnectionSeq != 2 || gotConnection.SessionSeq != 0 || gotConnection.SessionID != "" {
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

func TestHub_BroadcastToSessionDoesNotAdvanceSequenceWhenAllRecipientsFull(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-full-session")
	c.hub = h
	registerTestClient(h, c)
	h.SubscribeToSession(c, "session-a")

	for range cap(c.send) {
		c.send <- []byte(`{"type":"notification","action":"preloaded"}`)
	}
	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("session-a", msg)

	for range cap(c.send) {
		<-c.send
	}
	h.BroadcastToSession("session-a", msg)

	got := readStampedMessage(t, c)
	if got.SessionSeq != 1 {
		t.Fatalf("session seq after all-recipient drop = %d; want 1", got.SessionSeq)
	}
}

func TestClient_FailedSessionSendDoesNotAdvanceSessionSequence(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-full-after-check")
	c.hub = h
	registerTestClient(h, c)

	for range cap(c.send) {
		c.send <- []byte(`{"type":"notification","action":"preloaded"}`)
	}
	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	if c.sendMessageForSessionSeq("session-a", 0, msg) {
		t.Fatal("session send unexpectedly succeeded with a full client buffer")
	}
	if got := currentSessionSeq(h, "session-a"); got != 0 {
		t.Fatalf("session seq after failed send = %d; want 0", got)
	}

	for range cap(c.send) {
		<-c.send
	}
	if !c.sendMessageForSessionSeq("session-a", 0, msg) {
		t.Fatal("session send failed after draining buffer")
	}
	got := readStampedMessage(t, c)
	if got.SessionSeq != 1 {
		t.Fatalf("session seq after first successful send = %d; want 1", got.SessionSeq)
	}
}

func TestHub_BroadcastToSessionSequenceSurvivesResubscribe(t *testing.T) {
	h := newTestHub(t)
	c := newTestClient("conn-resubscribe")
	c.hub = h
	registerTestClient(h, c)
	h.SubscribeToSession(c, "session-a")

	msg, err := ws.NewNotification("session.message.added", map[string]string{"session_id": "session-a"})
	if err != nil {
		t.Fatalf("notification: %v", err)
	}
	h.BroadcastToSession("session-a", msg)
	first := readStampedMessage(t, c)

	h.UnsubscribeFromSession(c, "session-a")
	h.SubscribeToSession(c, "session-a")
	h.BroadcastToSession("session-a", msg)
	second := readStampedMessage(t, c)

	if first.SessionSeq != 1 || second.SessionSeq != 2 {
		t.Fatalf("session seqs across resubscribe = %d, %d; want 1, 2", first.SessionSeq, second.SessionSeq)
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

func TestWsSentLogIndexesSessionEntriesAndEvictsFromIndex(t *testing.T) {
	log := newWsSentLogWithCapacity(3)
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	log.Append(1, 1, "session-a", "notification", "session.message.added", base)
	log.Append(2, 1, "session-b", "notification", "session.message.added", base.Add(time.Second))
	log.Append(3, 0, "", "notification", "task.updated", base.Add(2*time.Second))
	log.Append(4, 2, "session-a", "notification", "session.message.updated", base.Add(3*time.Second))

	sessionA := log.SinceForSession("session-a")
	if len(sessionA) != 1 || sessionA[0].ConnectionSeq != 4 || sessionA[0].SessionSeq != 2 {
		t.Fatalf("session-a entries = %+v; want only non-evicted seq 2", sessionA)
	}
	sessionB := log.SinceForSession("session-b")
	if len(sessionB) != 1 || sessionB[0].ConnectionSeq != 2 {
		t.Fatalf("session-b entries = %+v; want connection seq 2", sessionB)
	}
}

func TestWsSentLogClearsEvictedSessionIndexSlot(t *testing.T) {
	log := newWsSentLogWithCapacity(3)
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	log.Append(1, 1, "session-a", "notification", "session.message.added", base)
	log.Append(2, 2, "session-a", "notification", "session.message.updated", base.Add(time.Second))
	log.Append(3, 1, "session-b", "notification", "session.message.added", base.Add(2*time.Second))
	log.Append(4, 2, "session-b", "notification", "session.message.updated", base.Add(3*time.Second))

	log.mu.RLock()
	entries := log.bySession["session-a"]
	if len(entries) != 1 || entries[0].ConnectionSeq != 2 {
		log.mu.RUnlock()
		t.Fatalf("session-a entries = %+v; want only connection seq 2", entries)
	}
	backingEntries := entries[:cap(entries)]
	if len(backingEntries) > len(entries) && backingEntries[len(entries)].Action != "" {
		log.mu.RUnlock()
		t.Fatalf("evicted backing slot was not cleared: %+v", backingEntries[len(entries)])
	}
	log.mu.RUnlock()
}

func TestWsSentLogConcurrentAppendAndRead(t *testing.T) {
	const (
		workers       = 8
		entriesPerRun = 200
		capacity      = 256
	)
	log := newWsSentLogWithCapacity(capacity)
	base := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)
	var nextSeq atomic.Int64
	var wg sync.WaitGroup

	for worker := range workers {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			sessionID := fmt.Sprintf("session-%d", worker%3)
			for i := range entriesPerRun {
				connectionSeq := nextSeq.Add(1)
				entrySessionID := ""
				sessionSeq := int64(0)
				if i%2 == 0 {
					entrySessionID = sessionID
					sessionSeq = int64(i/2 + 1)
				}
				log.Append(
					connectionSeq,
					sessionSeq,
					entrySessionID,
					"notification",
					"session.message.updated",
					base.Add(time.Duration(connectionSeq)*time.Millisecond),
				)
				if i%10 == 0 {
					_ = log.Since(connectionSeq - 20)
					_ = log.SinceForSession(sessionID)
				}
			}
		}(worker)
	}
	wg.Wait()

	events := log.Since(0)
	if len(events) > capacity {
		t.Fatalf("events len = %d; want <= %d", len(events), capacity)
	}
	seen := make(map[int64]bool, len(events))
	for _, event := range events {
		if seen[event.ConnectionSeq] {
			t.Fatalf("duplicate connection seq in sent log: %d", event.ConnectionSeq)
		}
		seen[event.ConnectionSeq] = true
	}
	if got, want := log.MaxConnectionSeq(), int64(workers*entriesPerRun); got != want {
		t.Fatalf("max connection seq = %d; want %d", got, want)
	}
	for sessionIndex := range 3 {
		sessionID := fmt.Sprintf("session-%d", sessionIndex)
		sessionEvents := log.SinceForSession(sessionID)
		for i, event := range sessionEvents {
			if event.SessionID != sessionID {
				t.Fatalf("%s event has session_id=%q", sessionID, event.SessionID)
			}
			if i > 0 && sessionEvents[i-1].SessionSeq > event.SessionSeq {
				t.Fatalf("%s events not sorted by session seq: %+v", sessionID, sessionEvents)
			}
		}
	}
}

func TestWsSentLogRejectsInvalidCapacity(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("newWsSentLogWithCapacity(0) did not panic")
		}
	}()
	_ = newWsSentLogWithCapacity(0)
}

func currentSessionSeq(h *Hub, sessionID string) int64 {
	value, ok := h.sessionSeqs.Load(sessionID)
	if !ok {
		return 0
	}
	return value.(*atomic.Int64).Load()
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
