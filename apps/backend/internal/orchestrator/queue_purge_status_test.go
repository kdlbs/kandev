package orchestrator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestArchiveTaskPublishesQueueStatusWithTaskID(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-archive-queue", "session-archive-queue", models.TaskSessionStateIdle)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	eventBus := bus.NewMemoryEventBus(testLogger())
	svc.eventBus = eventBus

	var saw atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data == nil {
			return nil
		}
		if taskID, _ := data["task_id"].(string); taskID == "task-archive-queue" {
			saw.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, err := svc.messageQueue.QueueMessage(ctx, "session-archive-queue", "task-archive-queue", "queued", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-archive-queue"); err != nil || got != 1 {
		t.Fatalf("pending before archive = %d err=%v, want 1", got, err)
	}

	// Archive through the task repository (same purge path production uses).
	// The service helper used by HTTP is heavier; repository purge is enough
	// to exercise notifyTaskQueuePurged + the wired notifier.
	if err := repo.ArchiveTask(ctx, "task-archive-queue"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for saw.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if saw.Load() == 0 {
		t.Fatal("expected message.queue.status_changed with task_id after archive purge")
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-archive-queue"); err != nil || got != 0 {
		t.Fatalf("pending after archive = %d err=%v, want 0", got, err)
	}
}

func TestDeleteSessionCancelsQueuedPromptsAndPublishesStatus(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-session-queue", "session-keep", models.TaskSessionStateIdle)
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-drop", TaskID: "task-session-queue",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(session-drop): %v", err)
	}

	manager := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
	eventBus := bus.NewMemoryEventBus(testLogger())
	svc.eventBus = eventBus

	var saw atomic.Int32
	if _, err := eventBus.Subscribe(events.MessageQueueStatusChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data == nil {
			return nil
		}
		if taskID, _ := data["task_id"].(string); taskID == "task-session-queue" {
			saw.Add(1)
		}
		return nil
	}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if _, err := svc.messageQueue.QueueMessage(ctx, "session-drop", "task-session-queue", "orphan me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage drop: %v", err)
	}
	if _, err := svc.messageQueue.QueueMessage(ctx, "session-keep", "task-session-queue", "keep me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage keep: %v", err)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-session-queue"); err != nil || got != 2 {
		t.Fatalf("pending before delete = %d err=%v, want 2", got, err)
	}

	if err := svc.DeleteSession(ctx, "session-drop"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	if got := svc.messageQueue.GetStatus(ctx, "session-drop").Count; got != 0 {
		t.Fatalf("session-drop queue count = %d, want 0", got)
	}
	if got, err := svc.messageQueue.CountPendingByTask(ctx, "task-session-queue"); err != nil || got != 1 {
		t.Fatalf("pending after delete = %d err=%v, want 1 (kept session only)", got, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for saw.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if saw.Load() == 0 {
		t.Fatal("expected message.queue.status_changed with task_id after session delete")
	}

}
