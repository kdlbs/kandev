package service

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func TestServiceCreateQueuedMessageOnce(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	sessionID, _ := seedServicePlanComment(t, ctx, repo, "once")
	db := sqlx.NewDb(repo.DB(), "sqlite3")
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatal(err)
	}
	request := func() *CreateMessageRequest {
		return &CreateMessageRequest{
			TaskSessionID: sessionID, TaskID: "task-123", Content: "reply", AuthorType: "user", AuthorID: "manager-a",
			Metadata: map[string]interface{}{"coordinator_reply_proposal_id": "p1"},
		}
	}
	queued := func() *messagequeue.QueuedMessage {
		return &messagequeue.QueuedMessage{
			Metadata: map[string]interface{}{"user_message_recorded": true}, QueuedBy: messagequeue.QueuedByUser,
		}
	}
	eventBus.ClearEvents()

	first, created, err := svc.CreateQueuedMessageOnce(ctx, "coordinator-reply:p1", request(), queued(), 10)
	if err != nil || !created {
		t.Fatalf("first: created=%v err=%v", created, err)
	}
	if first.AuthorID != "manager-a" {
		t.Fatalf("author = %q, want the request's author", first.AuthorID)
	}
	if got := eventTypesForPlanCommentTest(eventBus); len(got) == 0 || got[0] != events.MessageAdded {
		t.Fatalf("events after create = %v", got)
	}

	eventBus.ClearEvents()
	again, created, err := svc.CreateQueuedMessageOnce(ctx, "coordinator-reply:p1", request(), queued(), 10)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay: created=%v err=%v msg=%v", created, err, again)
	}
	if got := eventTypesForPlanCommentTest(eventBus); len(got) != 0 {
		t.Fatalf("replay published events %v", got)
	}
	entries, err := queueRepo.ListBySession(ctx, sessionID)
	if err != nil || len(entries) != 1 || entries[0].ID != "coordinator-reply:p1" {
		t.Fatalf("queue entries = %#v err=%v", entries, err)
	}
	if entries[0].Metadata["user_message_recorded"] != true {
		t.Fatalf("queue metadata = %#v", entries[0].Metadata)
	}
}

func TestServiceCreateQueuedMessageOnceRefusesNonUserAuthor(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	sessionID, _ := seedServicePlanComment(t, ctx, repo, "once-agent")
	req := &CreateMessageRequest{TaskSessionID: sessionID, TaskID: "task-123", Content: "x", AuthorType: "agent", AuthorID: "someone"}
	queued := &messagequeue.QueuedMessage{QueuedBy: messagequeue.QueuedByUser}
	if _, created, err := svc.CreateQueuedMessageOnce(ctx, "coordinator-reply:agent", req, queued, 10); err == nil || created {
		t.Fatalf("created=%v err=%v", created, err)
	}
}
