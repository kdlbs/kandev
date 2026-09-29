package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type queuedMessageOnceRepository interface {
	CreateMessageAndQueueOnce(context.Context, *models.Message, *messagequeue.QueuedMessage, int) (bool, error)
}

func onceFixture(id string, message *models.Message) (*models.Message, *messagequeue.QueuedMessage) {
	m := *message
	m.ID = id
	m.Content = "reply text"
	return &m, &messagequeue.QueuedMessage{
		ID: id, SessionID: m.TaskSessionID, TaskID: m.TaskID, Content: m.Content,
		Metadata: map[string]interface{}{"user_message_recorded": true}, QueuedBy: messagequeue.QueuedByUser,
	}
}

func runQueuedMessageOnceScenarios(t *testing.T, repo *Repository, base *models.Message) {
	t.Helper()
	ctx := context.Background()
	queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	writer, ok := any(repo).(queuedMessageOnceRepository)
	if !ok {
		t.Fatal("Repository does not implement CreateMessageAndQueueOnce")
	}
	count := func() int {
		entries, listErr := queueRepo.ListBySession(ctx, base.TaskSessionID)
		if listErr != nil {
			t.Fatal(listErr)
		}
		return len(entries)
	}

	t.Run("first call creates one message and one entry", func(t *testing.T) {
		message, queued := onceFixture("once-first", base)
		created, err := writer.CreateMessageAndQueueOnce(ctx, message, queued, 10)
		if err != nil || !created {
			t.Fatalf("created=%v err=%v, want true nil", created, err)
		}
		if _, err := repo.GetMessage(ctx, "once-first"); err != nil {
			t.Fatalf("message not stored: %v", err)
		}
		if got := count(); got != 1 {
			t.Fatalf("queue entries = %d, want 1", got)
		}
	})

	t.Run("second call with the same id adds nothing", func(t *testing.T) {
		message, queued := onceFixture("once-first", base)
		created, err := writer.CreateMessageAndQueueOnce(ctx, message, queued, 10)
		if err != nil || created {
			t.Fatalf("created=%v err=%v, want false nil", created, err)
		}
		if got := count(); got != 1 {
			t.Fatalf("queue entries = %d, want still 1", got)
		}
	})

	t.Run("failure inside the transaction stores neither", func(t *testing.T) {
		if err := queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
			ID: "once-conflict", SessionID: base.TaskSessionID, TaskID: base.TaskID, Content: "other",
			QueuedBy: messagequeue.QueuedByUser,
		}, 10); err != nil {
			t.Fatal(err)
		}
		before := count()
		message, queued := onceFixture("once-conflict", base)
		created, err := writer.CreateMessageAndQueueOnce(ctx, message, queued, 10)
		if !errors.Is(err, messagequeue.ErrQueueIDConflict) || created {
			t.Fatalf("created=%v err=%v, want queue id conflict", created, err)
		}
		if _, err := repo.GetMessage(ctx, "once-conflict"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("message persisted after failure: %v", err)
		}
		if got := count(); got != before {
			t.Fatalf("queue entries = %d, want %d", got, before)
		}
	})

	t.Run("full queue stores nothing", func(t *testing.T) {
		before := count()
		message, queued := onceFixture("once-full", base)
		created, err := writer.CreateMessageAndQueueOnce(ctx, message, queued, before)
		if !errors.Is(err, messagequeue.ErrQueueFull) || created {
			t.Fatalf("created=%v err=%v, want ErrQueueFull", created, err)
		}
		if _, err := repo.GetMessage(ctx, "once-full"); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("message persisted on full queue: %v", err)
		}
	})
}

func TestCreateMessageAndQueueOnce(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedMessagePlanComment(t, context.Background(), repo, "once")
	runQueuedMessageOnceScenarios(t, repo, planCommentMessage("once", "base"))
}
