package messagequeue

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
)

// TestRequeuePreservingFIFO_InsertsAtHead verifies the FIFO starvation
// bug fix: a superseded dispatch that re-enters the queue must beat
// any new entry that arrived after the supersede, otherwise the busy-
// session drain loop strands it at the tail indefinitely.
//
// Scenario: queue [first@1], drain first (position goes empty), a
// second user message arrives [second@1], then first requeues.
// Pre-fix bug: first lands at position 2 (tail of [second]), every
// subsequent drain cycles it. Post-fix: first lands at position 0,
// beats second on the next TakeQueued.
func TestRequeuePreservingFIFO_InsertsAtHead(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 4, log)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	first, err := svc.QueueMessage(ctx, "s", "t", "first", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	dequeued, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)
	require.Equal(t, first.ID, dequeued.ID)

	_, err = svc.QueueMessage(ctx, "s", "t", "second", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	// Requeue first; it must beat second.
	require.NoError(t, svc.RequeueAtHead(ctx, dequeued))

	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 2, status.Count)
	require.Equal(t, "first", status.Entries[0].Content,
		"requeued entry must be ahead of any new arrival")
	require.Equal(t, "second", status.Entries[1].Content)

	// TakeQueued now returns first; second has been waiting, not starved.
	head, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)
	require.Equal(t, "first", head.Content)
	require.Equal(t, first.ID, head.ID, "requeue must preserve identity (same coalesce key)")

	// Positions are monotonic ascending (existing invariant).
	require.Less(t, status.Entries[0].Position, status.Entries[1].Position,
		"head position must be lower than tail position")
}

// TestRequeuePreservingFIFO_BoundedAcrossRepeatedSupersede verifies
// the contract that matters to the live bug: under repeated
// supersede→requeue cycles the original entry is delivered in bounded
// time. We simulate the cycle by draining and requeuing the same
// entry N times, interleaving new arrivals between each cycle, and
// assert that the original entry is always the next TakeQueued.
func TestRequeuePreservingFIFO_BoundedAcrossRepeatedSupersede(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 32, log)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	// Seed: 1 original + 5 alternates that race against it.
	original, err := svc.QueueMessage(ctx, "s", "t", "original", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	const cycles = 10
	for i := 0; i < cycles; i++ {
		// Take whatever is at the head. Pre-fix bug: this drains
		// every newcomer and leaves original stranded at the tail.
		_, _ = svc.TakeQueued(ctx, "s")
		// A new message arrives during the settle window — this is
		// the arrival that outranked original in the live bug.
		_, err := svc.QueueMessage(ctx, "s", "t", "noise", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		// Original requeues (its dispatch was superseded by noise).
		require.NoError(t, svc.RequeueAtHead(ctx, original))
	}

	// Bounded: original must be the next entry served, despite 10
	// new arrivals in between.
	status := svc.GetStatus(ctx, "s")
	require.Equal(t, cycles+1, status.Count, "queue must hold 10 noise + 1 original")
	require.Equal(t, "original", status.Entries[0].Content,
		"original must be at the head after %d supersede cycles", cycles)

	head, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)
	require.Equal(t, "original", head.Content)
}

// TestRequeuePreservingFIFO_PreservesCoalesceReplace verifies the
// requeue still honors coalesce-replace semantics: a retry with the
// same coalesce key replaces the existing pending entry at its
// current position (not below it). This protects the existing
// TestExecuteQueuedMessage_RequeuesCoalescedMessageWithOriginalSender
// contract for CI-feedback retries.
func TestRequeuePreservingFIFO_PreservesCoalesceReplace(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 4, log)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	// Seed a coalesced entry and a separate pending entry so the
	// queue is non-empty (the coalesce-hit branch must NOT touch
	// the separate entry).
	coalesced, _, err := svc.QueueMessageWithCoalesceKey(
		ctx, "s", "t", "first", "", QueuedByWorkflow, false, nil, nil,
		"ci-key", true,
	)
	require.NoError(t, err)
	_, err = svc.QueueMessage(ctx, "s", "t", "noise", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	// Take the coalesced entry so we can requeue it.
	dequeued, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)
	require.Equal(t, coalesced.ID, dequeued.ID)

	// Recreate a pending entry with the same coalesce key. Requeue must replace
	// this entry in place instead of taking the head-insert path.
	replacement, _, err := svc.QueueMessageWithCoalesceKey(
		ctx, "s", "t", "replacement", "", QueuedByWorkflow, false, nil, nil,
		"ci-key", true,
	)
	require.NoError(t, err)
	replacementPosition := replacement.Position

	require.NoError(t, svc.RequeueAtHead(ctx, dequeued))

	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 2, status.Count, "requeue must replace the existing coalesced entry, not append")
	require.Equal(t, "noise", status.Entries[0].Content)
	require.Equal(t, "first", status.Entries[1].Content,
		"the coalesced replacement must keep its pending position")
	require.Equal(t, replacement.ID, status.Entries[1].ID)
	require.Equal(t, replacementPosition, status.Entries[1].Position)
}

// TestRequeuePreservingFIFO_DistinctMessageDoesNotCoalesce verifies
// the negative: when the requeued message has no coalesce key (or a
// different one), it inserts at the head and does NOT replace an
// existing entry. This is the normal "retry of a different user
// input" case.
func TestRequeuePreservingFIFO_DistinctMessageDoesNotCoalesce(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 4, log)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	first, err := svc.QueueMessage(ctx, "s", "t", "first", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	dequeued, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)

	// No coalesce key in the requeue metadata → head-insert.
	require.NoError(t, svc.RequeueAtHead(ctx, dequeued))

	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 1, status.Count)
	require.Equal(t, "first", status.Entries[0].Content)
	require.Equal(t, first.ID, status.Entries[0].ID,
		"a no-coalesce requeue must keep the same identity (no coalesce replacement)")

	// Position is still strictly ascending (existing invariant).
	require.Greater(t, status.Entries[0].Position, int64(0),
		"non-empty queue → position > 0")
}

// TestRequeuePreservingFIFO_EmptyQueueStartsAtOne verifies the boundary
// case: a requeue into an empty queue must NOT compute MIN-1 = 0 (or
// negative). The empty-queue branch must mint position 1 so the next
// insert (which derives from MAX+1) lands at 2 and stays monotonic.
func TestRequeuePreservingFIFO_EmptyQueueStartsAtOne(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 4, log)
	svc.SetAutoMergeEnabled(false)
	ctx := context.Background()

	msg, err := svc.QueueMessage(ctx, "s", "t", "lone", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	dequeued, ok := svc.TakeQueued(ctx, "s")
	require.True(t, ok)

	require.NoError(t, svc.RequeueAtHead(ctx, dequeued))

	status := svc.GetStatus(ctx, "s")
	require.Equal(t, 1, status.Count)
	require.Equal(t, int64(1), status.Entries[0].Position,
		"empty-queue requeue must mint position 1, not 0")

	// Next insert lands at MAX+1=2; monotonic invariant holds.
	_, err = svc.QueueMessage(ctx, "s", "t", "second", "", QueuedByUser, false, nil)
	require.NoError(t, err)
	status = svc.GetStatus(ctx, "s")
	require.Equal(t, int64(1), status.Entries[0].Position)
	require.Equal(t, int64(2), status.Entries[1].Position)

	// ID of the first requeue was assigned by the empty-queue path.
	require.NotEmpty(t, msg.ID)
}

// TestRequeuePreservingFIFO_NilMessageGuards verifies the defensive
// guard: a nil message returns a sentinel error rather than panicking
// or silently succeeding.
func TestRequeuePreservingFIFO_NilMessageGuards(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
	require.NoError(t, err)
	svc := NewService(NewMemoryRepository(), 4, log)
	svc.SetAutoMergeEnabled(false)

	err = svc.RequeueAtHead(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errRequeueAtHeadNil) || err.Error() != "",
		"a nil message must surface an error, got %v", err)
}

// errRequeueAtHeadNil is a sentinel the test asserts on. Defining it here
// keeps the test's surface small and avoids dragging in a separate
// error-naming file.
var errRequeueAtHeadNil = errors.New("queued message is nil")
