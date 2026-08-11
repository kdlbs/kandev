package sqlite

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

// TestArchiveTaskNotifiesQueuePurgeAfterCommit proves the post-commit purge
// notifier fires after ArchiveTask empties queued_messages. Live badge zeroing
// depends on this hook publishing message.queue.status_changed.
func TestArchiveTaskNotifiesQueuePurgeAfterCommit(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-queue-purge-notify")
	ctx := context.Background()

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	if _, err := queue.QueueMessage(ctx, "session-1", "task-queue-purge-notify", "follow up", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-queue-purge-notify"); err != nil || got != 1 {
		t.Fatalf("pending before archive = %d err=%v, want 1", got, err)
	}

	var notified atomic.Int32
	var notifiedTask string
	repo.SetTaskQueuePurgeNotifier(func(_ context.Context, taskID string) {
		notified.Add(1)
		notifiedTask = taskID
	})

	if err := repo.ArchiveTask(ctx, "task-queue-purge-notify"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	if notified.Load() != 1 {
		t.Fatalf("purge notifier calls = %d, want 1 after ArchiveTask", notified.Load())
	}
	if notifiedTask != "task-queue-purge-notify" {
		t.Fatalf("notified task_id = %q, want task-queue-purge-notify", notifiedTask)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-queue-purge-notify"); err != nil || got != 0 {
		t.Fatalf("pending after archive = %d err=%v, want 0", got, err)
	}

	task, err := repo.GetTask(ctx, "task-queue-purge-notify")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil after archive")
	}
}

func TestDeleteTaskNotifiesQueuePurgeAfterCommit(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-delete-purge-notify")
	ctx := context.Background()

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	if _, err := queue.QueueMessage(ctx, "session-1", "task-delete-purge-notify", "follow up", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage: %v", err)
	}

	var notified atomic.Int32
	repo.SetTaskQueuePurgeNotifier(func(_ context.Context, taskID string) {
		if taskID == "task-delete-purge-notify" {
			notified.Add(1)
		}
	})

	if err := repo.DeleteTask(ctx, "task-delete-purge-notify"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if notified.Load() != 1 {
		t.Fatalf("purge notifier calls = %d, want 1 after DeleteTask", notified.Load())
	}
	if _, err := repo.GetTask(ctx, "task-delete-purge-notify"); err == nil {
		t.Fatal("expected task gone after DeleteTask")
	}
}
