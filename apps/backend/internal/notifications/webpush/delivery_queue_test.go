package webpush

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/providers"
)

type blockingSender struct {
	started chan struct{}
	release chan struct{}
}

func (s *blockingSender) Send(ctx context.Context, _ *models.PushSubscription, _ []byte, _ models.VAPIDKeys, _ string, _ SendOptions) (int, error) {
	close(s.started)
	select {
	case <-s.release:
		return http.StatusCreated, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func TestProviderQueuesPushAndShutdownDrainsAcceptedJob(t *testing.T) {
	sender := &blockingSender{started: make(chan struct{}), release: make(chan struct{})}
	svc, _ := newTestService(t, sender)
	if _, err := svc.Subscribe(context.Background(), "u1", validInput("https://fcm.googleapis.com/device")); err != nil {
		t.Fatal(err)
	}
	cleanup := svc.StartAsyncDelivery()
	t.Cleanup(func() { _ = cleanup() })
	provider := NewProvider(svc)

	sent := make(chan error, 1)
	go func() {
		sent <- provider.Send(context.Background(), providers.Message{
			EventType: "session.permission_requested",
			UserID:    "u1",
			TaskID:    "task-1",
		})
	}()
	select {
	case err := <-sent:
		if err != nil {
			t.Fatalf("provider.Send() error = %v", err)
		}
	case <-time.After(time.Second):
		close(sender.release)
		t.Fatal("provider.Send() waited for the push network call")
	}
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("background worker did not start the accepted push")
	}

	closed := make(chan struct{})
	go func() {
		_ = cleanup()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("queue shutdown returned before accepted push completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(sender.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("queue shutdown did not drain the accepted push")
	}
}
