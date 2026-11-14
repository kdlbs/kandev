package journal

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAbandonSuccessorStreamRemovesMetadataForDeterministicRetry(t *testing.T) {
	ctx := context.Background()
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	_, err = store.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2", Identity: "event-1", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	expected, err := store.GetStream(ctx, "incarnation:g2")
	require.NoError(t, err)

	streams, err := store.ListSuccessorStreams(ctx, "session", "incarnation", 1)
	require.NoError(t, err)
	require.Equal(t, []Stream{expected}, streams)
	streams, err = store.ListSuccessorStreams(ctx, "other-session", "incarnation", 1)
	require.NoError(t, err)
	require.Empty(t, streams)

	require.NoError(t, store.AbandonStream(ctx, expected))
	_, err = store.GetStream(ctx, expected.StreamID)
	require.ErrorIs(t, err, ErrStreamNotFound)
	resolved, err := store.HasUnresolvedWork(ctx)
	require.NoError(t, err)
	require.False(t, resolved)
	retried, err := store.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: expected.StreamID, Identity: "event-1", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, retried.Sequence)
	events, _, err := store.Replay(ctx, expected.StreamID, 0, 10)
	require.NoError(t, err)
	require.Equal(t, []Event{retried}, events)
}

func TestAbandonSuccessorStreamAccountsForMixedEventPayloads(t *testing.T) {
	ctx := context.Background()
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	events, err := store.AppendBatch(ctx, []Event{
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "incarnation:g2", Type: "shell_output_update", Payload: []byte(`{"version":1,"patch":"output"}`)},
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "incarnation:g2", Type: "message_chunk", Payload: []byte(`{"message_id":"m1","segment":"first"}`)},
		{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "incarnation:g2", Type: "message_chunk", Payload: []byte(`{"message_id":"m1","segment":"second"}`)},
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.NoError(t, store.Acknowledge(ctx, "incarnation:g2", events[0].Sequence))
	stream, err := store.GetStream(ctx, "incarnation:g2")
	require.NoError(t, err)
	require.Greater(t, stream.Bytes, int64(0))
	require.EqualValues(t, 3, stream.HighWater)
	require.EqualValues(t, 1, stream.Acknowledged)

	require.NoError(t, store.AbandonStream(ctx, stream))
	_, err = store.GetStream(ctx, stream.StreamID)
	require.ErrorIs(t, err, ErrStreamNotFound)
}

func TestRolloverAndRetirementSealsRemainNonReopenable(t *testing.T) {
	t.Run("rollover", func(t *testing.T) {
		ctx := context.Background()
		store, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		event := Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "incarnation:g2", Type: "message_chunk", Payload: []byte("done")}
		_, err = store.Append(ctx, event)
		require.NoError(t, err)
		require.NoError(t, store.Acknowledge(ctx, event.StreamID, 1))
		require.NoError(t, store.RolloverStream(ctx, event.StreamID, Stream{StreamID: "incarnation:g2:rolled"}))
		_, err = store.Append(ctx, event)
		require.ErrorIs(t, err, ErrOwnerMismatch)
	})

	t.Run("retirement", func(t *testing.T) {
		ctx := context.Background()
		store, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		submission := Submission{ID: "interrupted", StreamID: "incarnation:g1", SessionID: "session",
			IncarnationID: "incarnation", HarnessGeneration: 1, Hash: "hash", Payload: []byte("prompt"),
			State: SubmissionInterruptedUnknown}
		_, err = store.PutSubmission(ctx, submission)
		require.NoError(t, err)
		event := Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: submission.StreamID, SubmissionID: submission.ID, Type: "context_window", Payload: []byte(`{}`)}
		_, err = store.Append(ctx, event)
		require.NoError(t, err)
		require.NoError(t, store.Acknowledge(ctx, event.StreamID, 1))
		_, err = store.RetireSubmission(ctx, submission.ID, 2)
		require.NoError(t, err)
		_, err = store.Append(ctx, event)
		require.ErrorIs(t, err, ErrOwnerMismatch)
	})
}

func TestAbandonSuccessorStreamKeepsChangedOrActiveStreams(t *testing.T) {
	ctx := context.Background()
	store, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	_, err = store.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "incarnation:g2", Type: "usage_update", Payload: []byte(`{}`),
	})
	require.NoError(t, err)
	expected, err := store.GetStream(ctx, "incarnation:g2")
	require.NoError(t, err)

	changed := expected
	changed.HighWater++
	require.ErrorIs(t, store.AbandonStream(ctx, changed), ErrSequenceConflict)
	_, err = store.PutSubmission(ctx, Submission{
		ID: "live", StreamID: expected.StreamID, SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 2, Hash: "live-hash", Payload: []byte("live"), State: SubmissionAccepted,
	})
	require.NoError(t, err)
	require.ErrorIs(t, store.AbandonStream(ctx, expected), ErrSubmissionState)
	stored, err := store.GetStream(ctx, expected.StreamID)
	require.NoError(t, err)
	require.False(t, stored.Sealed)
	require.Equal(t, expected.HighWater, stored.HighWater)
}
