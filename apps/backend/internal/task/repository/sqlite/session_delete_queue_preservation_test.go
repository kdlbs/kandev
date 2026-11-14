package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestDeleteTaskSessionPreservesPendingQueue(t *testing.T) {
	const taskID = "task-session-delete-queue"
	const sessionID = "session-delete-queue"
	repo := newRepoForArchiveTests(t, taskID)
	seedLiveSessionForQueue(t, repo, sessionID, taskID)
	ctx := context.Background()
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, log)
	entry, err := queue.QueueMessage(ctx, sessionID, taskID, "unread human message", "", "user", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTaskSession(ctx, session); !errors.Is(err, messagequeue.ErrSessionQueueNotEmpty) {
		t.Fatalf("session deletion error = %v, want pending queue conflict", err)
	}
	if _, err := repo.GetTaskSession(ctx, sessionID); err != nil {
		t.Fatalf("session was removed after rejected deletion: %v", err)
	}
	got, err := queueRepo.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != entry.ID || got[0].Content != entry.Content {
		t.Fatalf("queue changed after rejected deletion: %+v", got)
	}
}
