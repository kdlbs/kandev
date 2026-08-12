package sqlite

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// TestArchiveTaskNotifiesQueuePurgeAfterCommit proves the post-commit purge
// notifier fires after ArchiveTask empties queued_messages. Live badge zeroing
// depends on this hook publishing message.queue.status_changed.
func TestArchiveTaskNotifiesQueuePurgeAfterCommit(t *testing.T) {
	repo := newRepoForArchiveTests(t, "task-queue-purge-notify")
	ctx := context.Background()
	seedLiveSessionForQueue(t, repo, "session-1", "task-queue-purge-notify")

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
	seedLiveSessionForQueue(t, repo, "session-1", "task-delete-purge-notify")

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

func TestDeleteTaskSessionPurgesQueuedMessages(t *testing.T) {
	// Session delete used to leave queued_messages rows behind; CountPendingByTask
	// then kept the sidebar badge inflated. Cascade purge must clear them.
	repo := newRepoForArchiveTests(t, "task-session-queue-purge")
	ctx := context.Background()

	seedLiveSessionForQueue(t, repo, "session-1", "task-session-queue-purge")
	seedLiveSessionForQueue(t, repo, "session-drop", "task-session-queue-purge")

	mqRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	queue := messagequeue.NewService(mqRepo, messagequeue.DefaultMaxPerSession, log)
	if _, err := queue.QueueMessage(ctx, "session-drop", "task-session-queue-purge", "orphan me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage drop: %v", err)
	}
	if _, err := queue.QueueMessage(ctx, "session-1", "task-session-queue-purge", "keep me", "", "user", false, nil); err != nil {
		t.Fatalf("QueueMessage keep: %v", err)
	}

	if err := repo.DeleteTaskSession(ctx, "session-drop"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}
	if got := queue.GetStatus(ctx, "session-drop").Count; got != 0 {
		t.Fatalf("session-drop queue count = %d, want 0", got)
	}
	if got, err := queue.CountPendingByTask(ctx, "task-session-queue-purge"); err != nil || got != 1 {
		t.Fatalf("pending after session delete = %d err=%v, want 1 (kept session)", got, err)
	}
}

func seedLiveSessionForQueue(t *testing.T, repo *Repository, sessionID, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID,
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", sessionID, err)
	}
}
