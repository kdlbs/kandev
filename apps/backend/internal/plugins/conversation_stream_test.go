package plugins

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSessionEventLogPersistsSequenceAndTerminalRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	first, err := log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	removed, err := log.Append("session-1", stringPtr("task-1"), "session.removed", validSessionRemovedPayload())
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Sequence)
	require.Equal(t, uint64(2), removed.Sequence)
	require.NoError(t, log.Close())

	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	events, terminal := reopened.EventsAfter("session-1", 0)
	require.True(t, terminal)
	require.Equal(t, []uint64{1, 2}, []uint64{events[0].Sequence, events[1].Sequence})
	_, err = reopened.Append("session-1", stringPtr("task-1"), "message.updated", json.RawMessage(`{"type":"message.updated","id":"m1"}`))
	require.ErrorIs(t, err, ErrSessionRemoved)
}

func TestSessionEventLogPoisonBlocksAckUntilAuditedRequeue(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	dispatcher := NewSessionDeliveryDispatcher(log)
	_, err = log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	poison, err := log.Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	cursor := testCursor()
	require.NoError(t, log.RegisterCursor(cursor, 0))
	require.NoError(t, log.Acknowledge(cursor, 1))
	require.ErrorIs(t, log.Acknowledge(cursor, 2), ErrPoisonEvent)
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for attempt := range SessionPoisonMaxAttempts {
		claimAndCompletePoison(t, dispatcher, "session-1", poison.ID, base.Add(time.Duration(attempt)*time.Minute), true)
	}
	record, ok := log.Poison("session-1", poison.ID)
	require.True(t, ok)
	require.Equal(t, SessionPoisonExhausted, record.State)
	require.Equal(t, SessionPoisonMaxAttempts, record.Attempts)
	require.ErrorIs(t, dispatcher.Requeue("session-1", poison.ID, record.OwnerEpoch+1, "admin-1", base), ErrStaleOwnerEpoch)
	require.NoError(t, dispatcher.Requeue("session-1", poison.ID, record.OwnerEpoch, "admin-1", base))
}

func TestSessionEventPoisonDeliveryClaimControlsLeaseBackoffExhaustionAndRequeue(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	dispatcher := NewSessionDeliveryDispatcher(log)
	plain, err := log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("plain"))
	require.NoError(t, err)
	plainClaim, err := dispatcher.Claim("session-1", plain.ID, time.Now())
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryUntracked, plainClaim.Disposition)

	poison, err := log.Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	first := claimAndCompletePoison(t, dispatcher, "session-1", poison.ID, base, true)
	require.Equal(t, SessionDeliveryClaimed, first.Disposition)
	leased, err := dispatcher.Claim("session-1", poison.ID, base.Add(time.Second))
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryLeased, leased.Disposition)
	backoff, err := dispatcher.Claim("session-1", poison.ID, base.Add(SessionPoisonLease))
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryBackoff, backoff.Disposition)

	for attempt := 1; attempt < SessionPoisonMaxAttempts; attempt++ {
		claimAndCompletePoison(t, dispatcher, "session-1", poison.ID, base.Add(time.Duration(attempt)*time.Minute), true)
	}
	exhausted, err := dispatcher.Claim("session-1", poison.ID, base.Add(10*time.Minute))
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryExhausted, exhausted.Disposition)
	record, ok := log.Poison("session-1", poison.ID)
	require.True(t, ok)
	require.Equal(t, SessionPoisonMaxAttempts, record.Attempts)

	require.NoError(t, dispatcher.Requeue("session-1", poison.ID, record.OwnerEpoch, "admin-1", base.Add(11*time.Minute)))
	requeued := claimAndCompletePoison(t, dispatcher, "session-1", poison.ID, base.Add(11*time.Minute), true)
	require.Equal(t, SessionDeliveryClaimed, requeued.Disposition)
	record, ok = log.Poison("session-1", poison.ID)
	require.True(t, ok)
	require.Equal(t, 1, record.Attempts)
}

func claimAndCompletePoison(
	t *testing.T,
	dispatcher *SessionDeliveryDispatcher,
	sessionID string,
	eventID string,
	at time.Time,
	queued bool,
) SessionDeliveryClaim {
	t.Helper()
	claim, err := dispatcher.Claim(sessionID, eventID, at)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryClaimed, claim.Disposition)
	require.NoError(t, dispatcher.Complete(claim, queued, at))
	return claim
}

func TestSessionEventRetentionKeepsTerminalTombstoneThroughBothBounds(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return start }
	_, err = log.Append("session-1", stringPtr("task-1"), "session.removed", validSessionRemovedPayload())
	require.NoError(t, err)
	require.NoError(t, log.CollectExpired(start.Add(SessionEventRetention-time.Second)))
	_, terminal := log.EventsAfter("session-1", 0)
	require.True(t, terminal)
	require.NoError(t, log.CollectExpired(start.Add(SessionEventRetention)))
	events, terminal := log.EventsAfter("session-1", 0)
	require.False(t, terminal)
	require.Empty(t, events)
}

func TestSessionEventResyncAfterTerminalCollectionHealsForwardGap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return start }
	removed, err := log.AppendCommitted(SessionEvent{
		SessionID: "session-1", TaskID: stringPtr("task-1"), Sequence: 3,
		ID: "session-1:3", ProtocolVersion: SessionEventProtocolVersion, EventType: "session.removed",
		Payload: validSessionRemovedPayload(), CreatedAt: start,
	})
	require.NoError(t, err)
	require.True(t, removed)

	// After retention both bounds lapse the local partition is dropped: the
	// terminal tombstone is gone from the durable log.
	require.NoError(t, log.CollectExpired(start.Add(SessionEventRetention+time.Hour)))
	_, terminal := log.EventsAfter("session-1", 0)
	require.False(t, terminal)

	// The primary journal keeps only its session.removed row (all other rows
	// are pruned), which is not at sequence 1. Re-mirroring it must heal the
	// gap instead of failing forever.
	removed, err = log.AppendCommitted(SessionEvent{
		SessionID: "session-1", TaskID: stringPtr("task-1"), Sequence: 3,
		ID: "session-1:3", ProtocolVersion: SessionEventProtocolVersion, EventType: "session.removed",
		Payload: validSessionRemovedPayload(), CreatedAt: start.Add(time.Hour),
	})
	require.NoError(t, err)
	require.True(t, removed)
	events, terminal := log.EventsAfter("session-1", 0)
	require.True(t, terminal)
	require.Len(t, events, 1)
	require.Equal(t, uint64(3), events[0].Sequence)
}

func TestSessionEventResyncAfterTruncationHealsForwardGap(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return start }
	// seq 1 and 2 mirror fine; then the idle partition's rows age out and are
	// truncated while the primary keeps a newer row at seq 4 (seq 3 pruned).
	for _, sequence := range []uint64{1, 2} {
		_, err := log.AppendCommitted(SessionEvent{
			SessionID: "session-1", TaskID: stringPtr("task-1"), Sequence: sequence,
			ID: "session-1:" + strconv.FormatUint(sequence, 10), ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
			Payload: validMessageAddedPayload("m" + strconv.FormatUint(sequence, 10)), CreatedAt: start,
		})
		require.NoError(t, err)
	}
	require.NoError(t, log.CollectExpired(start.Add(SessionEventRetention+time.Hour)))
	_, terminal := log.EventsAfter("session-1", 0)
	require.False(t, terminal)
	require.Equal(t, uint64(2), log.Watermark("session-1"))

	appended, err := log.AppendCommitted(SessionEvent{
		SessionID: "session-1", TaskID: stringPtr("task-1"), Sequence: 4,
		ID: "session-1:4", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("m4"), CreatedAt: start.Add(time.Hour),
	})
	require.NoError(t, err)
	require.True(t, appended)
	require.Equal(t, uint64(4), log.Watermark("session-1"))
}

func TestSessionEventRetentionExpiresExhaustedPoisonState(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return start }
	event, err := log.Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	dispatcher := NewSessionDeliveryDispatcher(log)
	for attempt := range SessionPoisonMaxAttempts {
		claimAndCompletePoison(t, dispatcher, "session-1", event.ID, start.Add(time.Duration(attempt)*time.Minute), true)
	}
	require.NoError(t, log.CollectExpired(start.Add(SessionEventRetention+5*time.Minute)))
	_, ok := log.Poison("session-1", event.ID)
	require.False(t, ok)
	events, terminal := log.EventsAfter("session-1", 0)
	require.False(t, terminal)
	require.Empty(t, events)
}

func TestSessionEventCursorRegistrationAndExplicitRelease(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	event, err := log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	key := testCursor()
	require.NoError(t, log.RegisterCursor(key, event.Sequence))
	require.Contains(t, log.state.Cursors, deliveryCursorKey(key))
	require.NoError(t, log.ReleaseCursor(key))
	require.NotContains(t, log.state.Cursors, deliveryCursorKey(key))
}

func TestSessionEventCursorReplacementAdvancesExistingCursor(t *testing.T) {
	log, err := NewSessionEventLog("")
	require.NoError(t, err)
	event, err := log.Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	key := testCursor()
	require.NoError(t, log.RegisterCursor(key, 0))
	require.NoError(t, log.ReplaceCursor(key, event.Sequence))
	require.NoError(t, log.Acknowledge(key, event.Sequence))
}

func TestSessionEventCursorLifecycleFieldsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	key := testCursor()
	key.Generation = 4
	require.NoError(t, log.RegisterCursor(key, 2))
	leaseUntil := time.Date(2026, 9, 7, 12, 5, 0, 0, time.UTC)
	retryAt := time.Date(2026, 9, 7, 12, 6, 0, 0, time.UTC)
	log.mu.Lock()
	cursor := log.state.Cursors[deliveryCursorKey(key)]
	cursor.LeaseUntil = leaseUntil
	cursor.RetryAt = retryAt
	cursor.OwnerEpoch = 9
	err = log.persistLocked()
	log.mu.Unlock()
	require.NoError(t, err)

	require.NoError(t, log.Close())

	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	restored := reopened.state.Cursors[deliveryCursorKey(key)]
	require.Equal(t, uint64(9), restored.OwnerEpoch)
	require.Equal(t, leaseUntil, restored.LeaseUntil)
	require.Equal(t, retryAt, restored.RetryAt)
}

func TestSessionEventMaintenanceDoesNotCountUndeliveredPoisonAttempts(t *testing.T) {
	service := NewService(nil, NewRegistry(), nil, testLogger(t))
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	poison, err := service.SessionEvents().Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	// Maintenance runs many times; without any actual delivery attempt the
	// poison must stay pending at zero attempts instead of being exhausted.
	for _, at := range []time.Time{start, start.Add(SessionPoisonLease), start.Add(2 * SessionPoisonLease)} {
		require.NoError(t, service.maintainSessionEvents(context.Background(), at))
		record, ok := service.SessionEvents().Poison("session-1", poison.ID)
		require.True(t, ok)
		require.Equal(t, 0, record.Attempts)
		require.Equal(t, SessionPoisonPending, record.State)
	}
	// A real queued delivery advances the attempt bookkeeping.
	claimAndCompletePoison(t, service.SessionDelivery(), "session-1", poison.ID, start.Add(3*SessionPoisonLease), true)
	record, ok := service.SessionEvents().Poison("session-1", poison.ID)
	require.True(t, ok)
	require.Equal(t, 1, record.Attempts)
	require.Equal(t, SessionPoisonLeased, record.State)
}

func TestSessionEventCursorReplacementRestoresMemoryOnPersistFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	key := testCursor()
	require.NoError(t, log.RegisterCursor(key, 0))
	event, err := log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	// Close the held database before redirecting the path so persistence
	// failures exercise the closed-log guard.
	require.NoError(t, log.Close())
	log.path = filepath.Join(t.TempDir(), "missing", "session-events.db")

	require.Error(t, log.ReplaceCursor(key, event.Sequence))

	// The in-memory mutation rolled back, so a restart sees the committed
	// acknowledgement, not the failed replacement.
	restored := log.state.Cursors[deliveryCursorKey(key)]
	require.Equal(t, uint64(0), restored.AcknowledgedSequence)
	require.Equal(t, uint64(1), restored.OwnerEpoch)
	require.NoError(t, log.Close())

	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	require.Equal(t, uint64(0), reopened.state.Cursors[deliveryCursorKey(key)].AcknowledgedSequence)
}

func TestSessionEventPoisonAttemptRestoresMemoryOnPersistFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	dispatcher := NewSessionDeliveryDispatcher(log)
	poison, err := log.Append("session-1", stringPtr("task-1"), "unknown.event", json.RawMessage(`{"type":"unknown.event","session_id":"session-1","task_id":"task-1"}`))
	require.NoError(t, err)
	require.NoError(t, log.Close())
	log.path = filepath.Join(t.TempDir(), "missing", "session-events.db")

	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	_, err = dispatcher.Claim("session-1", poison.ID, base)
	require.Error(t, err)

	record, ok := log.Poison("session-1", poison.ID)
	require.True(t, ok)
	require.Equal(t, 0, record.Attempts)
	require.Equal(t, SessionPoisonPending, record.State)
}

func TestSessionEventCollectionRestoresMemoryOnPersistFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return start }
	_, err = log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	_, err = log.Append("session-1", stringPtr("task-1"), "session.removed", validSessionRemovedPayload())
	require.NoError(t, err)
	require.NoError(t, log.RegisterCursor(testCursor(), 0))
	require.NoError(t, log.Close())
	log.path = filepath.Join(t.TempDir(), "missing", "session-events.db")

	require.Error(t, log.CollectExpired(start.Add(SessionEventRetention+time.Hour)))

	// Nothing was collected in memory: the cursor, poison, and partition
	// survive until persistence actually commits.
	require.True(t, log.HasCursor(testCursor()))
	events, terminal := log.EventsAfter("session-1", 0)
	require.True(t, terminal)
	require.Len(t, events, 2)
}

func testCursor() SessionDeliveryCursorKey {
	return SessionDeliveryCursorKey{SessionID: "session-1", ConsumerKind: "plugin", ConsumerID: "consumer-1", PluginID: "plugin-1", Generation: 1, UserID: "user-1"}
}

func stringPtr(value string) *string { return &value }

func validMessageAddedPayload(messageID string) json.RawMessage {
	return json.RawMessage(`{"type":"message.added","session_id":"session-1","task_id":"task-1","message_id":"` + messageID + `","author_type":"user","content":"hello","message_type":"message","created_at":"2026-09-07T12:00:00Z"}`)
}

func validSessionRemovedPayload() json.RawMessage {
	return json.RawMessage(`{"type":"session.removed","session_id":"session-1","task_id":"task-1"}`)
}

func TestSessionEventAcknowledgePersistsOnlyTheCursorRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	event, err := log.Append("session-1", stringPtr("task-1"), "message.added", validMessageAddedPayload("m1"))
	require.NoError(t, err)
	_, err = log.AppendCommitted(SessionEvent{
		SessionID: "session-1", TaskID: stringPtr("task-1"), Sequence: event.Sequence + 1,
		ID: "session-1:2", EventType: "message.added", ProtocolVersion: 1,
		Payload: validMessageAddedPayload("m2"), CreatedAt: event.CreatedAt,
	})
	require.NoError(t, err)
	cursorA := testCursor()
	cursorA.ConsumerID = "consumer-a"
	cursorB := testCursor()
	cursorB.ConsumerID = "consumer-b"
	require.NoError(t, log.RegisterCursor(cursorA, 0))
	require.NoError(t, log.RegisterCursor(cursorB, 0))
	require.NoError(t, log.Acknowledge(cursorA, 2))

	require.NoError(t, log.Close())
	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	restoredA := reopened.state.Cursors[deliveryCursorKey(cursorA)]
	require.Equal(t, uint64(2), restoredA.AcknowledgedSequence)
	restoredB := reopened.state.Cursors[deliveryCursorKey(cursorB)]
	require.Equal(t, uint64(0), restoredB.AcknowledgedSequence)
	// The partition and both event rows survive the delta write untouched.
	events, terminal := reopened.EventsAfter("session-1", 0)
	require.False(t, terminal)
	require.Len(t, events, 2)
	require.Equal(t, uint64(2), reopened.Watermark("session-1"))
}
