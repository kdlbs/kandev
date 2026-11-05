package journal

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func appendReplayEvents(t *testing.T, store *Journal, count int, payloadBytes int, terminalLast bool) {
	t.Helper()
	for index := range count {
		eventType := "tool_update"
		terminal := false
		if terminalLast && index == count-1 {
			eventType = "complete"
			terminal = true
		}
		_, err := store.Append(context.Background(), Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
			StreamID: "stream", Type: eventType, Terminal: terminal,
			Payload: make([]byte, payloadBytes),
		})
		if err != nil {
			t.Fatalf("append event %d: %v", index, err)
		}
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.3
// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.2
func TestDeliveryReplayByteBudget(t *testing.T) {
	store := openDeliveryLimitJournal(t, Config{
		Path:           filepath.Join(t.TempDir(), "delivery.bbolt"),
		MaxEventBytes:  8 << 10,
		MaxReplayBytes: 8 << 10,
	})
	appendReplayEvents(t, store, 5, 1<<10, false)
	ctx := context.Background()
	after := uint64(0)
	pages := 0
	for after < 5 {
		events, stream, err := store.Replay(ctx, "stream", after, 1000)
		if err != nil {
			t.Fatalf("replay after %d: %v", after, err)
		}
		if len(events) == 0 {
			t.Fatalf("empty byte-limited page after %d before high-water %d", after, stream.HighWater)
		}
		if len(events) >= 5 {
			t.Fatalf("replay ignored the byte budget: page=%d", len(events))
		}
		for _, event := range events {
			if event.Sequence != after+1 {
				t.Fatalf("replay sequence %d follows cursor %d", event.Sequence, after)
			}
			after = event.Sequence
		}
		pages++
		if pages > 10 {
			t.Fatal("replay did not converge")
		}
	}
	if pages < 2 {
		t.Fatalf("near-limit records did not produce short pages: %d", pages)
	}
}

// @covers AC-PLATFORM-BOUNDED-TOOL-DATA-004.3
func TestDeliveryReplayShortPageReachesTerminal(t *testing.T) {
	store := openDeliveryLimitJournal(t, Config{
		Path:           filepath.Join(t.TempDir(), "delivery.bbolt"),
		MaxEventBytes:  8 << 10,
		MaxReplayBytes: 8 << 10,
	})
	appendReplayEvents(t, store, 7, 1<<10, true)
	ctx := context.Background()
	after := uint64(0)
	reachedTerminal := false
	for !reachedTerminal {
		events, stream, err := store.Replay(ctx, "stream", after, 1000)
		if err != nil {
			t.Fatalf("replay after %d: %v", after, err)
		}
		if len(events) == 0 && after < stream.HighWater {
			t.Fatalf("empty page before high-water %d", stream.HighWater)
		}
		for _, event := range events {
			if event.Sequence != after+1 {
				t.Fatalf("replay sequence %d follows cursor %d", event.Sequence, after)
			}
			after = event.Sequence
			reachedTerminal = reachedTerminal || event.Terminal
		}
		if after > 20 {
			t.Fatal("replay did not reach the terminal")
		}
	}
	if !reachedTerminal || after != 7 {
		t.Fatalf("terminal reached=%t cursor=%d", reachedTerminal, after)
	}

	tooSmall := openDeliveryLimitJournal(t, Config{
		Path:           filepath.Join(t.TempDir(), "budget.bbolt"),
		MaxEventBytes:  8 << 10,
		MaxReplayBytes: 1024,
	})
	appendReplayEvents(t, tooSmall, 1, 4<<10, false)
	if _, _, err := tooSmall.Replay(ctx, "stream", 0, 10); !errors.Is(err, ErrReplayPageTooLarge) {
		t.Fatalf("individually unrepresentable record error = %v, want ErrReplayPageTooLarge", err)
	}
	stream, err := tooSmall.GetStream(ctx, "stream")
	if err != nil {
		t.Fatalf("high-water probe: %v", err)
	}
	if stream.HighWater != 1 {
		t.Fatalf("replay failure advanced the high-water mark: %d", stream.HighWater)
	}
}
