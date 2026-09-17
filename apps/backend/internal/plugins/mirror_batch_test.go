package plugins

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestAppendCommittedBatchFallsBackToPerEventOnApplyFailure proves the first
// correctness constraint on cross-session batching: when applying one event
// in a batch fails (here, an unhealable forward gap for session-b), the
// caller's per-event fallback still durably mirrors every other session's
// otherwise-good events in that same batch instead of losing them to the
// batch's all-or-nothing rollback.
func TestAppendCommittedBatchFallsBackToPerEventOnApplyFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	eventA1 := SessionEvent{
		SessionID: "session-a", TaskID: stringPtr("task-a"), Sequence: 1,
		ID: "session-a:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("a1"), CreatedAt: start,
	}
	eventB1 := SessionEvent{
		SessionID: "session-b", TaskID: stringPtr("task-b"), Sequence: 1,
		ID: "session-b:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("b1"), CreatedAt: start,
	}
	// session-b's second event jumps to sequence 5 against a non-empty,
	// non-terminal partition: mirrorGapHealable rejects that, so applying
	// this event inside the batch fails with ErrForwardGap.
	eventB5 := SessionEvent{
		SessionID: "session-b", TaskID: stringPtr("task-b"), Sequence: 5,
		ID: "session-b:5", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("b5"), CreatedAt: start,
	}
	eventC1 := SessionEvent{
		SessionID: "session-c", TaskID: stringPtr("task-c"), Sequence: 1,
		ID: "session-c:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("c1"), CreatedAt: start,
	}

	// The whole-batch path fails and undoes everything it had applied
	// in-memory so far (session-a and session-b's first event), including
	// events for sessions that had nothing wrong with them.
	appended, err := log.AppendCommittedBatch([]SessionEvent{eventA1, eventB1, eventB5, eventC1})
	require.ErrorIs(t, err, ErrForwardGap)
	require.Nil(t, appended)
	require.Equal(t, uint64(0), log.Watermark("session-a"))
	require.Equal(t, uint64(0), log.Watermark("session-b"))
	require.Equal(t, uint64(0), log.Watermark("session-c"))

	// mirrorSyncBatch is the production caller that recovers from exactly
	// this failure: it retries the same events one at a time so session-a
	// and session-c's good writes still land, and only session-b's bad
	// event is reported as a failure.
	batch := newMirrorSyncBatch(log, 256, true)
	batch.add(eventA1)
	batch.add(eventB1)
	batch.add(eventB5)
	batch.add(eventC1)
	batch.flush()

	require.Equal(t, uint64(1), log.Watermark("session-a"))
	require.Equal(t, uint64(1), log.Watermark("session-b"))
	require.Equal(t, uint64(1), log.Watermark("session-c"))
	require.Len(t, batch.mirrored, 3)
	require.Len(t, batch.failures, 1)
	require.Contains(t, batch.failures[0], "session-b")

	// The durable log agrees: reopening it sees exactly the three good
	// events, not four and not zero.
	require.NoError(t, log.Close())
	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	eventsA, _ := reopened.EventsAfter("session-a", 0)
	eventsB, _ := reopened.EventsAfter("session-b", 0)
	eventsC, _ := reopened.EventsAfter("session-c", 0)
	require.Len(t, eventsA, 1)
	require.Len(t, eventsB, 1)
	require.Len(t, eventsC, 1)
}

// cancelAfterN reports ctx.Err() as nil for its first n calls and then as
// context.Canceled forever after, letting a test deterministically cancel a
// loop after a specific number of iterations without racing a timer.
type cancelAfterN struct {
	context.Context
	remaining int32
}

func (c *cancelAfterN) Err() error {
	if atomic.AddInt32(&c.remaining, -1) < 0 {
		return context.Canceled
	}
	return nil
}

// TestMirrorSweepCancellationStopsBetweenSessionsWithoutAdvancingPastCommit
// proves the second correctness constraint: cancelling the cross-session
// sweep partway through leaves already-committed sessions exactly at their
// durably mirrored watermark (nothing skipped ahead, nothing partially
// applied), untouched sessions are simply not attempted, and resuming with a
// fresh context finishes the sweep with no loss or duplication.
func TestMirrorSweepCancellationStopsBetweenSessionsWithoutAdvancingPastCommit(t *testing.T) {
	dir := t.TempDir()
	journal := seedPrimaryJournalFixture(t, filepath.Join(dir, "journal.db"), mirrorFixtureShape{
		name: "cancel-fixture", sessions: 4, eventsPerSession: 2,
	})
	service := newMirrorService(t, journal, filepath.Join(dir, "session-events.sqlite"))
	sessionIDs := []string{"session-000000", "session-000001", "session-000002", "session-000003"}

	ctx := &cancelAfterN{Context: context.Background(), remaining: 2}
	mirrored, err := service.sweepCommittedSessionEvents(ctx, true)
	require.NoError(t, err)
	require.Len(t, mirrored, 4) // exactly the first two sessions' two events each

	require.Equal(t, uint64(2), service.sessionEvents.Watermark(sessionIDs[0]))
	require.Equal(t, uint64(2), service.sessionEvents.Watermark(sessionIDs[1]))
	require.Equal(t, uint64(0), service.sessionEvents.Watermark(sessionIDs[2]))
	require.Equal(t, uint64(0), service.sessionEvents.Watermark(sessionIDs[3]))

	resumed, err := service.sweepCommittedSessionEvents(context.Background(), true)
	require.NoError(t, err)
	require.Len(t, resumed, 4) // the two sessions the cancelled sweep never reached
	for _, sessionID := range sessionIDs {
		require.Equal(t, uint64(2), service.sessionEvents.Watermark(sessionID))
	}
}
