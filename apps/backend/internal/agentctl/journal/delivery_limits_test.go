package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
)

func openDeliveryLimitJournal(t *testing.T, config Config) *Journal {
	t.Helper()
	if config.Path == "" {
		config.Path = filepath.Join(t.TempDir(), "delivery.bbolt")
	}
	store, err := Open(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func limitEvent(payloadBytes int64) Event {
	return Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "tool_update", Payload: make([]byte, payloadBytes),
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.1
// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.5
func TestDeliveryEventLimitBoundaries(t *testing.T) {
	store := openDeliveryLimitJournal(t, Config{})
	if store.MaxEventBytes() != deliverylimits.MaxEventBytes {
		t.Fatalf("default event limit = %d, want %d", store.MaxEventBytes(), deliverylimits.MaxEventBytes)
	}
	ctx := context.Background()
	if _, err := store.Append(ctx, limitEvent(deliverylimits.MaxEventBytes)); err != nil {
		t.Fatalf("maximum event payload rejected: %v", err)
	}
	_, err := store.Append(ctx, limitEvent(deliverylimits.MaxEventBytes+1))
	if !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("oversized event error = %v, want ErrEventTooLarge", err)
	}
	if !errors.Is(err, ErrStreamFull) {
		t.Fatalf("oversized event lost the historical ErrStreamFull compatibility: %v", err)
	}
	_, stream, err := store.Replay(ctx, "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if stream.HighWater != 1 {
		t.Fatalf("rejected oversized event moved the high-water mark: %d", stream.HighWater)
	}

	small := openDeliveryLimitJournal(t, Config{MaxEventBytes: 64 << 10})
	if _, err := small.Append(ctx, limitEvent(64<<10)); err != nil {
		t.Fatalf("custom event limit rejected an exact fit: %v", err)
	}
	if _, err := small.Append(ctx, limitEvent(64<<10+1)); !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("custom event limit error = %v, want ErrEventTooLarge", err)
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.4
func TestDeliveryEventLimitDoesNotRaiseSubmissionLimit(t *testing.T) {
	store := openDeliveryLimitJournal(t, Config{})
	if store.MaxSubmissionBytes() != deliverylimits.MaxSubmissionBytes {
		t.Fatalf("default submission limit = %d, want %d", store.MaxSubmissionBytes(), deliverylimits.MaxSubmissionBytes)
	}
	if store.MaxSubmissionBytes() >= store.MaxEventBytes() {
		t.Fatalf("submission limit %d was raised with the event limit %d", store.MaxSubmissionBytes(), store.MaxEventBytes())
	}
	ctx := context.Background()
	putSubmission := func(id string, size int64) error {
		_, err := store.PutSubmission(ctx, Submission{ID: id, Hash: "hash-" + id, Payload: make([]byte, size)})
		return err
	}
	if err := putSubmission("prompt-max", deliverylimits.MaxSubmissionBytes); err != nil {
		t.Fatalf("maximum prompt payload rejected: %v", err)
	}
	if err := putSubmission("prompt-over", deliverylimits.MaxSubmissionBytes+1); !errors.Is(err, ErrStreamFull) {
		t.Fatalf("oversized prompt error = %v, want ErrStreamFull", err)
	}

	small := openDeliveryLimitJournal(t, Config{MaxEventBytes: 32 << 10})
	if small.MaxSubmissionBytes() != 32<<10 {
		t.Fatalf("submission limit for a smaller event cap = %d, want %d", small.MaxSubmissionBytes(), 32<<10)
	}
	if err := putSubmissionOn(t, small, "small-max", 32<<10); err != nil {
		t.Fatalf("small prompt payload rejected: %v", err)
	}
	if err := putSubmissionOn(t, small, "small-over", 32<<10+1); !errors.Is(err, ErrStreamFull) {
		t.Fatalf("small oversized prompt error = %v, want ErrStreamFull", err)
	}

	explicit := openDeliveryLimitJournal(t, Config{MaxEventBytes: deliverylimits.MaxEventBytes, MaxSubmissionBytes: 4 << 10})
	if err := putSubmissionOn(t, explicit, "explicit-max", 4<<10); err != nil {
		t.Fatalf("explicit submission override rejected an exact fit: %v", err)
	}
	if err := putSubmissionOn(t, explicit, "explicit-over", 4<<10+1); !errors.Is(err, ErrStreamFull) {
		t.Fatalf("explicit submission override error = %v, want ErrStreamFull", err)
	}
}

func putSubmissionOn(t *testing.T, store *Journal, id string, size int64) error {
	t.Helper()
	_, err := store.PutSubmission(context.Background(), Submission{ID: id, Hash: "hash-" + id, Payload: make([]byte, size)})
	return err
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
func TestDeliveryLimitLegacyCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	ctx := context.Background()
	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	legacyPayload := []byte(`{"type":"tool_update","tool_call_id":"legacy","known_field":"retained"}`)
	legacy, err := store.Append(ctx, Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Type: "tool_update", Payload: legacyPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// A reopened journal with a smaller append limit must still read history.
	reopened, err := Open(Config{Path: path, MaxEventBytes: 64 << 10})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	first, stream, err := reopened.Replay(ctx, "stream", 0, 10)
	if err != nil {
		t.Fatalf("historical replay under a smaller append limit: %v", err)
	}
	if len(first) != 1 || string(first[0].Payload) != string(legacy.Payload) || first[0].Sequence != legacy.Sequence {
		t.Fatalf("historical replay changed accepted bytes: %#v", first)
	}
	second, _, err := reopened.Replay(ctx, "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if string(second[0].Payload) != string(first[0].Payload) || stream.HighWater != 1 {
		t.Fatal("duplicate replay changed payload bytes or high-water")
	}
	if _, err := reopened.Append(ctx, limitEvent(64<<10+1)); !errors.Is(err, ErrEventTooLarge) {
		t.Fatalf("new append limit error = %v, want ErrEventTooLarge", err)
	}
}
