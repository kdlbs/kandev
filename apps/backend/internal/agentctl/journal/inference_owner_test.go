package journal

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	systemmetrics "github.com/kandev/kandev/internal/system/metrics"
)

func TestJournalInferenceOwnerSurvivesReopenWithoutInventingFreshness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delivery.bbolt")
	retained, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if !retained.Fresh() {
		t.Fatal("new journal is not fresh")
	}
	original := []byte(`{"owner":"retained-proof"}`)
	ctx := context.Background()
	if err := retained.WriteInferenceOwner(ctx, original, true); err != nil {
		t.Fatal(err)
	}
	event, err := retained.Append(ctx, Event{SessionID: "session", IncarnationID: "session", HarnessGeneration: 1, StreamID: "session", Type: "message", Payload: []byte("history-preserved")})
	if err != nil {
		t.Fatal(err)
	}
	if err := retained.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	})
	if reopened.Fresh() {
		t.Fatal("reopened journal is a fresh empty inventory")
	}
	value, found, err := reopened.ReadInferenceOwner()
	if err != nil || !found || string(value) != string(original) {
		t.Fatalf("proof lost: %s %t %v", value, found, err)
	}
	events, _, err := reopened.Replay(ctx, "session", 0, 10)
	if err != nil || len(events) != 1 || events[0].Sequence != event.Sequence || string(events[0].Payload) != "history-preserved" {
		t.Fatalf("ownership changed original history: %+v %v", events, err)
	}
}

func TestJournalInferenceOwnerUsesExistingDiskReserve(t *testing.T) {
	reader := func(context.Context, string) (systemmetrics.DiskCapacity, error) {
		return systemmetrics.DiskCapacity{TotalBytes: 1 << 40, AvailableBytes: uint64(DefaultDiskReserveBytes + DefaultMaxEventBytes)}, nil
	}
	retained, err := Open(Config{Path: filepath.Join(t.TempDir(), "delivery.bbolt"), DiskCapacityReader: reader, DiskCapacityCacheTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := retained.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	original := []byte(`{"terminal":"exact-proof"}`)
	if err := retained.WriteInferenceOwner(ctx, original, true); err != nil {
		t.Fatalf("terminal reserve unavailable: %v", err)
	}
	if err := retained.WriteInferenceOwner(ctx, []byte(`{"new":"work"}`), false); err == nil {
		t.Fatal("new preparation bypassed disk reserve")
	}
	value, found, err := retained.ReadInferenceOwner()
	if err != nil || !found || string(value) != string(original) {
		t.Fatalf("refusal damaged terminal proof: %s %t %v", value, found, err)
	}
}
