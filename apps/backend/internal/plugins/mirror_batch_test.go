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

// TestMirrorSyncBatchSkipsRemainingEventsOfAFailedSessionInFlushEach pins
// that flushEach's per-event fallback must not continue to a failed
// session's next queued event. Applied against the freshly-undone (and so,
// again, brand-new) partition, that next event would satisfy
// mirrorGapHealable's newPartition case and "heal" straight over the failed
// sequence — jumping the watermark past it for good, so the failed event
// would never be re-queried on a later sweep. This reproduces the scenario
// with no fault injection: occupy the globally-unique session_events.event_id
// on an unrelated session, then feed the target session seq 1 with that same
// id (applies fine in-memory, UNIQUE-constraint-fails on persist) followed by
// seq 2 in the same flush.
func TestMirrorSyncBatchSkipsRemainingEventsOfAFailedSessionInFlushEach(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	occupied := SessionEvent{
		SessionID: "session-other", TaskID: stringPtr("task-other"), Sequence: 1,
		ID: "shared-event-id", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("other1"), CreatedAt: start,
	}
	appended, err := log.AppendCommitted(occupied)
	require.NoError(t, err)
	require.True(t, appended)

	zSeq1 := SessionEvent{
		SessionID: "session-z", TaskID: stringPtr("task-z"), Sequence: 1,
		// Collides with "session-other"'s durably committed event_id: the
		// in-memory apply cannot see it (dup/gap checks are per-session), so
		// only the durable INSERT fails.
		ID: "shared-event-id", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("z1"), CreatedAt: start,
	}
	zSeq2 := SessionEvent{
		SessionID: "session-z", TaskID: stringPtr("task-z"), Sequence: 2,
		ID: "session-z:2", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("z2"), CreatedAt: start,
	}
	good := SessionEvent{
		SessionID: "session-good", TaskID: stringPtr("task-good"), Sequence: 1,
		ID: "session-good:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("good1"), CreatedAt: start,
	}

	// One batch spanning session-z (bad) and session-good (fine) forces
	// AppendCommittedBatch to fail on zSeq1's persist and mirrorSyncBatch to
	// fall back to flushEach.
	batch := newMirrorSyncBatch(log, 256, true)
	batch.add(zSeq1)
	batch.add(zSeq2)
	batch.add(good)
	batch.flush()

	require.Len(t, batch.failures, 1)
	require.Contains(t, batch.failures[0], "session-z")
	require.Equal(t, uint64(0), log.Watermark("session-z"))
	require.Equal(t, uint64(1), log.Watermark("session-good"))

	require.NoError(t, log.Close())
	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	eventsZ, _ := reopened.EventsAfter("session-z", 0)
	// Not []SessionEvent{zSeq2}: that would mean zSeq1 was silently dropped
	// and the watermark advanced past it without ever re-querying it.
	require.Empty(t, eventsZ)
}

// TestAppendCommittedBatchUndoesAllAppliedEventsOnPersistFailure proves the
// other half of AppendCommittedBatch's all-or-nothing contract: when every
// event applies cleanly in-memory (no dup/gap/terminal rejection) but the
// durable transaction itself fails, undo() still unwinds every outcome the
// call accumulated, not just the ones a mid-loop apply failure would have
// left behind. Duplicate/gap/terminal checks are keyed by session, so a
// same-event_id collision across two different (both brand-new) sessions is
// invisible until the INSERT runs — exactly the shape that reaches the
// persist-time undo() path instead of the apply-time one.
func TestAppendCommittedBatchUndoesAllAppliedEventsOnPersistFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	occupied := SessionEvent{
		SessionID: "session-occupied", TaskID: stringPtr("task-occupied"), Sequence: 1,
		ID: "shared-event-id", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("occupied"), CreatedAt: start,
	}
	appended, err := log.AppendCommitted(occupied)
	require.NoError(t, err)
	require.True(t, appended)

	eventD := SessionEvent{
		SessionID: "session-d", TaskID: stringPtr("task-d"), Sequence: 1,
		// Collides with "session-occupied"'s durably committed event_id.
		ID: "shared-event-id", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("d1"), CreatedAt: start,
	}
	eventE := SessionEvent{
		SessionID: "session-e", TaskID: stringPtr("task-e"), Sequence: 1,
		ID: "session-e:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("e1"), CreatedAt: start,
	}

	appendedBatch, err := log.AppendCommittedBatch([]SessionEvent{eventE, eventD})
	require.Error(t, err)
	require.Nil(t, appendedBatch)
	require.Equal(t, uint64(0), log.Watermark("session-d"))
	require.Equal(t, uint64(0), log.Watermark("session-e"))

	require.NoError(t, log.Close())
	reopened, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	eventsD, _ := reopened.EventsAfter("session-d", 0)
	eventsE, _ := reopened.EventsAfter("session-e", 0)
	require.Empty(t, eventsD)
	require.Empty(t, eventsE)
	occupiedEvents, _ := reopened.EventsAfter("session-occupied", 0)
	require.Len(t, occupiedEvents, 1)
}

// TestAppendCommittedBatchDuplicateHandling covers AppendCommittedBatch's
// two branches that TestAppendCommittedBatchFallsBackToPerEventOnApplyFailure
// and TestAppendCommittedBatchUndoesAllAppliedEventsOnPersistFailure do not
// reach: a duplicate event (already durably committed) applies with no error
// and outcome.appended=false, so it contributes nothing to persist.
func TestAppendCommittedBatchDuplicateHandling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session-events.db")
	log, err := NewSessionEventLog(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	start := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	original := SessionEvent{
		SessionID: "session-x", TaskID: stringPtr("task-x"), Sequence: 1,
		ID: "session-x:1", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
		Payload: validMessageAddedPayload("x1"), CreatedAt: start,
	}
	appended, err := log.AppendCommitted(original)
	require.NoError(t, err)
	require.True(t, appended)

	t.Run("all duplicates short-circuits with no persist attempt", func(t *testing.T) {
		result, err := log.AppendCommittedBatch([]SessionEvent{original})
		require.NoError(t, err)
		require.Nil(t, result)
		require.Equal(t, uint64(1), log.Watermark("session-x"))
	})

	t.Run("mixed duplicate and new event persists only the new one", func(t *testing.T) {
		next := SessionEvent{
			SessionID: "session-x", TaskID: stringPtr("task-x"), Sequence: 2,
			ID: "session-x:2", ProtocolVersion: SessionEventProtocolVersion, EventType: "message.added",
			Payload: validMessageAddedPayload("x2"), CreatedAt: start,
		}
		result, err := log.AppendCommittedBatch([]SessionEvent{original, next})
		require.NoError(t, err)
		require.Equal(t, []SessionEvent{next}, result)
		require.Equal(t, uint64(2), log.Watermark("session-x"))

		require.NoError(t, log.Close())
		reopened, err := NewSessionEventLog(path)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, reopened.Close()) })
		events, _ := reopened.EventsAfter("session-x", 0)
		require.Len(t, events, 2)
	})
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
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, mirrored, 4) // exactly the first two sessions' two events each

	// The durable log agrees: the two swept sessions are fully mirrored,
	// the two the cancellation stopped before reaching are absent.
	require.NoError(t, service.sessionEvents.Close())
	reopened, err := NewSessionEventLog(filepath.Join(dir, "session-events.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	for _, sessionID := range sessionIDs[:2] {
		events, _ := reopened.EventsAfter(sessionID, 0)
		require.Len(t, events, 2)
	}
	for _, sessionID := range sessionIDs[2:] {
		events, _ := reopened.EventsAfter(sessionID, 0)
		require.Empty(t, events)
	}
	service.sessionEvents = reopened

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
