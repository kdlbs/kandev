package acp

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestSyncNotifQueue_BlocksUntilQueueDrained verifies the barrier-sync
// primitive used by sendPrompt before emitting EventTypeComplete: every
// notification enqueued before syncNotifQueue is processed (and visible on
// updatesCh) by the time the call returns.
func TestSyncNotifQueue_BlocksUntilQueueDrained(t *testing.T) {
	const burst = 50

	a := newTestAdapter()
	t.Cleanup(func() { _ = a.Close() })

	for i := 0; i < burst; i++ {
		a.enqueueACPUpdate(acp.SessionNotification{
			SessionId: acp.SessionId("s1"),
			Update: acp.SessionUpdate{
				AvailableCommandsUpdate: &acp.SessionAvailableCommandsUpdate{
					AvailableCommands: []acp.AvailableCommand{
						{Name: fmt.Sprintf("cmd-%d", i)},
					},
				},
			},
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.syncNotifQueue(ctx)

	events := drainEvents(a)
	if len(events) != burst {
		t.Fatalf("expected %d events on updatesCh after sync, got %d", burst, len(events))
	}
	for i, ev := range events {
		if ev.Type != streams.EventTypeAvailableCommands {
			t.Fatalf("event %d: unexpected type %q", i, ev.Type)
		}
		want := fmt.Sprintf("cmd-%d", i)
		if len(ev.AvailableCommands) != 1 || ev.AvailableCommands[0].Name != want {
			t.Fatalf("event %d: got %+v, want command %q", i, ev.AvailableCommands, want)
		}
	}
}
