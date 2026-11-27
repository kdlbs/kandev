package process

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestDetachedDurableProducerExceedsQueue(t *testing.T) {
	// Exceed the one-slot notification queue without making disk throughput
	// part of the detached-delivery assertion. Every event still commits to disk.
	const messageCount = 33
	ctx := context.Background()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	deliveryJournal, err := journal.Open(journal.Config{Path: journalPath})
	if err != nil {
		t.Fatalf("open journal: %v", err)
	}
	manager := &Manager{
		cfg: &config.InstanceConfig{
			DurableJournalPath:        journalPath,
			SessionID:                 "session-1",
			DeliveryStreamID:          "stream-1",
			DeliveryIncarnationID:     "incarnation-1",
			DeliveryHarnessGeneration: 1,
		},
		logger:          newTestLogger(t),
		updatesCh:       make(chan adapter.AgentEvent, 1),
		deliveryJournal: deliveryJournal,
	}
	t.Cleanup(func() {
		if err := manager.closeDeliveryJournal(); err != nil {
			t.Errorf("close journal: %v", err)
		}
	})

	updates := make(chan adapter.AgentEvent, messageCount+1)
	for i := 0; i < messageCount; i++ {
		updates <- adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, Text: "chunk"}
	}
	updates <- adapter.AgentEvent{Type: adapter.EventTypeComplete}
	close(updates)
	stub := &stubAdapter{updatesCh: updates}
	stopCh := make(chan struct{})
	done := make(chan struct{})
	manager.wg.Add(1)
	go func() {
		manager.forwardUpdates(stub, stopCh)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(90 * time.Second):
		_, stream, replayErr := deliveryJournal.Replay(ctx, "stream-1", 0, messageCount+1)
		t.Fatalf("durable producer stalled without a stream consumer (queued=%d, high-water=%d, replay error=%v)", len(manager.updatesCh), stream.HighWater, replayErr)
	}
	if queued := len(manager.updatesCh); queued != 0 {
		t.Fatalf("durable events queued in live notification channel = %d, want 0", queued)
	}

	events, stream, err := deliveryJournal.Replay(ctx, "stream-1", 0, messageCount+1)
	if err != nil {
		t.Fatalf("replay detached output: %v", err)
	}
	if len(events) != messageCount+1 {
		t.Fatalf("retained event count = %d, want %d", len(events), messageCount+1)
	}
	if stream.HighWater != messageCount+1 {
		t.Fatalf("stream high-water = %d, want %d", stream.HighWater, messageCount+1)
	}
	for i, event := range events {
		if event.Sequence != uint64(i+1) {
			t.Fatalf("event %d sequence = %d, want %d", i, event.Sequence, i+1)
		}
		if i < messageCount && (event.Type != adapter.EventTypeMessageChunk || event.Terminal) {
			t.Fatalf("event %d = type %q terminal=%t, want message chunk", i, event.Type, event.Terminal)
		}
	}
	last := events[len(events)-1]
	var terminal adapter.AgentEvent
	if err := json.Unmarshal(last.Payload, &terminal); err != nil {
		t.Fatalf("decode terminal payload: %v", err)
	}
	if last.Sequence != messageCount+1 || !last.Terminal || terminal.Type != adapter.EventTypeComplete {
		t.Fatalf("terminal = sequence %d, journal terminal %t, payload type %q", last.Sequence, last.Terminal, terminal.Type)
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.4
func TestDurableDeliveryDefaultRetentionExceedsFormerStreamLimit(t *testing.T) {
	const (
		messageCount = 230
		messageBytes = 900_000
	)
	ctx := context.Background()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	manager := NewManager(&config.InstanceConfig{
		WorkDir:                   t.TempDir(),
		SessionID:                 "retention-session",
		DeliveryStreamID:          "retention-stream",
		DeliveryIncarnationID:     "retention-incarnation",
		DeliveryHarnessGeneration: 4,
		DurableJournalPath:        journalPath,
	}, newTestLogger(t))
	if manager.deliveryJournalErr != nil {
		t.Fatalf("open journal: %v", manager.deliveryJournalErr)
	}
	cancelled := make(chan struct{}, 1)
	manager.adapter = &pressureCancelAdapter{stubAdapter: &stubAdapter{}, cancelled: cancelled}
	manager.status.Store(StatusRunning)
	t.Cleanup(func() {
		manager.CloseAdmission()
		if err := manager.closeDeliveryJournal(); err != nil {
			t.Errorf("close journal: %v", err)
		}
	})

	submission, err := manager.AdmitDeliverySubmission(ctx, journal.Submission{
		ID: "retention-submission", SessionID: "retention-session", IncarnationID: "retention-incarnation",
		HarnessGeneration: 4, Hash: "retention-hash", Payload: []byte("prompt"),
	})
	if err != nil {
		t.Fatalf("admit submission: %v", err)
	}
	if _, err := manager.deliveryJournal.TransitionSubmission(ctx, submission.ID, journal.SubmissionDispatching, time.Now().UTC()); err != nil {
		t.Fatalf("mark submission dispatching: %v", err)
	}
	manager.deliveryActiveMu.Lock()
	manager.deliveryActiveID = submission.ID
	manager.deliveryActiveMu.Unlock()

	text := strings.Repeat("x", messageBytes)
	for i := 0; i < messageCount; i++ {
		_, err := manager.persistDeliveryBatch(ctx, []adapter.AgentEvent{{
			Type: adapter.EventTypeMessageChunk, Text: text, DeliverySubmissionID: submission.ID,
		}})
		if err != nil {
			t.Fatalf("persist output %d: %v", i+1, err)
		}
		manager.deliveryActiveMu.RLock()
		cancelledID := manager.deliveryPressureCancelledID
		manager.deliveryActiveMu.RUnlock()
		if cancelledID != "" {
			t.Fatalf("active disconnected output was cancelled at %d records with usable disk: submission %q", i+1, cancelledID)
		}
	}

	manager.deliveryActiveMu.RLock()
	cancelledID := manager.deliveryPressureCancelledID
	manager.deliveryActiveMu.RUnlock()
	if cancelledID != "" {
		t.Fatalf("active disconnected output was cancelled at %d records with usable disk: submission %q", messageCount, cancelledID)
	}
	select {
	case <-cancelled:
		t.Fatal("agent received a pressure cancellation while usable disk remained")
	default:
	}

	if _, err := manager.persistDeliveryBatch(ctx, []adapter.AgentEvent{{
		Type: adapter.EventTypeComplete, Text: "done", DeliverySubmissionID: submission.ID,
	}}); err != nil {
		t.Fatalf("persist terminal output: %v", err)
	}
	if _, err := manager.deliveryJournal.TransitionSubmission(ctx, submission.ID, journal.SubmissionCompleted, time.Now().UTC()); err != nil {
		t.Fatalf("complete submission: %v", err)
	}
	capacity, err := manager.deliveryJournal.Capacity(ctx, "retention-stream")
	if err != nil {
		t.Fatalf("read retained capacity: %v", err)
	}
	const formerStreamLimit = 256 << 20
	if capacity.StreamBytes <= formerStreamLimit || capacity.JournalBytes <= formerStreamLimit {
		t.Fatalf("retained stream/journal bytes = %d/%d, want both above former 256 MiB stream limit", capacity.StreamBytes, capacity.JournalBytes)
	}
	if err := manager.closeDeliveryJournal(); err != nil {
		t.Fatalf("close journal before reopen: %v", err)
	}
	reopened, err := journal.Open(journal.Config{Path: journalPath, ExistingOnly: true})
	if err != nil {
		t.Fatalf("reopen journal: %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("close reopened journal: %v", err)
		}
	})

	var replayed int
	var after uint64
	for {
		events, stream, err := reopened.Replay(ctx, "retention-stream", after, 4)
		if err != nil {
			t.Fatalf("replay after %d: %v", after, err)
		}
		if len(events) == 0 {
			if after != stream.HighWater {
				t.Fatalf("replay stopped at %d before high-water %d", after, stream.HighWater)
			}
			break
		}
		for _, event := range events {
			if event.Sequence != after+1 {
				t.Fatalf("replay sequence = %d after %d", event.Sequence, after)
			}
			var update adapter.AgentEvent
			if err := json.Unmarshal(event.Payload, &update); err != nil {
				t.Fatalf("decode event %d: %v", event.Sequence, err)
			}
			if update.DeliverySubmissionID != submission.ID {
				t.Fatalf("event %d submission = %q", event.Sequence, update.DeliverySubmissionID)
			}
			if event.Sequence <= messageCount {
				if update.Type != adapter.EventTypeMessageChunk || len(update.Text) != messageBytes || update.Text != text {
					t.Fatalf("message %d changed during retention", event.Sequence)
				}
			} else if update.Type != adapter.EventTypeComplete || !event.Terminal {
				t.Fatalf("terminal event = sequence %d type %q terminal=%t", event.Sequence, update.Type, event.Terminal)
			}
			after = event.Sequence
			replayed++
		}
	}
	if replayed != messageCount+1 {
		t.Fatalf("replayed records = %d, want %d", replayed, messageCount+1)
	}
	t.Logf("retained %d stream bytes across %d records; replayed all records after reopen without cancellation", capacity.StreamBytes, replayed)
}
