package journal

import (
	"context"
	"errors"
	"testing"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-001.4
func TestCapacityPressureRecoversOnlyAfterAcknowledgment(t *testing.T) {
	j, err := Open(Config{Path: t.TempDir() + "/journal", MaxStreamBytes: 4096, MaxJournalBytes: 16384, ReserveBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	ctx := context.Background()
	var last Event
	for i := 0; i < 30; i++ {
		event, err := j.Append(ctx, Event{SessionID: "session", StreamID: "stream", Type: "message", Payload: make([]byte, 200)})
		if err != nil {
			break
		}
		last = event
	}
	before, err := j.Capacity(ctx, "stream")
	if err != nil {
		t.Fatal(err)
	}
	if !before.Guarded || before.StreamBytes == 0 {
		t.Fatalf("expected guard, got %+v", before)
	}
	events, _, err := j.Replay(ctx, "stream", 0, 100)
	if err != nil || len(events) == 0 {
		t.Fatalf("replay at pressure: %v", err)
	}
	if err := j.Acknowledge(ctx, "stream", last.Sequence); err != nil {
		t.Fatal(err)
	}
	after, err := j.Capacity(ctx, "stream")
	if err != nil {
		t.Fatal(err)
	}
	if !after.Recovered || after.StreamBytes != 0 {
		t.Fatalf("capacity after ACK = %+v", after)
	}
	if _, err := j.Append(ctx, Event{SessionID: "session", StreamID: "stream", Type: "message", Payload: []byte("next")}); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalCanUseReserveWithoutGrowingQuota(t *testing.T) {
	for _, scope := range []string{"stream", "journal"} {
		t.Run(scope, func(t *testing.T) {
			cfg := Config{Path: t.TempDir() + "/journal", MaxStreamBytes: 4096, MaxJournalBytes: 16384, ReserveBytes: 512}
			if scope == "journal" {
				cfg.MaxStreamBytes = 16384
				cfg.MaxJournalBytes = 4096
			}
			j, err := Open(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = j.Close() }()
			ctx := context.Background()
			for {
				_, err = j.Append(ctx, Event{SessionID: "session", StreamID: "stream", Type: "message", Payload: make([]byte, 200)})
				if errors.Is(err, ErrStreamFull) || errors.Is(err, ErrJournalFull) {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err = j.Append(ctx, Event{SessionID: "session", StreamID: "stream", Type: "complete", Terminal: true, Payload: []byte("{}")}); err != nil {
				t.Fatalf("terminal reserve unavailable: %v", err)
			}
			capacity, err := j.Capacity(ctx, "stream")
			if err != nil {
				t.Fatal(err)
			}
			if capacity.StreamBytes > cfg.MaxStreamBytes || capacity.JournalBytes > cfg.MaxJournalBytes {
				t.Fatalf("quota grew: %+v", capacity)
			}
		})
	}
}
