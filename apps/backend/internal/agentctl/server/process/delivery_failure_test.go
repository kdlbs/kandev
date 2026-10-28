package process

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func seedDispatchingSubmission(t *testing.T, store *journal.Journal, id string) {
	t.Helper()
	ctx := context.Background()
	_, err := store.PutSubmission(ctx, journal.Submission{
		ID: id, Hash: "hash-" + id, Payload: []byte("prompt"),
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "stream",
	})
	require.NoError(t, err)
	_, err = store.TransitionSubmission(ctx, id, journal.SubmissionAccepted, time.Time{})
	require.NoError(t, err)
	_, err = store.TransitionSubmission(ctx, id, journal.SubmissionDispatching, time.Time{})
	require.NoError(t, err)
}

func waitForDeliveryFailure(t *testing.T, manager *Manager) *types.DeliveryFailureSnapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if failure := manager.DeliveryHealth().Failure; failure != nil {
			return failure
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("delivery failure snapshot was not captured")
	return nil
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
func TestDeliveryFailureDrainsActualTerminal(t *testing.T) {
	manager := newAllToolManager(t, 1024)
	seedDispatchingSubmission(t, manager.deliveryJournal, "submission-1")
	seedDispatchingSubmission(t, manager.deliveryJournal, "submission-2")
	manager.TrackDeliverySubmission("submission-1", 1)
	manager.TrackDeliverySubmission("submission-2", 2)
	manager.deliveryFailureDrain = 5 * time.Second

	updates := make(chan adapter.AgentEvent)
	done := make(chan struct{})
	manager.wg.Add(1)
	go func() {
		manager.forwardUpdates(&stubAdapter{updatesCh: updates}, make(chan struct{}))
		close(done)
	}()

	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, Text: strings.Repeat("x", 4096)}
	failure := waitForDeliveryFailure(t, manager)
	require.Equal(t, types.DeliveryFailureEventTooLarge, failure.Reason)
	require.Equal(t, "submission-1", failure.SubmissionID)
	require.Equal(t, "stream", failure.StreamID)
	require.Equal(t, "session", failure.SessionID)

	// Ordinary output after the failed append is dropped, not published.
	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, Text: "dropped"}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 1}
	require.Eventually(t, func() bool {
		stream, streamErr := manager.deliveryJournal.GetStream(context.Background(), "stream")
		return streamErr == nil && stream.HighWater == 1
	}, 10*time.Second, 10*time.Millisecond, "the cancellation terminal was not committed")
	require.Eventually(t, func() bool {
		return manager.DeliveryHealth().Failure == nil
	}, 10*time.Second, 10*time.Millisecond, "a committed terminal re-establishes healthy delivery")

	// The same live consumer must forward a distinct next instruction exactly
	// once after the drain. Exiting here would leave the retained runtime with
	// no journal writer and a falsely healthy admission state.
	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 2, Text: "next instruction"}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 2}
	close(updates)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("delivery forwarding did not finish after the next instruction")
	}

	events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 3, "the failed ordinary write and dropped output must not be published")
	require.Equal(t, adapter.EventTypeComplete, events[0].Type)
	require.True(t, events[0].Terminal)
	require.Equal(t, "submission-1", events[0].SubmissionID)
	require.Equal(t, "next instruction", decodeEventText(t, events[1].Payload))
	require.Equal(t, "submission-2", events[1].SubmissionID)
	require.Equal(t, adapter.EventTypeComplete, events[2].Type)
	require.True(t, events[2].Terminal)
	require.Equal(t, "submission-2", events[2].SubmissionID)

	retained, err := manager.deliveryJournal.GetSubmission(context.Background(), "submission-1")
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionCompleted, retained.State)
	require.True(t, retained.TerminalEventRetained)
	next, err := manager.deliveryJournal.GetSubmission(context.Background(), "submission-2")
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionCompleted, next.State)
	require.True(t, next.TerminalEventRetained)
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestDeliveryFailureTerminalWritePreservesUnknown(t *testing.T) {
	manager := newAllToolManager(t, 1<<20)
	seedDispatchingSubmission(t, manager.deliveryJournal, "submission-1")
	accepted, err := manager.persistDeliveryEvent(adapter.AgentEvent{
		Type: adapter.EventTypeMessageChunk, Text: "accepted before failure",
	})
	require.NoError(t, err)
	require.NotZero(t, accepted.DeliverySequence)
	require.NoError(t, manager.closeDeliveryJournal())

	updates := make(chan adapter.AgentEvent, 1)
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 1, DeliverySubmissionID: "submission-1"}
	close(updates)
	manager.wg.Add(1)
	manager.forwardUpdates(&stubAdapter{updatesCh: updates}, make(chan struct{}))

	health := manager.DeliveryHealth()
	require.NotNil(t, health.Failure)
	require.Equal(t, types.DeliveryFailureStorageUnavailable, health.Failure.Reason)
	require.Equal(t, "submission-1", health.Failure.SubmissionID)

	// Previously accepted records remain durable; the failed terminal does not
	// fabricate a completed outcome.
	reopened, err := journal.Open(journal.Config{Path: manager.cfg.DurableJournalPath})
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	events, _, err := reopened.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "accepted before failure", decodeEventText(t, events[0].Payload))
	submission, err := reopened.GetSubmission(context.Background(), "submission-1")
	require.NoError(t, err)
	require.Equal(t, journal.SubmissionDispatching, submission.State)
	require.False(t, submission.TerminalEventRetained)
}

func decodeEventText(t *testing.T, payload []byte) string {
	t.Helper()
	var event streams.AgentEvent
	require.NoError(t, json.Unmarshal(payload, &event))
	return event.Text
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestDeliveryHealthStatusBypassesFailedWriter(t *testing.T) {
	manager := newAllToolManager(t, 1024)
	require.NoError(t, manager.closeDeliveryJournal())
	_, err := manager.persistDeliveryEvent(adapter.AgentEvent{
		Type: adapter.EventTypeMessageChunk, Text: strings.Repeat("x", 4096),
	})
	require.Error(t, err)

	started := time.Now()
	health := manager.DeliveryHealth()
	require.Less(t, time.Since(started), 500*time.Millisecond, "status must not wait on the failed writer")
	require.NotNil(t, health.Failure)
	require.Equal(t, types.DeliveryFailureStorageUnavailable, health.Failure.Reason)
	require.Equal(t, int64(1024), health.Failure.EventLimit)
}

func TestDeliveryFailureClassification(t *testing.T) {
	for name, test := range map[string]struct {
		err  error
		want string
	}{
		"event_too_large":     {journal.ErrEventTooLarge, types.DeliveryFailureEventTooLarge},
		"storage_full":        {journal.ErrJournalFull, types.DeliveryFailureStorageFull},
		"storage_corrupt":     {journal.ErrJournalCorrupt, types.DeliveryFailureStorageCorrupt},
		"storage_unavailable": {journal.ErrJournalClosed, types.DeliveryFailureStorageUnavailable},
		"writer_failed":       {context.DeadlineExceeded, types.DeliveryFailureWriterFailed},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, classifyDeliveryFailure(test.err))
		})
	}
}

func TestDeliveryFailureDrainDropsOrdinaryOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drain.bbolt")
	store, err := journal.Open(journal.Config{Path: path, MaxEventBytes: 1024})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	manager := &Manager{logger: newTestLogger(t), deliveryJournal: store, cfg: nil}
	updates := make(chan adapter.AgentEvent, 3)
	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "ordinary"}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeToolUpdate, Text: "ordinary"}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
	close(updates)
	manager.deliveryFailureDrain = time.Second
	_, _, _ = manager.drainAfterDeliveryFailure(updates, make(chan struct{}), "submission-1")
	// The manager is not configured for durable delivery, so the terminal is a
	// no-op; the drain must still consume every update without publishing them.
	require.Equal(t, 0, len(updates))
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.2
func TestDeliveryFailureTimeoutSuppressesFailedOutput(t *testing.T) {
	manager := newAllToolManager(t, 1024)
	seedDispatchingSubmission(t, manager.deliveryJournal, "submission-1")
	seedDispatchingSubmission(t, manager.deliveryJournal, "submission-2")
	manager.TrackDeliverySubmission("submission-1", 1)
	manager.TrackDeliverySubmission("submission-2", 2)
	manager.deliveryFailureDrain = 50 * time.Millisecond

	updates := make(chan adapter.AgentEvent)
	done := make(chan struct{})
	manager.wg.Add(1)
	go func() {
		manager.forwardUpdates(&stubAdapter{updatesCh: updates}, make(chan struct{}))
		close(done)
	}()

	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, Text: strings.Repeat("x", 4096)}
	waitForDeliveryFailure(t, manager)
	time.Sleep(100 * time.Millisecond)

	// The drain timeout has expired. Ordinary output of the failed submission
	// must not be published and must not clear the sticky failure.
	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1, Text: "must be dropped"}
	time.Sleep(100 * time.Millisecond)
	events, _, err := manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	if !errors.Is(err, journal.ErrStreamNotFound) {
		require.NoError(t, err)
	}
	require.Empty(t, events, "ordinary output of the failed submission was published after the drain timeout")
	require.NotNil(t, manager.DeliveryHealth().Failure, "ordinary output cleared the failed writer")

	// A different submission proves the consumer is still usable and releases
	// the sticky failure only through its own committed event.
	updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 2, Text: "next instruction"}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 2}
	close(updates)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("forwarding did not finish after the successor submission")
	}
	events, _, err = manager.deliveryJournal.Replay(context.Background(), "stream", 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "next instruction", decodeEventText(t, events[0].Payload))
	require.Equal(t, "submission-2", events[0].SubmissionID)
	require.Equal(t, adapter.EventTypeComplete, events[1].Type)
	require.Equal(t, "submission-2", events[1].SubmissionID)
	require.Nil(t, manager.DeliveryHealth().Failure)
}
