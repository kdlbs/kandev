package webpush

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/notifications/providers"
)

type recordingNotifier struct {
	userID  string
	payload Payload
	opts    SendOptions
	err     error
}

type queuedRecordingNotifier struct {
	queued   bool
	notified bool
}

func (q *queuedRecordingNotifier) Enqueue(_ context.Context, _ string, _ Payload, _ SendOptions) error {
	q.queued = true
	return nil
}

func (q *queuedRecordingNotifier) Notify(context.Context, string, Payload, SendOptions) (int, error) {
	q.notified = true
	return 1, nil
}

func (r *recordingNotifier) Notify(_ context.Context, userID string, p Payload, o SendOptions) (int, error) {
	r.userID, r.payload, r.opts = userID, p, o
	return 1, r.err
}

func TestProviderBuildsDeepLinkTagAndUrgency(t *testing.T) {
	n := &recordingNotifier{}
	err := NewProvider(n).Send(context.Background(), providers.Message{
		EventType: "session.permission_requested", UserID: "u1", Title: "T", Body: "B",
		TaskID: "task-1", TaskSessionID: "sess-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if n.userID != "u1" || n.payload.URL != "/t/task-1?sessionId=sess-1" {
		t.Errorf("recipient/url = %q %q", n.userID, n.payload.URL)
	}
	if n.payload.Tag != "kandev-session.permission_requested-sess-1" {
		t.Errorf("tag = %q", n.payload.Tag)
	}
	if n.opts.Urgency != UrgencyHigh {
		t.Errorf("urgency = %q, want high for an event that blocks the agent", n.opts.Urgency)
	}
	if len(n.opts.Topic) == 0 || len(n.opts.Topic) > 32 {
		t.Errorf("topic %q must be 1..32 chars", n.opts.Topic)
	}
}

func TestProviderUsesNormalUrgencyForTurnFinishedAndHomeLinkWithoutTask(t *testing.T) {
	n := &recordingNotifier{}
	_ = NewProvider(n).Send(context.Background(), providers.Message{EventType: "session.turn_finished", UserID: "u1"})
	if n.opts.Urgency != UrgencyNormal || n.payload.URL != "/" {
		t.Errorf("urgency/url = %q %q", n.opts.Urgency, n.payload.URL)
	}
}

func TestProviderPropagatesDeliveryFailure(t *testing.T) {
	want := errors.New("boom")
	err := NewProvider(&recordingNotifier{err: want}).Send(context.Background(), providers.Message{UserID: "u1"})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestProviderQueuesDeliveryWhenNotifierSupportsIt(t *testing.T) {
	notifier := &queuedRecordingNotifier{}
	if err := NewProvider(notifier).Send(context.Background(), providers.Message{
		EventType: "session.permission_requested", UserID: "u1", TaskID: "task-1", TaskSessionID: "session-1",
	}); err != nil {
		t.Fatal(err)
	}
	if !notifier.queued || notifier.notified {
		t.Fatalf("queued = %t, notified = %t; want queue-only delivery", notifier.queued, notifier.notified)
	}
}
