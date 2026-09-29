package process

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestDetachedDurableProducerExceedsQueue(t *testing.T) {
	const messageCount = 2501
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
