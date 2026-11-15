package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

type asyncEventBus struct {
	mu       sync.Mutex
	handlers map[string]bus.EventHandler
}

func (b *asyncEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	b.mu.Lock()
	handler := b.handlers[subject]
	b.mu.Unlock()
	if handler != nil {
		go func() { _ = handler(ctx, event) }()
	}
	return nil
}

func (b *asyncEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.handlers == nil {
		b.handlers = map[string]bus.EventHandler{}
	}
	b.handlers[subject] = handler
	return noopSubscription{}, nil
}

func (*asyncEventBus) QueueSubscribe(string, string, bus.EventHandler) (bus.Subscription, error) {
	return noopSubscription{}, nil
}
func (*asyncEventBus) Request(context.Context, string, *bus.Event, time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (*asyncEventBus) Close()            {}
func (*asyncEventBus) IsConnected() bool { return true }

type noopSubscription struct{}

func (noopSubscription) Unsubscribe() error { return nil }
func (noopSubscription) IsValid() bool      { return true }

func TestWorktreeInventoryDeliveryQueuesOnMemoryAndAsyncBuses(t *testing.T) {
	cases := map[string]bus.EventBus{
		"memory": func() bus.EventBus {
			log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			return bus.NewMemoryEventBus(log)
		}(),
		"async": &asyncEventBus{},
	}
	for name, eventBus := range cases {
		t.Run(name, func(t *testing.T) {
			processed := make(chan string, 1)
			svc := &Service{}
			svc.worktreeObservations = newWorktreeObservationWorker(func(_ context.Context, payload *events.WorktreeInventoryData) {
				processed <- payload.TaskID
			})
			svc.StartWorktreeAdoptionWorker(context.Background())
			t.Cleanup(svc.StopWorktreeAdoptionWorker)
			subject := events.BuildWorkspaceWorktreesSubject("session")
			if _, err := eventBus.Subscribe(subject, func(ctx context.Context, event *bus.Event) error {
				payload, _ := event.Data.(*events.WorktreeInventoryData)
				svc.HandleWorkspaceWorktreesObserved(ctx, payload)
				return nil
			}); err != nil {
				t.Fatalf("subscribe: %v", err)
			}
			if err := eventBus.Publish(context.Background(), subject, bus.NewEvent(
				events.WorkspaceWorktreesObserved, "test",
				&events.WorktreeInventoryData{TaskID: "task", TaskEnvironmentID: "env"},
			)); err != nil {
				t.Fatalf("publish: %v", err)
			}
			select {
			case taskID := <-processed:
				if taskID != "task" {
					t.Fatalf("processed task = %q", taskID)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("worktree snapshot was not processed")
			}
		})
	}
}
