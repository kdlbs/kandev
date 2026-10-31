package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestEventIdentityRetryKeepsOneLogicalResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	j, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	event := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Identity: "plugin:event-1", Type: "background_work_updated", Payload: []byte("first"),
	}
	first, err := j.AppendBatch(ctx, []Event{event})
	if err != nil {
		t.Fatal(err)
	}
	retry := event
	second, err := j.AppendBatch(ctx, []Event{retry})
	if err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if second[0].Sequence != first[0].Sequence {
		t.Fatalf("retry sequence = %d, want %d", second[0].Sequence, first[0].Sequence)
	}
	events, _, err := j.Replay(ctx, "stream", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("replay events = %d, want one logical event", len(events))
	}
	_ = j.Close()

	reopened, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	afterRestart, err := reopened.AppendBatch(ctx, []Event{event})
	if err != nil {
		t.Fatalf("retry after reopen error = %v", err)
	}
	if afterRestart[0].Sequence != first[0].Sequence {
		t.Fatalf("restart retry sequence = %d, want %d", afterRestart[0].Sequence, first[0].Sequence)
	}
}

func TestEventIdentityConflictFailsClosed(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	ctx := context.Background()
	base := Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Identity: "plugin:event-1", Type: "background_work_updated", Payload: []byte("first"),
	}
	if _, err := j.AppendBatch(ctx, []Event{base}); err != nil {
		t.Fatal(err)
	}
	conflict := base
	conflict.Payload = []byte("different")
	if _, err := j.AppendBatch(ctx, []Event{conflict}); !errors.Is(err, ErrEventIdentityConflict) {
		t.Fatalf("conflict error = %v, want ErrEventIdentityConflict", err)
	}
	differentType := base
	differentType.Type = "other"
	if _, err := j.AppendBatch(ctx, []Event{differentType}); !errors.Is(err, ErrEventIdentityConflict) {
		t.Fatalf("type conflict error = %v, want ErrEventIdentityConflict", err)
	}
}

func TestEventIdentityLengthIsBounded(t *testing.T) {
	j, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt")})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	identity := make([]byte, maxEventIdentityBytes+1)
	for index := range identity {
		identity[index] = 'a'
	}
	_, err = j.AppendBatch(context.Background(), []Event{{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "stream", Identity: string(identity), Type: "background_work_updated",
	}})
	if err == nil {
		t.Fatal("oversized event identity was accepted")
	}
}
