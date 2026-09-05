package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

type planCommentMessageRepository interface {
	CreateMessageWithPlanComments(
		context.Context,
		*models.Message,
		[]models.TaskPlanCommentRef,
		bool,
	) (*models.TaskPlanCommentSnapshot, error)
}

type queuedPlanCommentMessageRepository interface {
	CreateMessageWithPlanCommentsAndQueue(
		context.Context,
		*models.Message,
		*messagequeue.QueuedMessage,
		[]models.TaskPlanCommentRef,
		bool,
		int,
	) (*models.TaskPlanCommentSnapshot, error)
}

func requirePlanCommentMessageRepository(t *testing.T, repo *Repository) planCommentMessageRepository {
	t.Helper()
	contract, ok := any(repo).(planCommentMessageRepository)
	if !ok {
		t.Fatal("Repository does not implement atomic plan-comment message creation")
	}
	return contract
}

func TestCreateMessageWithPlanCommentsConsumesAtomically(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "atomic")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("atomic", "message-plan-comments")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-atomic", Version: 1}}, true)
	if err != nil {
		t.Fatalf("CreateMessageWithPlanComments: %v", err)
	}
	if snapshot.Revision != 2 || len(snapshot.Comments) != 0 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	want := "### Plan Comments\n\n```\nselected\n```\n> stored atomic\n\n---\n\ntyped content"
	if message.Content != want {
		t.Fatalf("message content = %q, want %q", message.Content, want)
	}
	stored, err := repo.GetMessageWithPromptIndex(ctx, message.ID)
	if err != nil || stored.Content != want || stored.PromptIndex != 1 {
		t.Fatalf("stored message = %#v, err=%v", stored, err)
	}
	var storedMetadata string
	if err := repo.db.GetContext(ctx, &storedMetadata, repo.db.Rebind(
		`SELECT metadata FROM task_session_messages WHERE id = ?`,
	), message.ID); err != nil {
		t.Fatalf("read stored metadata: %v", err)
	}
	if storedMetadata != "{}" {
		t.Fatalf("stored metadata = %q, want an empty JSON object", storedMetadata)
	}
}

func TestCreateMessageWithPlanCommentsRollsBackOnStaleReference(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "stale")
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("stale", "message-plan-comments-stale")

	snapshot, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-stale", Version: 9}}, false)
	var changed *plancommenttx.CommentsChangedError
	if !errors.As(err, &changed) || snapshot != nil || changed.Snapshot == nil {
		t.Fatalf("stale result snapshot=%#v err=%#v", snapshot, err)
	}
	if _, err := repo.GetMessageWithPromptIndex(ctx, message.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rolled-back message lookup error = %v, want sql.ErrNoRows", err)
	}
	pending, err := repo.ListTaskPlanComments(ctx, "task-message-comments-stale")
	if err != nil || pending.Revision != 1 || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, err)
	}
}

func TestCreateMessageWithPlanCommentsRejectsStalePrimary(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	seedMessagePlanComment(t, ctx, repo, "primary")
	secondaryID := "session-message-comments-secondary"
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: secondaryID, TaskID: "task-message-comments-primary", State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatal(err)
	}
	writes := requirePlanCommentMessageRepository(t, repo)
	message := planCommentMessage("primary", "message-plan-comments-primary")
	message.TaskSessionID = secondaryID

	_, err := writes.CreateMessageWithPlanComments(ctx, message,
		[]models.TaskPlanCommentRef{{ID: "comment-primary", Version: 1}}, true)
	var changed *plancommenttx.PrimarySessionChangedError
	if !errors.As(err, &changed) || changed.SessionID != "session-message-comments-primary" {
		t.Fatalf("primary guard error = %#v", err)
	}
	pending, listErr := repo.ListTaskPlanComments(ctx, "task-message-comments-primary")
	if listErr != nil || len(pending.Comments) != 1 {
		t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
	}
}

func TestCreateMessageWithPlanCommentsAndQueueIsAtomic(t *testing.T) {
	tests := []struct {
		name             string
		occupyQueueID    bool
		wantErr          error
		wantCommentCount int
		wantMessage      bool
	}{
		{name: "commits message queue and consumption", wantMessage: true},
		{
			name: "queue conflict rolls back message and consumption", occupyQueueID: true,
			wantErr: messagequeue.ErrQueueIDConflict, wantCommentCount: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			seedMessagePlanComment(t, ctx, repo, "queued")
			queueRepo, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
			if err != nil {
				t.Fatal(err)
			}
			if test.occupyQueueID {
				err = queueRepo.Insert(ctx, &messagequeue.QueuedMessage{
					ID: "message-plan-comments-queued", SessionID: "session-message-comments-queued",
					TaskID: "task-message-comments-queued", Content: "different", QueuedBy: messagequeue.QueuedByUser,
				}, 10)
				if err != nil {
					t.Fatal(err)
				}
			}
			writes, ok := any(repo).(queuedPlanCommentMessageRepository)
			if !ok {
				t.Fatal("Repository does not implement atomic queued plan-comment message creation")
			}
			message := planCommentMessage("queued", "message-plan-comments-queued")
			queued := &messagequeue.QueuedMessage{
				ID: message.ID, SessionID: message.TaskSessionID, TaskID: message.TaskID,
				Content: message.Content, Model: "test-model", PlanMode: true,
				Metadata: map[string]interface{}{"user_message_recorded": true}, QueuedBy: messagequeue.QueuedByUser,
			}

			snapshot, err := writes.CreateMessageWithPlanCommentsAndQueue(
				ctx, message, queued, []models.TaskPlanCommentRef{{ID: "comment-queued", Version: 1}}, true, 10,
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateMessageWithPlanCommentsAndQueue error = %v, want %v", err, test.wantErr)
			}
			pending, listErr := repo.ListTaskPlanComments(ctx, message.TaskID)
			if listErr != nil || len(pending.Comments) != test.wantCommentCount {
				t.Fatalf("pending snapshot = %#v, err=%v", pending, listErr)
			}
			stored, messageErr := repo.GetMessageWithPromptIndex(ctx, message.ID)
			if test.wantMessage {
				if messageErr != nil || stored.Content != queued.Content || snapshot == nil || snapshot.Revision != 2 {
					t.Fatalf("stored message=%#v queued=%#v snapshot=%#v err=%v", stored, queued, snapshot, messageErr)
				}
			} else if !errors.Is(messageErr, sql.ErrNoRows) {
				t.Fatalf("rolled-back message lookup error = %v, want sql.ErrNoRows", messageErr)
			}
			entries, queueErr := queueRepo.ListBySession(ctx, message.TaskSessionID)
			if queueErr != nil {
				t.Fatal(queueErr)
			}
			if test.wantMessage {
				if len(entries) != 1 || entries[0].ID != message.ID || entries[0].Content != message.Content {
					t.Fatalf("queued entries = %#v", entries)
				}
			} else if len(entries) != 1 || entries[0].Content != "different" {
				t.Fatalf("conflicting queue entry changed: %#v", entries)
			}
		})
	}
}

func seedMessagePlanComment(t *testing.T, ctx context.Context, repo *Repository, suffix string) {
	t.Helper()
	taskID := "task-message-comments-" + suffix
	sessionID := "session-message-comments-" + suffix
	seedForMsgTest(t, repo, taskID, sessionID, "turn-message-comments-"+suffix)
	if err := repo.SetSessionPrimary(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskPlan(ctx, &models.TaskPlan{
		ID: "plan-" + suffix, TaskID: taskID, Content: "Plan",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateTaskPlanComment(ctx, &models.TaskPlanComment{
		ID: "comment-" + suffix, TaskID: taskID, PlanID: "plan-" + suffix,
		Body: "stored " + suffix, SelectedText: "selected", AnchorFrom: 1, AnchorTo: 4,
	}); err != nil {
		t.Fatal(err)
	}
}

func planCommentMessage(suffix, id string) *models.Message {
	return &models.Message{
		ID: id, TaskID: "task-message-comments-" + suffix,
		TaskSessionID: "session-message-comments-" + suffix,
		TurnID:        "turn-message-comments-" + suffix,
		AuthorType:    models.MessageAuthorUser, Type: models.MessageTypeMessage,
		Content: plancomments.WithPlaceholder("typed content"),
	}
}
