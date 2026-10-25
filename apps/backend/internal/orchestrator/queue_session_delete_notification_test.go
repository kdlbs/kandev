package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type blockingSessionAttachmentCleaner struct {
	started chan struct{}
	release chan struct{}
}

func (c *blockingSessionAttachmentCleaner) DeleteSessionMessageAttachments(context.Context, string, string) error {
	close(c.started)
	<-c.release
	return nil
}

func TestDeletedSessionCleanupSerializesAttachmentCleanupWithQueueMutation(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger())
	entry, err := queue.QueueMessage(ctx, "session-delete-race", "task-delete-race", "queued", "", messagequeue.QueuedByUser, false, nil)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}
	cleaner := &blockingSessionAttachmentCleaner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := &Service{
		messageQueue:             queue,
		sessionAttachmentCleaner: cleaner,
		logger:                   testLogger(),
	}

	cleanupDone := make(chan struct{})
	go func() {
		svc.cancelDeletedSessionQueue(ctx, entry.TaskID, entry.SessionID)
		close(cleanupDone)
	}()
	select {
	case <-cleaner.started:
	case <-time.After(time.Second):
		t.Fatal("attachment cleanup did not start")
	}

	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- queue.UpdateMessageWithMetadata(
			ctx, entry.SessionID, entry.ID, "changed", nil, nil, messagequeue.QueuedByUser,
		)
	}()
	select {
	case err := <-mutationDone:
		t.Fatalf("queue mutation completed during attachment cleanup: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(cleaner.release)

	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("session cleanup did not complete")
	}
	if err := <-mutationDone; err == nil {
		t.Fatal("queue mutation unexpectedly succeeded after cleanup")
	}
}

func TestDeleteSessionPublishesOneQueueStatusNotification(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-session-delete-once", "session-delete-once", models.TaskSessionStateCompleted)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc.eventBus = eventBus
	repo.SetTaskSessionQueuePurgeNotifier(func(ctx context.Context, taskID, sessionID string) {
		svc.purgeDeletedSessionQueue(ctx, taskID, sessionID)
	})
	svc.sessionQueuePurgeNotifierRegistered = true

	var statusEvents atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(context.Context, *bus.Event) error {
		statusEvents.Add(1)
		return nil
	}); err != nil {
		t.Fatalf("subscribe queue status: %v", err)
	}

	if err := svc.DeleteSession(ctx, "session-delete-once"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if got := statusEvents.Load(); got != 1 {
		t.Fatalf("queue status notifications = %d, want 1", got)
	}
}

func TestDeletedSessionQueueNotificationIgnoresCancelledContext(t *testing.T) {
	queue := messagequeue.NewServiceMemory(testLogger())
	entry, err := queue.QueueMessage(
		context.Background(),
		"session-delete-cancelled",
		"task-delete-cancelled",
		"queued",
		"",
		messagequeue.QueuedByUser,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}

	svc := &Service{messageQueue: queue, logger: testLogger()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	svc.purgeDeletedSessionQueue(ctx, entry.TaskID, entry.SessionID)

	if got := queue.GetStatus(context.Background(), entry.SessionID).Count; got != 1 {
		t.Fatalf("post-commit notification changed queue count = %d, want 1", got)
	}
}

func TestFallbackDeletedSessionCleanupPublishesQueueStatus(t *testing.T) {
	queue := messagequeue.NewServiceMemory(testLogger())
	entry, err := queue.QueueMessage(
		context.Background(),
		"session-delete-fallback",
		"task-delete-fallback",
		"queued",
		"",
		messagequeue.QueuedByUser,
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("queue message: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(func() { eventBus.Close() })
	svc := &Service{
		messageQueue: queue,
		eventBus:     eventBus,
		logger:       testLogger(),
	}
	var statusEvents atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data["session_id"] == entry.SessionID {
			statusEvents.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("subscribe queue status: %v", err)
	}

	svc.cancelDeletedSessionQueue(context.Background(), entry.TaskID, entry.SessionID)

	if got := statusEvents.Load(); got != 1 {
		t.Fatalf("fallback queue status notifications = %d, want 1", got)
	}
}
